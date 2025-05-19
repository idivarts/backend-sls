package trendlyapis

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

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

	c.JSON(http.StatusOK, gin.H{"message": "Successfully deactivated the user"})
}
