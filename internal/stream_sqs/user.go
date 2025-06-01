package streamsqs

import (
	"fmt"

	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/templates"
)

func HandleUnreadMessage(body *StreamWebhook) {
	if body.Type != "user.unread_message_reminder" {
		return
	}

	userId := body.User.ID
	user := &trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		fmt.Printf("Error retrieving user %s: %s\n", userId, err.Error())
		return
	}

	var firstChannelKey string
	for k := range body.Channels {
		firstChannelKey = k
		break
	}
	channel := body.Channels[firstChannelKey]

	// Dynamic Variables:
	// {{.RecipientName}}     => Name of the user receiving the email
	// {{.CollabTitle}}       => Title of the collaboration
	// {{.FirstPendingMessage}} => Preview of the first unread message
	// {{.PendingCount}}      => Total number of unread messages
	// {{.OpenChatLink}}      => Link to open the chat in the app
	data := map[string]interface{}{
		"RecipientName":       user.Name,
		"CollabTitle":         channel.Channel.Name,                                                             // Assuming the first channel is the relevant one
		"FirstPendingMessage": channel.Messages[0].Text,                                                         // Preview of the first unread message
		"PendingCount":        len(channel.Messages),                                                            // Total number of unread messages
		"OpenChatLink":        fmt.Sprintf("%s/channel/%s", constants.TRENDLY_CREATORS_FE, channel.Channel.CID), // Link to open the chat
	}

	err = myemail.SendCustomHTMLEmail(*user.Email, templates.MessageReminder, templates.SubjectUnreadMessageReminder, data)
	if err != nil {
		fmt.Printf("Error sending email to user %s: %s\n", *user.Email, err.Error())
		return
	}

}
