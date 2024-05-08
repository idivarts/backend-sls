package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	openaitools "github.com/TrendsHub/th-backend/internal/openai/tools"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	thread, err := openai.CreateThread()
	if err != nil {
		return
	}

	conv := &sqsevents.ConversationEvent{
		IGSID:    "test-user-" + strconv.Itoa(rand.Intn(1000)),
		ThreadID: thread.ID,
	}
	log.Println("Custom IGSID - ", conv.IGSID)
	cData := models.Conversation{
		IGSID:    conv.IGSID,
		ThreadID: conv.ThreadID,
	}
	cData.Insert()

	openai.SendMessage(thread.ID, "Hello Rahul, How are you doing?\nI came across your profile. Would you be interested to collab with brands?", true)

	for i := 0; i < 100; i++ {
		fmt.Print("Enter your input: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}
		_, err = openai.SendMessage(thread.ID, input, false)
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}

		run, err := openai.StartRun(thread.ID, openai.ArjunAssistant, "", "")
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}
		time.Sleep(5 * time.Second)
		for j := 0; j < 10; j++ {
			run, err = openai.GetRunStatus(thread.ID, run.ID)
			if err != nil {
				log.Printf("Error %s", err.Error())
				return
			}
			if run.Status == openai.COMPLETED_STATUS {
				break
			} else if run.Status == openai.REQUIRES_ACTION_STATUS {
				log.Println("Requires Action", run.RequiredAction.SubmitToolOutputs.ToolCalls[0].ID, "\n", run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Name, run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Arguments)
				toolOutput := []openai.ToolOutput{}
				for _, toolOption := range run.RequiredAction.SubmitToolOutputs.ToolCalls {
					if toolOption.Function.Name == openai.CanConversationEndFn {
						t, err := openaitools.CanConversationEnd(toolOption)
						if err != nil {
							log.Printf("Error %s", err.Error())
							return
						}
						toolOutput = append(toolOutput, *t)
					} else if toolOption.Function.Name == openai.ChangePhaseFn {
						t, err := openaitools.ChangePhaseFn(conv, toolOption)
						if err != nil {
							log.Printf("Error %s", err.Error())
							return
						}
						toolOutput = append(toolOutput, *t)
					} else {
						err = errors.New("Not implemented function -- " + string(toolOption.Function.Name))
						log.Printf("Error %s", err.Error())
						return
					}
				}
				_, err = openai.SubmitToolOutput(thread.ID, run.ID, toolOutput)
				if err != nil {
					log.Printf("Error %s", err.Error())
					return
				}
				// return
			}
			time.Sleep(time.Second)
		}
		messages, err := openai.GetMessages(thread.ID, 10, run.ID)
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}
		for i2, j := 0, len(messages.Data)-1; i2 < j; i2, j = i2+1, j-1 {
			messages.Data[i2], messages.Data[j] = messages.Data[j], messages.Data[i2]
		}

		for _, v := range messages.Data {
			log.Println("Arjun :", v.Content[0].Text.Value)
		}
	}

	// fmt.Println("You entered:", input)
}
