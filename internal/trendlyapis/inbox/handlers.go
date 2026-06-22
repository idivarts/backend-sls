package inbox

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// GET /api/v2/brands/:brandId/inbox
// Returns connected Meta accounts + unified conversations (read-through cached).
func GetInbox(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}

	accounts, err := ListAccounts(brandID)
	if err != nil {
		log.Printf("inbox: list accounts failed for %s: %v", brandID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load accounts"})
		return
	}

	// Optional server-side filter (?filter=unread|dm|comment|instagram|facebook).
	// Omitted → returns all (default; the web client filters client-side).
	filter := c.Query("filter")
	conversations, err := GetConversationsFiltered(brandID, filter)
	if err != nil {
		log.Printf("inbox: list conversations failed for %s: %v", brandID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connectedAccounts": accounts,
		"conversations":     conversations,
		"unreadTotal":       UnreadCount(brandID),
	})
}

// POST /api/v2/brands/:brandId/inbox/sync
// Forces a fresh read-through sync from Meta, then returns conversations.
func SyncInbox(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	if err := SyncFromMeta(brandID); err != nil {
		log.Printf("inbox: manual sync failed for %s: %v", brandID, err)
		// Non-fatal: still return whatever is cached.
	}
	conversations, err := ListConversations(brandID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load conversations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversations": conversations})
}

// POST /api/v2/brands/:brandId/inbox/resync
// Clears the brand's cached DM conversations and re-pulls them fresh from Meta so
// participant names/avatars are rebuilt and stale/duplicate DM docs are dropped.
// Comments are left intact. Repair/dev tool.
func ResyncInbox(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	conversations, err := ResyncFromMeta(brandID)
	if err != nil {
		log.Printf("inbox: resync failed for %s: %v", brandID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resync inbox"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversations": conversations})
}

type replyRequest struct {
	Text string `json:"text" binding:"required"`
}

// POST /api/v2/brands/:brandId/inbox/conversations/:id/reply
func ReplyToConversation(c *gin.Context) {
	brandID := c.Param("brandId")
	convID := c.Param("id")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	var req replyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Reply(brandID, convID, req.Text); err != nil {
		log.Printf("inbox: reply failed for %s/%s: %v", brandID, convID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to send reply"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sent"})
}

type hideRequest struct {
	Hidden bool `json:"hidden"`
}

// POST /api/v2/brands/:brandId/inbox/conversations/:id/hide
func HideComment(c *gin.Context) {
	brandID := c.Param("brandId")
	convID := c.Param("id")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	var req hideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := SetCommentHidden(brandID, convID, req.Hidden); err != nil {
		log.Printf("inbox: hide failed for %s/%s: %v", brandID, convID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to update comment"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// DELETE /api/v2/brands/:brandId/inbox/conversations/:id
func DeleteConversation(c *gin.Context) {
	brandID := c.Param("brandId")
	convID := c.Param("id")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	if err := DeleteComment(brandID, convID); err != nil {
		log.Printf("inbox: delete failed for %s/%s: %v", brandID, convID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete comment"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// POST /api/v2/brands/:brandId/inbox/conversations/:id/read
func ReadConversation(c *gin.Context) {
	brandID := c.Param("brandId")
	convID := c.Param("id")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	if err := MarkRead(brandID, convID); err != nil {
		log.Printf("inbox: mark read failed for %s/%s: %v", brandID, convID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark read"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
