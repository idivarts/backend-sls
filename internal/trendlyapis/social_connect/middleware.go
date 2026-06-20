package social_connect

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

// ValidateQueryTokenMiddleware validates a Firebase ID token passed as the
// `?token=` query parameter. This is used for the OAuth init routes that are
// navigated to by the connect portal (a static web page that cannot set
// Authorization headers on a browser redirect).
//
// On success it sets "firebaseUID" in the Gin context — identical to what
// ValidateSessionMiddleware sets — so handlers can use middlewares.GetUserId().
func ValidateQueryTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		idToken := c.Query("token")
		if idToken == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token query parameter is required"})
			return
		}

		token, err := fauth.Client.VerifyIDToken(context.Background(), idToken)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set("firebaseUID", token.UID)
		c.Next()
	}
}

