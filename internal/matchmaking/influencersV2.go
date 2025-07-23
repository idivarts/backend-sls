package matchmaking

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func GetInfluencerForInfluencer(c *gin.Context) {
	influencers, err := trendlymodels.GetInfluencerIDs(nil, 100)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Influencers not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": "Influencers found", "influencers": influencers})
}
