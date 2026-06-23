package social_connect

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// ListBrandSocials returns all connected social accounts for a brand.
// GET /api/v2/brands/:brandId/socials
func ListBrandSocials(c *gin.Context) {
	brandID := c.Param("brandId")
	if brandID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId is required"})
		return
	}

	socials, err := trendlymodels.ListBrandSocialAccounts(brandID)
	if err != nil {
		log.Printf("listBrandSocials: failed for %s: %v", brandID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch brand socials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"socials": socials})
}

// DeleteBrandSocial removes a connected social account from a brand.
// DELETE /api/v2/brands/:brandId/socials/:id
func DeleteBrandSocial(c *gin.Context) {
	brandID := c.Param("brandId")
	socialID := c.Param("id")

	if brandID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId is required"})
		return
	}
	if socialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "social ID is required"})
		return
	}

	// Read the account first so we can clean up its webhook-routing index
	// entries (page id and any linked IG business id). Best-effort.
	account, getErr := trendlymodels.GetBrandSocialAccount(brandID, socialID)

	if err := trendlymodels.DeleteBrandSocialAccount(brandID, socialID); err != nil {
		log.Printf("deleteBrandSocial: failed to delete %s/%s: %v", brandID, socialID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete brand social"})
		return
	}

	// Remove only THIS brand's ownership from the routing index — other brands
	// connected to the same account must keep receiving webhooks. The index doc
	// is deleted only when its last owner is removed.
	if getErr == nil && account != nil {
		if account.PlatformAccountID != "" {
			_ = trendlymodels.RemoveSocialAccountOwner(account.PlatformAccountID, "brands", brandID)
		}
		if account.InstagramBusinessID != "" {
			_ = trendlymodels.RemoveSocialAccountOwner(account.InstagramBusinessID, "brands", brandID)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "brand social account disconnected"})
}
