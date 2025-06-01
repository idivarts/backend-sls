package streamsqs

import (
	"fmt"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func HandleUnreadMessage(body *StreamWebhook) {
	if body.Type != "user.unread_message_reminder" {
		return
	}

	// for channelID, reminderData := range body.Channels {
	// 	fmt.Printf("Channel ID: %s\n", channelID)
	// 	fmt.Printf("Messages: %d\n", len(reminderData.Messages))
	// 	for _, message := range reminderData.Messages {
	// 		fmt.Printf("Message ID: %s, Text: %s\n", message.ID, message.Text)
	// 	}
	// }

	userId := body.User.ID
	user := &trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		fmt.Printf("Error retrieving user %s: %v\n", userId, err)
		return
	}

}
