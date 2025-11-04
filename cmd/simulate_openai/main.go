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

	sqsevents "github.com/idivarts/backend-sls/internal/message_sqs/events"
	"github.com/idivarts/backend-sls/internal/models"
	openaitools "github.com/idivarts/backend-sls/internal/openai/tools"
	"github.com/idivarts/backend-sls/pkg/myopenai"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	thread, err := myopenai.CreateThread()
	if err != nil {
		return
	}

	conv := &sqsevents.ConversationEvent{
		LeadID:   "test-user-" + strconv.Itoa(rand.Intn(1000)),
		ThreadID: thread.ID,
	}
	log.Println("Custom IGSID - ", conv.LeadID)
	cData := models.Conversation{
		LeadID:   conv.LeadID,
		ThreadID: conv.ThreadID,
	}
	cData.Insert()

	fMsg := "Hello Debangana, How are you doing?\nI came across your profile. Would you be interested to collab with brands?"
	myopenai.SendMessage(thread.ID, fMsg, nil, true)
	log.Println("\n---------------------\nArjun :", fMsg, "\n---------------------")

	for i := 0; i < 100; i++ {
		fmt.Print("Enter your input: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}
		_, err = myopenai.SendMessage(thread.ID, input, nil, false)
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}

		run, err := myopenai.StartRun(thread.ID, myopenai.ArjunAssistant, "", string(myopenai.ChangePhaseFn))
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}
		time.Sleep(5 * time.Second)
		for j := 0; j < 10; j++ {
			run, err = myopenai.GetRunStatus(thread.ID, run.ID)
			if err != nil {
				log.Printf("Error %s", err.Error())
				return
			}
			if run.Status == myopenai.COMPLETED_STATUS {
				break
			} else if run.Status == myopenai.REQUIRES_ACTION_STATUS {
				// log.Println("Requires Action", run.RequiredAction.SubmitToolOutputs.ToolCalls[0].ID, "\n", run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Name, run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Arguments)
				log.Println("\n-------------------------")
				toolOutput := []myopenai.ToolOutput{}
				for _, toolOption := range run.RequiredAction.SubmitToolOutputs.ToolCalls {
					if toolOption.Function.Name == myopenai.CanConversationEndFn {
						t, err := openaitools.CanConversationEnd(toolOption)
						if err != nil {
							log.Printf("Error %s", err.Error())
							return
						}
						toolOutput = append(toolOutput, *t)
					} else if toolOption.Function.Name == myopenai.ChangePhaseFn {
						t, err := openaitools.ChangePhaseFn(conv, toolOption, &cData)
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
				_, err = myopenai.SubmitToolOutput(thread.ID, run.ID, toolOutput)
				if err != nil {
					log.Printf("Error %s", err.Error())
					return
				}
				// return
			}
			time.Sleep(time.Second)
		}
		messages, err := myopenai.GetMessages(thread.ID, 10, run.ID)
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}
		for i2, j := 0, len(messages.Data)-1; i2 < j; i2, j = i2+1, j-1 {
			messages.Data[i2], messages.Data[j] = messages.Data[j], messages.Data[i2]
		}

		for _, v := range messages.Data {
			log.Println("\n---------------------\nArjun :", v.Content[0].Text.Value, "\n---------------------")
		}
	}

	// fmt.Println("You entered:", input)
}
