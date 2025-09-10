package trendlydiscovery

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Make sure the the influencers discovery credit is reduced
// If the influencer is already fetched before, do not reduce the credit
// Also make sure the influencer is added uniquely to the user's list of influencers
func FetchInfluencer(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "api is functional"})
}

func RequestConnection(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "api is functional"})
}
