package social_connect

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// ListSocials returns all connected social accounts for the authenticated user.
// GET /api/v2/socials/v2
func ListSocials(c *gin.Context) {
	userId, ok := middlewares.GetUserId(c)
	if !ok || userId == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	socials, err := trendlymodels.ListSocialAccounts(userId)
	if err != nil {
		log.Printf("listSocials: failed for %s: %v", userId, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch socials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"socials": socials})
}

// DeleteSocial removes a connected social account for the authenticated user.
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

	if err := trendlymodels.DeleteSocialAccount(userId, socialID); err != nil {
		log.Printf("deleteSocial: failed to delete %s/%s: %v", userId, socialID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete social"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "social account disconnected"})
}
