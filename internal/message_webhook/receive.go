package messagewebhook

import (
	"encoding/json"
	"log"
	"net/http"

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

	log.Printf("Received Message of Type %s", instainterfaces.CalcualateMessageType(&message.Entry[0].Messaging[0]))
	log.Println("Complete Message", message.Entry[0].Messaging[0].Message.Text)

	data, err := json.Marshal(&message)
	if err == nil {
		log.Println("Complete Message After Marshall", string(data))
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Instagram webhook received successfully",
		"instagram": message,
	})
}
