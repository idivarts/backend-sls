package middlewares

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

func GetUserType(c *gin.Context) string {
	return c.GetString("userType")
}
func GetUserObject(c *gin.Context) map[string]interface{} {
	if GetUserType(c) == "user" {
		return c.MustGet("user").(map[string]interface{})
	}
	return c.MustGet("manager").(map[string]interface{})
}

func TrendlyMiddleware(model string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId, b := GetUserId(c)
		if !b {
			c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
			return
		}

		if model == "common" {
			user, err := firestoredb.Client.Collection("users").Doc(userId).Get(context.Background())
			if err != nil {
				manager, err := firestoredb.Client.Collection("managers").Doc(userId).Get(context.Background())
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "User not found in User nor Manager Databse"})
					return
				}
				c.Set("userType", "manager")
				c.Set("manager", manager.Data())
			} else {
				c.Set("userType", "user")
				c.Set("user", user.Data())
			}
		} else {
			user, err := firestoredb.Client.Collection(model).Doc(userId).Get(context.Background())
			if err != nil {
				if model == "managers" {
					c.Set("userType", "manager")
					c.Set("manager", user.Data())
				} else {
					c.Set("userType", "user")
					c.Set("user", user.Data())
				}
			}
		}

		// Continue to the next handler
		c.Next()
	}
}
