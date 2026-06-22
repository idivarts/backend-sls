package inbox

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/socialsync"
)

// All unit-level resync handlers queue the work (return 202) and let the client's
// Firestore listener surface the refreshed item. Gated by the same inbox privilege.

// POST /api/v2/brands/:brandId/inbox/conversations/:id/resync-profile
func ResyncConversationProfile(c *gin.Context) {
	brandID := c.Param("brandId")
	convID := c.Param("id")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	err := enqueueOrRunMsg(
		socialsync.Message{Type: socialsync.OpProfileResync, BrandID: brandID, ConversationID: convID},
		func() error { return ResyncProfile(brandID, convID) },
	)
	if err != nil {
		log.Printf("inbox: profile resync failed for %s/%s: %v", brandID, convID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue profile resync"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}

// POST /api/v2/brands/:brandId/inbox/conversations/:id/resync
func ResyncConversationThread(c *gin.Context) {
	brandID := c.Param("brandId")
	convID := c.Param("id")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	err := enqueueOrRunMsg(
		socialsync.Message{Type: socialsync.OpThreadResync, BrandID: brandID, ConversationID: convID},
		func() error { return ResyncThread(brandID, convID) },
	)
	if err != nil {
		log.Printf("inbox: thread resync failed for %s/%s: %v", brandID, convID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue thread resync"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}

// POST /api/v2/brands/:brandId/inbox/conversations/:id/messages/:msgId/resync
func ResyncConversationMessage(c *gin.Context) {
	brandID := c.Param("brandId")
	convID := c.Param("id")
	msgID := c.Param("msgId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	err := enqueueOrRunMsg(
		socialsync.Message{Type: socialsync.OpMessageResync, BrandID: brandID, ConversationID: convID, MessageID: msgID},
		func() error { return ResyncMessage(brandID, convID, msgID) },
	)
	if err != nil {
		log.Printf("inbox: message resync failed for %s/%s/%s: %v", brandID, convID, msgID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue message resync"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}

// POST /api/v2/brands/:brandId/inbox/media/:mediaId/resync?socialId=&channel=
func ResyncMedia(c *gin.Context) {
	brandID := c.Param("brandId")
	mediaID := c.Param("mediaId")
	socialID := c.Query("socialId")
	channel := c.Query("channel")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureSocialAccounts, trendlymodels.PrivSocialInbox); !ok {
		return
	}
	if socialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "socialId is required"})
		return
	}
	err := enqueueOrRunMsg(
		socialsync.Message{Type: socialsync.OpMediaResync, BrandID: brandID, MediaID: mediaID, SocialID: socialID, Channel: channel},
		func() error { return ResyncMediaItem(brandID, mediaID, socialID, channel) },
	)
	if err != nil {
		log.Printf("inbox: media resync failed for %s/%s: %v", brandID, mediaID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue media resync"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}
