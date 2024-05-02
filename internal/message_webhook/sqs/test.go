package sqsapp

import (
	"net/http"

	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
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

	sqshandler.SendToMessageQueue(testMesage.Message, testMesage.Delay)
	c.JSON(http.StatusOK, gin.H{
		"message": testMesage,
	})
}
