package middlewares

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ClientPlatformHeader carries which client the request came from. The apps set
// it centrally in HttpWrapper.fetch (shared-libs/utils/http-wrapper.ts).
const ClientPlatformHeader = "X-Client-Platform"

// allowedClientPlatforms whitelists the values we will persist. The header is
// client-controlled, so anything unrecognised is dropped rather than written
// through to Firestore.
var allowedClientPlatforms = map[string]bool{
	"ios":         true,
	"android":     true,
	"web-desktop": true,
	"web-mobile":  true,
}

func normalizeClientPlatform(raw string) string {
	platform := strings.ToLower(strings.TrimSpace(raw))
	if !allowedClientPlatforms[platform] {
		return ""
	}
	return platform
}

// touchManagerLastSeen records which client this manager is using, so the admin
// Brand CRM can profile how customers actually reach the product.
//
// Runs on every authenticated manager request, so it is throttled hard: the
// manager document is already loaded here, which makes the staleness check free
// and limits writes to roughly one per manager per hour.
func touchManagerLastSeen(c *gin.Context, managerID string, data map[string]interface{}) {
	platform := normalizeClientPlatform(c.GetHeader(ClientPlatformHeader))
	if platform == "" {
		return
	}

	var stored trendlymodels.Manager
	stored.LastSeenPlatform, _ = data["lastSeenPlatform"].(string)
	stored.LastSeenAt, _ = data["lastSeenAt"].(int64)

	now := time.Now().UnixMilli()
	if !stored.ShouldRefreshLastSeen(platform, now) {
		return
	}

	// Synchronous on purpose: Lambda freezes the execution environment once the
	// response is written, so a background goroutine here may never run. Errors
	// are logged and swallowed — telemetry must never fail a user's request.
	if err := trendlymodels.TouchManagerLastSeen(c.Request.Context(), managerID, platform, now); err != nil {
		log.Printf("[lastSeen] manager=%s platform=%s: %v", managerID, platform, err)
	}
}

func GetUserType(c *gin.Context) string {
	return c.GetString("userType")
}
func GetUserObject(c *gin.Context) map[string]interface{} {
	if GetUserType(c) == "user" {
		return c.MustGet("user").(map[string]interface{})
	}
	return c.MustGet("manager").(map[string]interface{})
}

func GetUserModel(c *gin.Context) trendlymodels.User {
	userMap := c.MustGet("user").(map[string]interface{})
	jsonData, _ := json.Marshal(userMap)
	var user trendlymodels.User
	json.Unmarshal(jsonData, &user)
	return user
}

func GetManagerModel(c *gin.Context) trendlymodels.Manager {
	managerMap := c.MustGet("manager").(map[string]interface{})
	jsonData, _ := json.Marshal(managerMap)
	var manager trendlymodels.Manager
	json.Unmarshal(jsonData, &manager)
	return manager
}

func TrendlyMiddleware(model string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId, b := GetUserId(c)
		if !b {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "User not found"})
			return
		}

		if model == "common" {
			user, err := firestoredb.Client.Collection("users").Doc(userId).Get(context.Background())
			if err != nil {
				manager, err := firestoredb.Client.Collection("managers").Doc(userId).Get(context.Background())
				if err != nil {
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "User not found in User nor Manager Databse"})
					return
				}
				c.Set("userType", "manager")
				c.Set("manager", manager.Data())
				touchManagerLastSeen(c, userId, manager.Data())
			} else {
				c.Set("userType", "user")
				c.Set("user", user.Data())
			}
		} else {
			user, err := firestoredb.Client.Collection(model).Doc(userId).Get(context.Background())
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "User not found in User nor Manager Databse"})
				return
			}
			if model == "managers" {
				c.Set("userType", "manager")
				c.Set("manager", user.Data())
				touchManagerLastSeen(c, userId, user.Data())
			} else {
				c.Set("userType", "user")
				c.Set("user", user.Data())
			}
		}

		// Continue to the next handler
		c.Next()
	}
}

func TrendlyExtension() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-USER-ID")
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "X-USER-ID header is missing"})
			return
		}
		c.Set("firebaseUID", userID)

		// Continue to the next handler
		c.Next()
	}
}
