package messagewebhook

import (
	"encoding/json"
	"log"
	"net/http"

	mwh_handler "github.com/TrendsHub/th-backend/internal/message_webhook/handler"
	instainterfaces "github.com/TrendsHub/th-backend/pkg/interfaces/instaInterfaces"
	"github.com/gin-gonic/gin"
)

func Receive(c *gin.Context) {
	var message instainterfaces.IMessageWebhook
	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		log.Println(err.Error())
		return
	}

	// Handle the Instagram message as needed
	// You can access message fields like message.Sender.ID, message.Message.Text, etc.

	// openai.SendMessage(message.Entry[0].Messaging[0].Sender.ID, message.Entry[0].Messaging[0].Message.Text)

	// log.Printf("Received Message of Type %s", instainterfaces.CalcualateMessageType(&message.Entry[0].Messaging[0]))
	// log.Println("Complete Message", message.Entry[0].Messaging[0].Message.Text)

	data, err := json.Marshal(&message)
	if err == nil {
		log.Println("Complete Message After Marshall", string(data))
	}

	for i := 0; i < len(message.Entry); i++ {
		pageId := message.Entry[i].ID
		for j := 0; j < len(message.Entry[i].Messaging); j++ {
			entry := &message.Entry[i].Messaging[j]
			// if pageId == entry.Sender.ID {
			// 	continue
			// }

			mType := instainterfaces.CalcualateMessageType(entry)
			if mType == instainterfaces.MessageTypeMessage {
				err = mwh_handler.IGMessagehandler{
					IGSID:   entry.Sender.ID,
					Message: entry.Message,
					PageID:  pageId,
					Entry:   entry,
					// ConversationID: ,
				}.HandleMessage()
				if err != nil {
					log.Println(err.Error())
				}
			}
			// else if mType == instainterfaces.MessageTypeRead {
			// 	err = mwh_handler.IGMessagehandler{
			// 		IGSID:  entry.Sender.ID,
			// 		Read:   entry.Read,
			// 		PageID: pageId,
			// 		// ConversationID: ,
			// 	}.HandleMessage()
			// 	if err != nil {
			// 		log.Println(err.Error())
			// 	}
			// }
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"message":   "Instagram webhook received successfully",
		"instagram": message,
	})
}
