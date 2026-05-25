package social_connect

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ListSocials returns all connected social accounts (V2) for the authenticated user.
// GET /api/v2/socials/v2
func ListSocials(c *gin.Context) {
	userId, ok := middlewares.GetUserId(c)
	if !ok || userId == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	ctx := context.Background()
	docs, err := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialsV2", userId)).
		Documents(ctx).
		GetAll()
	if err != nil {
		log.Printf("listSocials: firestore query failed for %s: %v", userId, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch socials"})
		return
	}

	socials := make([]trendlymodels.SocialV2, 0, len(docs))
	for _, doc := range docs {
		var s trendlymodels.SocialV2
		if err := doc.DataTo(&s); err != nil {
			log.Printf("listSocials: failed to decode doc %s: %v", doc.Ref.ID, err)
			continue
		}
		socials = append(socials, s)
	}

	c.JSON(http.StatusOK, gin.H{"socials": socials})
}

// DeleteSocial removes a connected social account (V2) for the authenticated user.
// DELETE /api/v2/socials/v2/:id
func DeleteSocial(c *gin.Context) {
	userId, ok := middlewares.GetUserId(c)
	socialID := c.Param("id")

	if !ok || userId == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if socialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "social ID is required"})
		return
	}

	ctx := context.Background()

	pubRef := firestoredb.Client.Collection(fmt.Sprintf("users/%s/socialsV2", userId)).Doc(socialID)
	privRef := firestoredb.Client.Collection(fmt.Sprintf("users/%s/socialsV2Private", userId)).Doc(socialID)

	batch := firestoredb.Client.Batch()
	batch.Delete(pubRef)
	batch.Delete(privRef)

	if _, err := batch.Commit(ctx); err != nil {
		log.Printf("deleteSocial: failed to delete %s/%s: %v", userId, socialID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete social"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "social account disconnected"})
}
