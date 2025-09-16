package trendlydiscovery

import (
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}
type CalculatedData struct {
	Quality         int   `json:"quality"`
	Trustablity     int   `json:"trustablity"`
	EstimatedBudget Range `json:"estimatedBudget"`
	EstimatedReach  Range `json:"estimatedReach"`
}

func calculateTrustablity(social *trendlybq.Socials) int {
	// Weights
	wEngagement := 0.30
	wRatios := 0.25
	wVerification := 0.20
	wActivity := 0.15
	wMisc := 0.10

	// 1) Engagement consistency (0–100)
	er := float64(social.EngagementRate) // e.g., 0.025 = 2.5%
	// Map ER into [0..100] with ideal band roughly 2%–6%
	var engagementScore float64
	if er <= 0.005 { // <=0.5%
		engagementScore = 10
	} else if er >= 0.08 { // >=8%
		engagementScore = 95
	} else {
		// Linear scale between 0.5% and 8%
		scaled := (er - 0.005) / (0.08 - 0.005)
		engagementScore = 10 + 85*clampFloat(scaled, 0, 1)
	}

	// 2) Ratio sanity checks (0–100)
	avgViews := float64(social.AverageViews)
	followers := float64(social.FollowerCount)
	avgLikes := float64(social.AverageLikes)
	avgComments := float64(social.AverageComments)

	vfr := safeDiv(avgViews, followers)                // views/followers
	ltv := safeDiv(avgLikes, avgViews)                 // likes/views
	ctl := safeDiv(avgComments, math.Max(avgLikes, 1)) // comments/likes

	// Score views-to-followers: ideal 0.2–0.8
	var vfrScore float64
	if vfr >= 0.2 && vfr <= 0.8 {
		vfrScore = 100
	} else if vfr < 0.2 {
		// 0.01→0, 0.2→100
		vfrScore = 100 * clampFloat((vfr-0.01)/(0.19), 0, 1)
	} else { // vfr > 0.8 up to 2.0
		vfrScore = 100 * (1 - clampFloat((vfr-0.8)/(1.2), 0, 1))
	}

	// Score likes-to-views: ideal 1%–10%
	var ltvScore float64
	if ltv >= 0.01 && ltv <= 0.10 {
		ltvScore = 100
	} else if ltv < 0.01 {
		ltvScore = 100 * clampFloat(ltv/0.01, 0, 1)
	} else {
		ltvScore = 100 * (1 - clampFloat((ltv-0.10)/0.20, 0, 1)) // decay after 10%
	}

	// Score comments-to-likes: ideal 2%–20%
	var ctlScore float64
	if ctl >= 0.02 && ctl <= 0.20 {
		ctlScore = 100
	} else if ctl < 0.02 {
		ctlScore = 100 * clampFloat(ctl/0.02, 0, 1)
	} else {
		ctlScore = 100 * (1 - clampFloat((ctl-0.20)/0.40, 0, 1))
	}

	ratiosScore := (vfrScore + ltvScore + ctlScore) / 3.0

	// 3) Verification & professionalism (0–100)
	verificationScore := 30.0
	if social.ProfileVerified {
		verificationScore += 35
	}
	if social.HasContacts {
		verificationScore += 35
	}
	verificationScore = clampFloat(verificationScore, 0, 100)

	// 4) Activity & freshness (0–100)
	var activityScore float64 = 50
	if social.LastUpdateTime > 0 {
		// Assume LastUpdateTime is epoch seconds of last observed content activity
		days := time.Since(time.Unix(social.LastUpdateTime, 0)).Hours() / 24
		// 0 days → 100, 180+ days → 20
		if days <= 0 {
			activityScore = 100
		} else if days >= 180 {
			activityScore = 20
		} else {
			activityScore = 100 - (80 * (days / 180))
		}
	}
	// Small bump if significant content volume
	if social.ContentCount >= 200 {
		activityScore += 5
	} else if social.ContentCount <= 10 {
		activityScore -= 10
	}
	activityScore = clampFloat(activityScore, 0, 100)

	// 5) Misc signals (0–100)
	miscScore := 100.0
	// following/follower ratio penalty
	ffr := safeDiv(float64(social.FollowingCount), math.Max(followers, 1))
	if ffr > 1.0 {
		miscScore -= 30
	} else if ffr > 0.5 {
		miscScore -= 20
	} else if ffr > 0.2 {
		miscScore -= 10
	}
	// Missing UI buttons suggests odd setup
	if !social.HasFollowButton {
		miscScore -= 10
	}
	if !social.HasMessageButton {
		miscScore -= 10
	}
	miscScore = clampFloat(miscScore, 0, 100)

	// Weighted aggregate
	total := engagementScore*wEngagement + ratiosScore*wRatios + verificationScore*wVerification + activityScore*wActivity + miscScore*wMisc
	return clampInt(int(math.Round(total)), 0, 100)
}

