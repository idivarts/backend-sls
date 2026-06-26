package facebook_test

import (
	"testing"

	"github.com/idivarts/backend-sls/pkg/facebook"
)

var conversationID = ""

func TestGetAll(t *testing.T) {
	data, err := facebook.GetConversationsPaginated("", 10, facebook.TestPageAccessToken, facebook.PlatformMessenger)
	if err != nil {
		t.Fail()
	}
	// str, err := json.Marshal(*data)
	// if err != nil {
	// 	t.Fail()
	// }
	// t.Log("Print Data", string(str))
	if len(data.Data) > 0 {
		conversationID = data.Data[0].ID
		getMessages(t)
		// fmt.Println("Sending Message to", data.Data[0].Participants.Data[0].ID, data.Data[0].Participants.Data[0].Username)
		messageParticipant(facebook.GetRecepientIDFromParticipants(data.Data[0].Participants, facebook.TestUserName), t)
	}
}
func messageParticipant(igSid string, t *testing.T) {
	_, err := facebook.SendTextMessage(igSid, "Hello Everyone! Final test is here", facebook.TestPageAccessToken)
	if err != nil {
		t.Log(err)
		t.Fail()
	}
}
func getMessages(t *testing.T) {
	messages, err := facebook.GetConversationById(conversationID, facebook.TestPageAccessToken)
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
		msg, err := facebook.GetMessageInfo(messages.Messages.Data[0].ID, facebook.TestPageAccessToken)
		if err != nil {
			// t.Log(err)
			t.Fail()
		}
		t.Log(msg)
	}

}
