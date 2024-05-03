package messenger_test

import (
	"testing"

	"github.com/TrendsHub/th-backend/pkg/messenger"
)

var conversationID = ""

func TestGetAll(t *testing.T) {
	data, err := messenger.GetAllConversationInfo()
	if err != nil {
		t.Fail()
	}
	// str, err := json.Marshal(*data)
	// if err != nil {
	// 	t.Fail()
	// }
	// t.Log("Print Data", string(str))
	if len(data.Data) > 0 {
		// conversationID = data.Data[0].ID
		// getMessages(t)
		// fmt.Println("Sending Message to", data.Data[0].Participants.Data[0].ID, data.Data[0].Participants.Data[0].Username)
		messageParticipant(messenger.GetRecepientIDFromParticipants(data.Data[0].Participants), t)
	}
}
func messageParticipant(igSid string, t *testing.T) {
	err := messenger.SendTextMessage(igSid, "Hello Everyone! Final test is here")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
}
func getMessages(t *testing.T) {
	messages, err := messenger.GetConversationMessages(conversationID)
	if err != nil {
		// t.Log(err)
		t.Fail()
	}
	// str, err := json.Marshal(*data)
	// if err != nil {
	// 	t.Fail()
	// }
	// t.Log("Print Messages Data", string(str))
	if len(messages.Messages.Data) > 0 {
		msg, err := messenger.GetMessageInfo(messages.Messages.Data[0].ID)
		if err != nil {
			// t.Log(err)
			t.Fail()
		}
		t.Log(msg)
	}

}
