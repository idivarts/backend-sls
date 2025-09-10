package trendlydiscovery

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

// Make sure the the influencers discovery credit is reduced
// If the influencer is already fetched before, do not reduce the credit
// Also make sure the influencer is added uniquely to the user's list of influencers
func FetchInfluencer(c *gin.Context) {
	influencerId := c.Param("influencerId")
	if influencerId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Influencer Id missing", "error": "influencer-id-missing"})
	}

	social := &trendlybq.Socials{}

	err := social.Get(influencerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Cant fetch", "error": err.Error()})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Fetched influencer", "social": social})
}

func RequestConnection(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "api is functional"})
}
