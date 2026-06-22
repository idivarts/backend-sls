package inbox

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/socialsync"
)

// queryCount parses an optional ?count= query param (<=0 → 0, let the service default).
func queryCount(c *gin.Context) int {
	n, err := strconv.Atoi(c.Query("count"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// POST/GET /api/v2/brands/:brandId/inbox/media
// Queues a background refresh of the brand's published posts/reels and returns
// immediately. The worker upserts each item to Firestore (brands/{brandId}/
// inboxMedia); the client observes them live via its Firestore listener.
func GetMediaList(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	if err := enqueueOrRun(brandID, socialsync.OpMedia); err != nil {
		log.Printf("inbox media: enqueue refresh failed for %s: %v", brandID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue media refresh"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}

// GET /api/v2/brands/:brandId/inbox/media/:mediaId/comments?socialId=&channel=
// Lists the top-level comments on a single piece of media.
func GetMediaComments(c *gin.Context) {
	brandID := c.Param("brandId")
	mediaID := c.Param("mediaId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	socialID := c.Query("socialId")
	if socialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "socialId is required"})
		return
	}
	channel, err := channelOrDefault(c.Query("channel"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comments, err := ListMediaComments(brandID, socialID, channel, mediaID, queryCount(c))
	if err != nil {
		log.Printf("inbox media: comments failed for %s/%s: %v", brandID, mediaID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load comments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"comments": comments})
}

type mediaCommentReplyRequest struct {
	SocialID string `json:"socialId" binding:"required"`
	Channel  string `json:"channel"`
	Text     string `json:"text" binding:"required"`
}

// POST /api/v2/brands/:brandId/inbox/comments/:commentId/reply
func ReplyToMediaCommentHandler(c *gin.Context) {
	brandID := c.Param("brandId")
	commentID := c.Param("commentId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	var req mediaCommentReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	channel, err := channelOrDefault(req.Channel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ReplyToMediaComment(brandID, req.SocialID, channel, commentID, req.Text); err != nil {
		log.Printf("inbox media: reply failed for %s/%s: %v", brandID, commentID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to send reply"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sent"})
}

type mediaCommentHideRequest struct {
	SocialID string `json:"socialId" binding:"required"`
	Channel  string `json:"channel"`
	Hidden   bool   `json:"hidden"`
}

// POST /api/v2/brands/:brandId/inbox/comments/:commentId/hide
func HideMediaCommentHandler(c *gin.Context) {
	brandID := c.Param("brandId")
	commentID := c.Param("commentId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	var req mediaCommentHideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	channel, err := channelOrDefault(req.Channel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := SetMediaCommentHidden(brandID, req.SocialID, channel, commentID, req.Hidden); err != nil {
		log.Printf("inbox media: hide failed for %s/%s: %v", brandID, commentID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to update comment"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// DELETE /api/v2/brands/:brandId/inbox/comments/:commentId?socialId=&channel=
func DeleteMediaCommentHandler(c *gin.Context) {
	brandID := c.Param("brandId")
	commentID := c.Param("commentId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	socialID := c.Query("socialId")
	if socialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "socialId is required"})
		return
	}
	channel, err := channelOrDefault(c.Query("channel"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := DeleteMediaComment(brandID, socialID, channel, commentID); err != nil {
		log.Printf("inbox media: delete failed for %s/%s: %v", brandID, commentID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete comment"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
