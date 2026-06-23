package socialwebhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// DataDeletion implements Meta's Data Deletion Request Callback. Meta POSTs a
// `signed_request` (a Facebook-signed payload) and expects a JSON body with a
// status URL and a confirmation code. Required for App Review of any app that
// handles user data.
//
// POST /webhooks/data-deletion   (Content-Type: application/x-www-form-urlencoded)
func DataDeletion(c *gin.Context) {
	signed := c.PostForm("signed_request")
	if signed == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing signed_request"})
		return
	}

	payload, err := parseSignedRequest(signed)
	if err != nil {
		log.Printf("social_webhooks/data_deletion: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signed_request"})
		return
	}

	userID, _ := payload["user_id"].(string)
	log.Printf("social_webhooks/data_deletion: deletion request for platform user %s", userID)

	// Erase synchronously before acknowledging. A background goroutine would be
	// unreliable on Lambda (the environment can freeze after the response), and
	// the per-user volume is small, so we complete the purge in-request.
	purgeUserData(userID)

	code := userID
	if code == "" {
		code = "deletion"
	}
	statusURL := fmt.Sprintf("%s/webhooks/data-deletion/status?code=%s", constants.GetTrendlyBE(), code)

	c.JSON(http.StatusOK, gin.H{
		"url":               statusURL,
		"confirmation_code": code,
	})
}

// DataDeletionStatus is the human-visible status page Meta links the user to.
// GET /webhooks/data-deletion/status?code=...
func DataDeletionStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":   c.Query("code"),
		"status": "Your data deletion request has been received and processed.",
	})
}

// parseSignedRequest validates and decodes a Facebook signed_request.
func parseSignedRequest(signed string) (map[string]interface{}, error) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed signed_request")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("bad signature encoding: %w", err)
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("bad payload encoding: %w", err)
	}

	// Verify the HMAC-SHA256 signature against any configured app secret.
	verified := false
	for _, secret := range metaAppSecrets() {
		if secret == "" {
			continue
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(parts[1]))
		if hmac.Equal(mac.Sum(nil), sig) {
			verified = true
			break
		}
	}
	if !verified {
		return nil, fmt.Errorf("signature mismatch")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("bad payload json: %w", err)
	}
	return payload, nil
}

// purgeUserData erases all inbox data associated with a platform user id. The
// participant id equals the platform user id; a collection-group query deletes
// every DM/comment conversation that user appears in, across all brands.
func purgeUserData(userID string) {
	if userID == "" {
		return
	}
	n, err := trendlymodels.DeleteInboxConversationsByParticipant(userID)
	if err != nil {
		log.Printf("social_webhooks/data_deletion: purge for %s failed after %d deletes: %v", userID, n, err)
		return
	}
	log.Printf("social_webhooks/data_deletion: purged %d inbox conversation(s) for platform user %s", n, userID)
}
