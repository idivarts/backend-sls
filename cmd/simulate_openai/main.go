package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"time"

	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	thread, err := openai.CreateThread()
	if err != nil {
		return
	}

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
			if run.Status == "completed" {
				break
			} else if run.Status == "requires_action" {
				log.Println("Requires Action", run.RequiredAction.SubmitToolOutputs.ToolCalls[0].ID, "\n", run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Name, run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Arguments)
				if run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Name == "can_conversation_end" {
					cce := &openaifc.CanConversationEnd{}
					err = cce.ParseJson(run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Arguments)
					if err != nil {
						log.Printf("Error %s", err.Error())
						return
					}
					eOutput, err := cce.FindEmptyFields()
					if err != nil {
						log.Printf("Error %s", err.Error())
						return
					}
					log.Println("Output to be send", *eOutput)
					run, err = openai.SubmitToolOutput(thread.ID, run.ID, []openai.ToolOutput{
						{
							ToolCallId: run.RequiredAction.SubmitToolOutputs.ToolCalls[0].ID,
							Output:     *eOutput,
						},
					})
					if err != nil {
						log.Printf("Error %s", err.Error())
						return
					}
				}
				// return
			}
			time.Sleep(time.Second)
		}
		message, err := openai.GetMessages(thread.ID, 1, run.ID)
		if err != nil {
			log.Printf("Error %s", err.Error())
			return
		}
		log.Println("Arjun :", message.Data[0].Content[0].Text.Value)
	}

	// fmt.Println("You entered:", input)
}
