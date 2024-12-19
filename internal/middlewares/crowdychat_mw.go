package middlewares

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// Make the debug mode true to bypass the firebase token verification through id token method
// In Production, it should be false
const DEBUG_MODE = true

func GetUserId(c *gin.Context) (string, bool) {
	uId, exists := c.Get("firebaseUID")
	if exists {
		return uId.(string), exists
	}
	return "", exists
	// return
}

// Validate UID
func isValidUID(client *auth.Client, uid string) bool {
	// Try to get user by UID
	_, err := client.GetUser(context.Background(), uid)
	if err != nil {
		if auth.IsUserNotFound(err) {
			_, err := firestoredb.Client.Collection("users").Doc(uid).Get(context.Background())
			return err == nil
		}
		return false // Some other error
	}

	return true // UID is valid
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
		token, err := fauth.Client.VerifyIDToken(context.Background(), idToken)
		if err != nil {
			if DEBUG_MODE && isValidUID(fauth.Client, idToken) {
				token = &auth.Token{
					UID: idToken,
				}
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid ID token"})
				c.Abort()
				return
			}
		}
		// token := auth.Token{
		// 	UID: idToken,
		// }

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
	_, err := firestoredb.Client.Collection(fmt.Sprintf("organizations/%s/members", orgID)).Doc(userUID).Get(context.Background())
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
			c.JSON(http.StatusForbidden, gin.H{"error": "User does not belong to the specified organization", "user": firebaseUID, "org": orgID})
			c.Abort()
			return
		}

		c.Set("organizationID", orgID)

		// If all headers are valid, proceed to the next handler
		c.Next()
	}
}
