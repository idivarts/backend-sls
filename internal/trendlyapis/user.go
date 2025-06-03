package trendlyapis

import (
	"context"
	"net/http"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	"github.com/idivarts/backend-sls/pkg/streamchat"
)

func userPreChecks(user *trendlymodels.User) bool {
	if user == nil {
		return false
	}
	if user.Settings == nil {
		user.Settings = &trendlymodels.UserSettings{}
	}
	if user.Settings.AccountStatus == nil {
		user.Settings.AccountStatus = aws.String("Activated")
	}
	if *user.Settings.AccountStatus != "Activated" {
		return false
	}

	// Add all prechecks like all contracts should be in closed state
	return true
}

func DeativateUser(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not authenticated", "message": "UserId not found"})
		return
	}

	user := trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "User not found"})
		return
	}

	if !userPreChecks(&user) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User cannot be deleted", "message": "User has active collaborations or contracts"})
		return
	}

	if user.Settings == nil {
		user.Settings = &trendlymodels.UserSettings{
			AccountStatus: aws.String("Deactivated"),
		}
	} else {
		user.Settings.AccountStatus = aws.String("Deactivated")
	}

	_, err = user.Insert(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error setting user"})
		return
	}

	err = fauth.Client.RevokeRefreshTokens(context.Background(), userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error revoking the user session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully deactivated the user"})
}

func DeleteUser(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not authenticated", "message": "UserId not found"})
		return
	}

	user := trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "User not found"})
		return
	}

	if !userPreChecks(&user) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User cannot be deleted", "message": "User has active collaborations or contracts"})
		return
	}

	if user.Settings == nil {
		user.Settings = &trendlymodels.UserSettings{
			AccountStatus: aws.String("Deleted"),
		}
	} else {
		user.Settings.AccountStatus = aws.String("Deleted")
	}

	_, err = user.Insert(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error setting user"})
		return
	}

	err = fauth.Client.DeleteUser(context.Background(), userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error removing user authentication"})
		return
	}

	_, err = streamchat.Client.DeleteUser(context.Background(), userId, stream_chat.DeleteUserWithHardDelete())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error removing user on Stream"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully deactivated the user"})
}
