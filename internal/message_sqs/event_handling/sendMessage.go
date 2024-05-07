package eventhandling

import (
	"encoding/json"
	"errors"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
)

func WaitAndSend(conv *sqsevents.ConversationEvent) error {
	log.Println("Getting messaged from thread", conv.ThreadID)

	run, err := openai.GetRunStatus(conv.ThreadID, conv.RunID)
	if err != nil {
		return err
	}
	if run.Status == openai.COMPLETED_STATUS {
		msgs, err := openai.GetMessages(conv.ThreadID, 1, conv.RunID)
		if err != nil {
			return err
		}
		log.Println("Message received", len(msgs.Data[0].Content), msgs.Data[0].Content[0].Text.Value)

		for _, v := range msgs.Data {
			if v.RunID == conv.RunID {
				aMsg := v.Content[0].Text
				log.Println("Sending Message", conv.IGSID, aMsg.Value, v.ID)
				messenger.SendTextMessage(conv.IGSID, aMsg.Value)

				return nil
			}
		}
		return errors.New("Cant find the message even after completion of Run --" + conv.RunID)
	} else if run.Status == openai.REQUIRES_ACTION_STATUS {
		log.Println("Requires Action", run.RequiredAction.SubmitToolOutputs.ToolCalls[0].ID, "\n", run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Name, run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Arguments)
		if run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Name == "can_conversation_end" {
			cce := &openaifc.CanConversationEnd{}
			err = cce.ParseJson(run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Arguments)
			if err != nil {
				// log.Printf("Error %s", err.Error())
				return err
			}
			eOutput, err := cce.FindEmptyFields()
			if err != nil {
				// log.Printf("Error %s", err.Error())
				return err
			}
			log.Println("Output to be send", *eOutput)
			_, err = openai.SubmitToolOutput(conv.ThreadID, conv.RunID, []openai.ToolOutput{
				{
					ToolCallId: run.RequiredAction.SubmitToolOutputs.ToolCalls[0].ID,
					Output:     *eOutput,
				},
			})
			if err != nil {
				// log.Printf("Error %s", err.Error())
				return err
			}
		} else {
			return errors.New("Not implemented function -- " + run.RequiredAction.SubmitToolOutputs.ToolCalls[0].Function.Name)
		}
		// return
	} else if run.Status == openai.EXPIRED_STATUS {
		log.Println("The run is exipired --- Exiting")
		return nil
	}
	// time.Sleep(time.Second)

	log.Println("Waiting for 1 second")
	b, err := json.Marshal(conv)
	if err != nil {
		return err
	}
	log.Println("Sending wait message", string(b))
	sqshandler.SendToMessageQueue(string(b), 1)
	return nil

}
