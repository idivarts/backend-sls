package sqsapp

import (
	"context"
	"net/http"

	delayedsqs "github.com/TrendsHub/th-backend/pkg/delayed_sqs"
	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
	"github.com/gin-gonic/gin"
)

type testSQSMessage struct {
	Message string `form:"message" json:"message" binding:"required"`
	Delay   int64  `form:"delay" json:"delay" binding:"required"`
}

func SendTestSQSMessage(c *gin.Context) {
	var testMesage testSQSMessage
	if err := c.ShouldBindQuery(&testMesage); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, _, err := firestoredb.Client.Collection("testMessages").Add(context.Background(), map[string]interface{}{
		"message": testMesage.Message,
		"delay":   testMesage.Delay,
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delayedsqs.Send(testMesage.Message, testMesage.Delay)
	c.JSON(http.StatusOK, gin.H{
		"message": testMesage,
	})
}
