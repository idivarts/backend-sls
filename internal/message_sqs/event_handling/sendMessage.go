package eventhandling

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	openaitools "github.com/TrendsHub/th-backend/internal/openai/tools"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
)

type IProcessedInput struct {
	StringVal string
	Number    int
}

func processInput(input string) []IProcessedInput {
	// Split the input string by "\n" delimiter
	lines := strings.Split(input, "\n")

	var result []IProcessedInput

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		// Calculate the weight based on the length of the string
		weight := (len(line) / 7)

		obj := IProcessedInput{
			StringVal: line,
			Number:    weight,
		}
		result = append(result, obj)
	}

	return result
}
func InstaSend(conv *sqsevents.ConversationEvent) error {
	log.Println("Sending Message to instagram", conv.Message)
	_, err := messenger.SendTextMessage(conv.IGSID, conv.Message, conv.PageToken)
	if err != nil {
		return err
	}
	if conv.LastMessage != nil && !*conv.LastMessage {
		_, err = messenger.SendAction(conv.IGSID, messenger.TYPING_ON, conv.PageToken)
		if err != nil {
			log.Println("Error while send Action", err.Error())
		}
	}
	// if mResp != nil {
	// 	mID = mResp.MessageID
	// }
	return nil
}
func WaitAndSend(conv *sqsevents.ConversationEvent) error {
	log.Println("Getting messaged from thread", conv.ThreadID)

	run, err := openai.GetRunStatus(conv.ThreadID, conv.RunID)
	if err != nil {
		return err
	}
	if run.Status == openai.COMPLETED_STATUS {
		msgs, err := openai.GetMessages(conv.ThreadID, 10, conv.RunID)
		if err != nil {
			return err
		}
		log.Println("Fetching Conversation", conv.IGSID)
		cData := &models.Conversation{}
		err = cData.Get(conv.IGSID)
		if err != nil {
			return err
		}
		log.Println("Fetching Page", cData.PageID)
		pData := &models.Page{}
		err = pData.Get(cData.PageID)
		if err != nil || pData.PageID == "" {
			return err
		}

		// log.Println("Message received", len(msgs.Data[0].Content), msgs.Data[0].Content[0].Text.Value)
		for i, j := 0, len(msgs.Data)-1; i < j; i, j = i+1, j-1 {
			msgs.Data[i], msgs.Data[j] = msgs.Data[j], msgs.Data[i]
		}

		cData.LastBotMessageTime = time.Now().UnixMilli()
		_, err = cData.Insert()
		if err != nil {
			return err
		}

		// mID := ""
		for _, v := range msgs.Data {
			if v.RunID == conv.RunID {
				aMsg := v.Content[0].Text
				log.Println("Sending Message", conv.IGSID, aMsg.Value, v.ID)
				pInp := processInput(aMsg.Value)
				secondsElapsed := 0
				_, err = messenger.SendAction(cData.IGSID, messenger.TYPING_ON, pData.AccessToken)
				if err != nil {
					log.Println("Error while send Action", err.Error())
				}
				for i, v := range pInp {
					isLast := (i == len(pInp))
					st := sqsevents.ConversationEvent{
						IGSID:       conv.IGSID,
						Message:     v.StringVal,
						PageToken:   pData.AccessToken,
						Action:      sqsevents.INSTA_SEND,
						LastMessage: &isLast,
					}
					b, err := json.Marshal(&st)
					secondsElapsed = secondsElapsed + v.Number
					if err == nil {
						sqshandler.SendToMessageQueue(string(b), int64(secondsElapsed))
					}
				}
			}
		}
		// if mID == "" {
		// 	return errors.New("Cant find the message even after completion of Run --" + conv.RunID)
		// }
		// cData.LastMID = mID
		return nil
	} else if run.Status == openai.REQUIRES_ACTION_STATUS {
		toolOutput := []openai.ToolOutput{}
		for _, toolOption := range run.RequiredAction.SubmitToolOutputs.ToolCalls {
			if toolOption.Function.Name == openai.CanConversationEndFn {
				t, err := openaitools.CanConversationEnd(toolOption)
				if err != nil {
					return err
				}
				toolOutput = append(toolOutput, *t)
			} else if toolOption.Function.Name == openai.ChangePhaseFn {
				t, err := openaitools.ChangePhaseFn(conv, toolOption, nil)
				if err != nil {
					return err
				}
				toolOutput = append(toolOutput, *t)
			} else {
				return errors.New("Not implemented function -- " + string(toolOption.Function.Name))
			}
		}
		_, err = openai.SubmitToolOutput(conv.ThreadID, conv.RunID, toolOutput)
		if err != nil {
			// log.Printf("Error %s", err.Error())
			return err
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
