package matchmaking

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
)

type IBrandMember struct {
	BrandID string `form:"brandId" binding:"required"`
}

type ExploreInfluencerCache struct {
	Time int64    `json:"time" firestore:"time"`
	IDs  []string `json:"ids" firestore:"ids"`
}

func GetInfluencers(c *gin.Context) {
	var req IBrandMember
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Input is incorrect"})
		return
	}

	managerId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not authenticated", "message": "UserId not found"})
		return
	}

	membership := trendlymodels.BrandMember{}
	err := membership.Get(req.BrandID, managerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Brand Membership not found"})
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Brand not found"})
		return
	}

	ids, err := RunBQ(brand.Preferences)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error Executing Query"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Succesfully fetched data", "data": ids})
}
func RunBQ(preference *trendlymodels.BrandPreferences) ([]string, error) {
	locations := []string{}
	categories := []string{}
	languages := []string{}
	if preference != nil {
		locations = preference.Locations
		categories = preference.InfluencerCategories
		languages = preference.Languages
	}

	influencersModel := trendlyrdb.Influencers{}
	return influencersModel.GetExploreInfluencerIDs(locations, categories, languages, 100)
}
