package trendlyapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/pkg/streamchat"
)

func ChatAuth(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not found"})
		return
	}

	isManager := false
	if middlewares.GetUserType(c) == "manager" {
		isManager = true
	}

	userObject := middlewares.GetUserObject(c)

	// Upsert user to the stream chat
	_, err := streamchat.CreateOrUpdateUser(streamchat.User{
		ID:        userId,
		Name:      userObject["name"].(string),
		Image:     userObject["profileImage"].(string),
		IsManager: isManager,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating/updating user in chat", "error": err.Error()})
		return
	}
	token := ""
	if userObject["isChatConnected"] == true {
		t, err := streamchat.CreateToken(userId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating token", "error": err.Error()})
			return
		}
		token = t
	}

	c.JSON(http.StatusOK, gin.H{"message": "Chat Authentication successful", "token": token})
}

func ChatConnect(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not found"})
		return
	}

	userObject := middlewares.GetUserObject(c)

	if userObject["isChatConnected"] == false {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Chat not connected"})
		return
	}

	token, err := streamchat.CreateToken(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating token", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Chat Connected", "token": token})
}

func ChatChannel(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Chat Channel"})
}
