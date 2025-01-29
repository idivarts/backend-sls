package trendlyapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Notification struct {
	UserID    []string    `json:"userId"`
	ManagerID []string    `json:"managerId"`
	Payload   interface{} `json:"payload"`
}

func Notify(c *gin.Context) {
	req := Notification{}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid Request"})
		return
	}

	// Send Notification
	c.JSON(http.StatusOK, gin.H{"message": "Notification Sent"})
}
