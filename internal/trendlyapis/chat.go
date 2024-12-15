package trendlyapis

import (
	"context"
	"net/http"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
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

	if userObject["isChatConnected"] != true {
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

// ICreateChannel includes both json binding
type ICreateChannel struct {
	Name    *string  `json:"name,omitempty"`
	UserIDs []string `json:"userIds" binding:"required"`
}

func ChatChannel(c *gin.Context) {
	var req ICreateChannel
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request", "error": err.Error()})
		return
	}
	if len(req.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "At leat 1 user is requireed in the list"})
		return
	}

	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not found"})
		return
	}

	// Check if req.UserIDs contains userId
	contains := false
	for _, id := range req.UserIDs {
		if id == userId {
			contains = true
			break
		}
	}
	if contains {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Current user cannot be in the list of users"})
		return
	}

	req.UserIDs = append(req.UserIDs, userId)

	// Before creating channel make sure all users has isChatConnected true
	for _, id := range req.UserIDs {

		var uObj map[string]interface{}
		isManager := false
		user, err := firestoredb.Client.Collection("users").Doc(id).Get(context.Background())
		if err != nil {
			manager, err2 := firestoredb.Client.Collection("managers").Doc(id).Get(context.Background())
			if err2 != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Error in getting user and/or manager", "error1": err.Error(), "error2": err2.Error()})
				return
			}
			manager.DataTo(&uObj)
			isManager = true
		} else {
			user.DataTo(&uObj)
		}

		if uObj["isChatConnected"] == false {
			_, err := streamchat.CreateOrUpdateUser(streamchat.User{
				ID:        id,
				Name:      uObj["name"].(string),
				Image:     uObj["profileImage"].(string),
				IsManager: isManager,
			})
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating/updating user in chat", "error": err.Error()})
				return
			}
			uObj["isChatConnected"] = true
			if isManager {
				_, err := firestoredb.Client.Collection("managers").Doc(id).Set(context.Background(), uObj)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"message": "Error in updating manager", "error": err.Error()})
					return
				}
			} else {
				_, err := firestoredb.Client.Collection("users").Doc(id).Set(context.Background(), uObj)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"message": "Error in updating user", "error": err.Error()})
					return
				}
			}
		}
	}

	res, err := streamchat.Client.CreateChannel(context.Background(), "messaging", "", userId, &stream_chat.ChannelRequest{
		Members: req.UserIDs,
		ExtraData: map[string]interface{}{
			"name": req.Name,
			// "image": "https://getstream.io/random_svg/?id=chat-1&name=Chat",
		},
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating channel", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Channel Created", "channel": res.Channel})
}