func calculateBudget(social *trendlybq.Socials) Range {
	followers := float64(social.FollowerCount)
	avgViews := float64(social.AverageViews)
	er := float64(social.EngagementRate)

	// ------- Followers-based model (₹ per 1k followers baseline by region) -------
	basePerK := 800.0 // India baseline
	loc := strings.ToLower(social.Location)
	if strings.Contains(loc, "united states") || strings.Contains(loc, "usa") || strings.Contains(loc, "us") || strings.Contains(loc, "canada") || strings.Contains(loc, "uk") || strings.Contains(loc, "united kingdom") || strings.Contains(loc, "australia") {
		basePerK = 2500.0
	}

	// Niche premium multipliers
	nicheMult := 1.0
	candidate := strings.ToLower(social.Category)
	for _, n := range social.Niches {
		candidate += "," + strings.ToLower(n)
	}
	if strings.Contains(candidate, "finance") {
		nicheMult *= 1.35
	}
	if strings.Contains(candidate, "tech") || strings.Contains(candidate, "saas") {
		nicheMult *= 1.25
	}
	if strings.Contains(candidate, "beauty") || strings.Contains(candidate, "fashion") {
		nicheMult *= 1.15
	}
	if strings.Contains(candidate, "gaming") || strings.Contains(candidate, "travel") {
		nicheMult *= 1.10
	}

	// Engagement multiplier
	erMult := 1.0
	if er >= 0.05 { // >=5%
		erMult = 1.30
	} else if er >= 0.02 {
		erMult = 1.10
	} else if er >= 0.01 {
		erMult = 0.90
	} else {
		erMult = 0.75
	}

	// Verification multiplier
	verMult := 1.0
	if social.ProfileVerified {
		verMult = 1.10
	}

	// Trust multiplier (0.8–1.2)
	trust := float64(calculateTrustablity(social))
	trustMult := 0.8 + 0.004*trust
	trustMult = clampFloat(trustMult, 0.8, 1.2)

	// Quality multiplier (0.6–1.3)
	// quality: 0 (cheap creators) → 0.6x, 100 (rich/classy/aesthetic) → 1.3x
	quality := float64(social.QualityScore)
	qualityMult := 0.6 + 0.007*quality // maps 0..100 → 0.8..1.3
	qualityMult = clampFloat(qualityMult, 0.6, 1.3)

	followersBased := (followers / 1000.0) * basePerK * nicheMult * erMult * verMult * trustMult * qualityMult
	followersMin := followersBased * 0.75
	followersMax := followersBased * 1.25

	// ------- Views-based model (CPM approach) -------
	var viewsMin, viewsMax float64
	if avgViews > 0 {
		// Choose CPM band based on ER
		cpmLow := 200.0  // ₹ per 1000 views
		cpmHigh := 600.0 // ₹ per 1000 views
		if er >= 0.05 {
			cpmLow, cpmHigh = 400, 900
		} else if er < 0.01 {
			cpmLow, cpmHigh = 150, 400
		}
		viewsMin = (avgViews / 1000.0) * cpmLow * nicheMult * verMult * qualityMult
		viewsMax = (avgViews / 1000.0) * cpmHigh * nicheMult * verMult * qualityMult
	} else {
		// Fallback to followers if views unknown
		viewsMin, viewsMax = followersMin, followersMax
	}

	// Combine (average the two models)
	minBudget := (followersMin + viewsMin) / 2.0
	maxBudget := (followersMax + viewsMax) / 2.0

	// Round to nearest ₹50
	return Range{
		Min: int(roundToNearest(minBudget, 50)),
		Max: int(roundToNearest(maxBudget, 50)),
	}
}

func calculateReach(social *trendlybq.Socials) Range {
	followers := float64(social.FollowerCount)
	avgViews := float64(social.AverageViews)
	er := float64(social.EngagementRate)
	trust := float64(calculateTrustablity(social)) / 100.0

	var baseReach float64
	if avgViews > 0 {
		baseReach = avgViews
	} else {
		// Fallback: percentage of followers based on ER
		reachRatio := 0.22 // default ~22%
		if er >= 0.05 {
			reachRatio = 0.35
		} else if er < 0.01 {
			reachRatio = 0.12
		}
		baseReach = followers * reachRatio
	}

	// Build a range around base using ER and trust
	lowMult := 0.80 - 0.10*clampFloat(0.02-er, 0, 0.02)/0.02 // penalize if ER <2%
	highMult := 1.20 + 0.10*trust                            // slight upside with trust
	lowMult = clampFloat(lowMult, 0.6, 0.95)
	highMult = clampFloat(highMult, 1.05, 1.40)

	minReach := baseReach * lowMult
	maxReach := baseReach * highMult

	return Range{
		Min: int(math.Round(minReach)),
		Max: int(math.Round(maxReach)),
	}
}

// ----------------- Helpers -----------------
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func roundToNearest(x float64, step int) float64 {
	if step <= 0 {
		return math.Round(x)
	}
	st := float64(step)
	return math.Round(x/st) * st
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant fetch"})
		return
	}

	calculatedValue := CalculatedData{
		Quality:         social.QualityScore,
		Trustablity:     calculateTrustablity(social),
		EstimatedBudget: calculateBudget(social),
		EstimatedReach:  calculateReach(social),
	}

	c.JSON(http.StatusOK, gin.H{"message": "Fetched influencer", "social": social, "analysis": calculatedValue})
}

func TestCalculations(social *trendlybq.Socials) CalculatedData {
	calculatedValue := CalculatedData{
		Quality:         social.QualityScore,
		Trustablity:     calculateTrustablity(social),
		EstimatedBudget: calculateBudget(social),
		EstimatedReach:  calculateReach(social),
	}
	return calculatedValue
}

func RequestConnection(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "api is functional"})
}
