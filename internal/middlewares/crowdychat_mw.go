package middlewares

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/TrendsHub/th-backend/pkg/firebase/auth"
	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
	"github.com/gin-gonic/gin"
)

func GetUserId(c *gin.Context) (string, bool) {
	uId, exists := c.Get("firebaseUID")
	if exists {
		return uId.(string), exists
	}
	return "", exists
	// return
}

// ValidateSessionMiddleware checks for required headers and validates them
func ValidateSessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is missing"})
			c.Abort()
			return
		}

		idToken := strings.TrimPrefix(authHeader, "Bearer ")
		if idToken == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
			c.Abort()
			return
		}

		// Verify the token with Firebase Admin SDK
		token, err := auth.Client.VerifyIDToken(context.Background(), idToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid ID token"})
			c.Abort()
			return
		}

		// Token is valid; set user info in Gin context for use in handlers
		c.Set("firebaseUID", token.UID)

		// Continue to the next handler
		c.Next()
	}
}

func GetOrganizationId(c *gin.Context) (string, bool) {
	orgId, exists := c.Get("organizationID")
	if exists {
		return orgId.(string), exists
	}
	return "", exists
	// return
}

// validateUserOrganization checks if the user belongs to the specified organization.
func validateUserOrganization(userUID, orgID string) bool {
	_, err := firestoredb.Client.Collection(fmt.Sprintf("/organizations/%s/members", orgID)).Doc(userUID).Get(context.Background())
	return err == nil
}

// ValidateSessionMiddleware checks for required headers and validates them
func ValidateOrganizationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetHeader("X-Organization-ID")
		if orgID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-Organization-ID header is missing"})
			c.Abort()
			return
		}

		// Validate if the user belongs to the specified organization
		firebaseUID, b := GetUserId(c)
		if !b {
			c.JSON(http.StatusBadRequest, gin.H{"error": "User doesnt have a valid session"})
			c.Abort()
			return
		}

		if !validateUserOrganization(firebaseUID, orgID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "User does not belong to the specified organization"})
			c.Abort()
			return
		}

		c.Set("organizationID", orgID)

		// If all headers are valid, proceed to the next handler
		c.Next()
	}
}
