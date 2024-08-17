package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetUserId(c *gin.Context) (string, bool) {
	uId, exists := c.Get("userId")
	if exists {
		return uId.(string), exists
	}
	return "", exists
	// return
}

// ValidateSessionMiddleware checks for required headers and validates them
func ValidateSessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for the Authorization header
		authHeader := c.GetHeader("token")
		if authHeader == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Authorization header is missing",
			})
			c.Abort()
			return
		}
		c.Set("userId", "")

		// If all headers are valid, proceed to the next handler
		c.Next()
	}
}

func GetOrganizationId(c *gin.Context) (string, bool) {
	orgId, exists := c.Get("organizationId")
	if exists {
		return orgId.(string), exists
	}
	return "", exists
	// return
}

// ValidateSessionMiddleware checks for required headers and validates them
func ValidateOrganizationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for the Authorization header
		authHeader := c.GetHeader("organizationToken")
		if authHeader == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Authorization header is missing",
			})
			c.Abort()
			return
		}
		c.Set("organizationId", "")
		// If all headers are valid, proceed to the next handler
		c.Next()
	}
}
