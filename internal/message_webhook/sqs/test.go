package sqsapp

import (
	"net/http"

	snshandler "github.com/TrendsHub/th-backend/pkg/sns_handler"
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

	snshandler.Send(testMesage.Message, testMesage.Delay)
	c.JSON(http.StatusOK, gin.H{
		"message": testMesage,
	})
}
