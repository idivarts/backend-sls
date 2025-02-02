package trendlyapis

import (
	"context"
	"net/http"

	"firebase.google.com/go/v4/messaging"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/firebase/fmessaging"
)

type Notification struct {
	UserID    []string `json:"userId"`
	ManagerID []string `json:"managerId"`
	Payload   struct {
		Data         map[string]string        `json:"data,omitempty"`
		Notification *messaging.Notification  `json:"notification"`
		Android      *messaging.AndroidConfig `json:"android,omitempty"`
		Webpush      *messaging.WebpushConfig `json:"webpush,omitempty"`
		APNS         *messaging.APNSConfig    `json:"apns,omitempty"`
		FCMOptions   *messaging.FCMOptions    `json:"fcmOptions,omitempty"`
	} `json:"payload"`
}

func Notify(c *gin.Context) {
	req := Notification{}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid Request"})
		return
	}

	fmessaging.Client.SendEachForMulticast(context.Background(), &messaging.MulticastMessage{
		Tokens:       []string{},
		Data:         req.Payload.Data,
		Notification: req.Payload.Notification,
		Android:      req.Payload.Android,
		Webpush:      req.Payload.Webpush,
		APNS:         req.Payload.APNS,
		FCMOptions:   req.Payload.FCMOptions,
	})
	// Send Notification
	c.JSON(http.StatusOK, gin.H{"message": "Notification Sent"})
}
