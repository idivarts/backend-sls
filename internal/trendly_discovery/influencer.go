package trendlydiscovery

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// If the influencer has practive to hide the views likes and comments ->

func calculateTrustablity(social *trendlybq.Socials) int {

	return 100
}

func calculateBudget(social *trendlybq.Socials) Range {

	return Range{}
}

func calculateReach(social *trendlybq.Socials) Range {

	return Range{}
}

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

	type CalculatedData struct {
		Quality         int   `json:"quality"`
		Trustablity     int   `json:"trustablity"`
		EstimatedBudget Range `json:"estimatedBudget"`
		EstimatedReach  Range `json:"estimatedReach"`
	}

	calculatedValue := CalculatedData{
		Quality:         social.QualityScore,
		Trustablity:     calculateTrustablity(social),
		EstimatedBudget: calculateBudget(social),
		EstimatedReach:  calculateReach(social),
	}

	c.JSON(http.StatusOK, gin.H{"message": "Fetched influencer", "social": social, "analysis": calculatedValue})
}

func RequestConnection(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "api is functional"})
}
