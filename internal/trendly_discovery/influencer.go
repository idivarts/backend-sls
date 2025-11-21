package trendlydiscovery

import (
	"context"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myquery"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"google.golang.org/api/iterator"
)

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}
type CalculatedData struct {
	Quality         int     `json:"quality"`
	Trustablity     int     `json:"trustablity"`
	EstimatedBudget Range   `json:"estimatedBudget"`
	EstimatedReach  Range   `json:"estimatedReach"`
	CPM             float32 `json:"cpm"`
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

	// // ------- Followers-based model (₹ per 1k followers baseline by region) -------
	// For nano-influencers: you might see CPMs as low as ₹50-₹200 per 1,000 views (depending on how many views actually happen, and engagement).
	// •	For micro-influencers: maybe ₹150-₹500 per 1,000 views.
	// •	For mid-to-macro influencers: could be ₹500-₹1,500+ per 1,000 views, especially in desirable niches or when high production or exclusivity is involved.
	basePerK := 50.0 // India baseline
	if social.FollowerCount < 10000 {
		basePerK = 50.0 // India baseline
	} else if social.FollowerCount < 50000 {
		basePerK = 80.0 // India baseline
	} else if social.FollowerCount < 100000 {
		basePerK = 125.0 // India baseline
	} else {
		basePerK = 200.0 // India baseline
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

	allMult := nicheMult * erMult * verMult * trustMult * qualityMult * 0.75

	followersBased := (followers / 1000.0) * basePerK * allMult
	followersMin := followersBased * 0.85
	followersMax := followersBased * 1.15

	// ------- Views-based model (CPM approach) -------
	var viewsMin, viewsMax float64
	if avgViews > 0 {
		// Choose CPM band based on ER
		cpmLow := 200.0  // ₹ per 1000 views
		cpmHigh := 300.0 // ₹ per 1000 views
		if er >= 4 {
			cpmLow, cpmHigh = 250, 500
		} else if er < 1 {
			cpmLow, cpmHigh = 100, 200
		}
		viewsMin = (avgViews / 1000.0) * cpmLow * allMult
		viewsMax = (avgViews / 1000.0) * cpmHigh * allMult
	} else {
		// Fallback to followers if views unknown
		viewsMin, viewsMax = followersMin, followersMax
	}

	// Combine (average the two models)
	minBudget := (followersMin + viewsMin) / 2.0
	maxBudget := (followersMax + viewsMax) / 2.0

	// ---- Apply hard tier caps so budgets stay within expected ranges ----
	if _, capMax, ok := budgetTierCaps(social.FollowerCount); ok {
		// Ensure min/max remain within [capMin, capMax]

		// if minBudget < capMin {
		// 	minBudget = capMin
		// }
		if maxBudget > capMax {
			maxBudget = capMax
		}
		// Guard: if the model produced a narrow band above capMax
		if minBudget > maxBudget {
			minBudget = capMax
			maxBudget = capMax
		}
	}

	// Enforced Influencer Tier Caps (INR per Post/Reel)
	// Nano (1K-10K): ₹1,000 – ₹5,000
	// Micro (10K-100K): ₹5,000 – ₹50,000
	// Mid-tier (100K-500K): ₹50,000 – ₹2,00,000
	// Note: These caps are applied via budgetTierCaps() above and will bound the returned range.

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

// budgetTierCaps returns hard min/max caps (INR) for price ranges by follower tier.
// If a tier is not covered, ok=false and no cap is applied.
func budgetTierCaps(followers int64) (min float64, max float64, ok bool) {
	// Influencer Tier - Approximate Rate per Post / Reel (INR)
	// Nano (1K-10K)      -> ₹1,000 – ₹5,000
	// Micro (10K-100K)   -> ₹5,000 – ₹50,000
	// Mid-tier (100K-500K)-> ₹50,000 – ₹2,00,000
	switch {
	case followers >= 1000 && followers < 10000:
		return 1000, 5000, true
	case followers >= 10000 && followers < 100000:
		return 5000, 50000, true
	case followers >= 100000 && followers < 500000:
		return 50000, 200000, true
	default:
		return 0, 0, false
	}
}

// Make sure the the influencers discovery credit is reduced
// If the influencer is already fetched before, do not reduce the credit
// Also make sure the influencer is added uniquely to the user's list of influencers
func FetchInfluencer(c *gin.Context) {
	influencerId := c.Param("influencerId")
	if influencerId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Influencer Id missing", "error": "influencer-id-missing"})
	}

	brandId := c.Param("brandId")

	brand := &trendlymodels.Brand{}
	err := brand.Get(brandId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching brand"})
		return
	}

	var appended bool
	brand.DiscoveredInfluencers, appended = myutil.AppendUnique(brand.DiscoveredInfluencers, influencerId)
	if appended {
		if brand.Credits.Discovery <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no-discovery-credits", "message": "No Discovery Credits Available"})
			return
		}

		brand.Credits.Discovery -= 1
		_, err = brand.Insert(brandId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error Updating brand"})
			return
		}
	}

	social := &trendlybq.Socials{}

	err = social.Get(influencerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant fetch Social"})
		return
	}

	calculatedValue := CalculatedData{
		Quality:         social.QualityScore,
		Trustablity:     calculateTrustablity(social),
		EstimatedBudget: calculateBudget(social),
		EstimatedReach:  calculateReach(social),
	}
	calculatedValue.CPM = float32(calculatedValue.EstimatedBudget.Max+calculatedValue.EstimatedBudget.Min) * 1000 / float32(calculatedValue.EstimatedReach.Max+calculatedValue.EstimatedReach.Min)

	c.JSON(http.StatusOK, gin.H{"message": "Fetched influencer", "social": social, "analysis": calculatedValue})
}

func FetchInvitedInfluencers(c *gin.Context) {
	var req struct {
		Offset int    `json:"offset" binding:"required"`
		Limit  int    `json:"limit" binding:"required"`
		Filter string `json:"filter"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request"})
		return
	}

	filter := InfluencerFilters{
		Offset: &req.Offset,
		Limit:  &req.Limit,
	}
	base := FormSQL(filter)
	q := myquery.Client.Query(base)
	it, err := q.Read(context.Background())
	if err != nil {
		c.JSON(500, gin.H{"message": "Query failed", "error": err.Error(), "sql": base})
		return
	}

	type bqRow struct {
		UserID         string  `bigquery:"userId"`
		Fullname       string  `bigquery:"fullname"`
		Username       string  `bigquery:"username"`
		URL            string  `bigquery:"url"`
		Picture        string  `bigquery:"picture"`
		Followers      int64   `bigquery:"followers"`
		Views          int64   `bigquery:"views"`
		Engagements    int64   `bigquery:"engagements"`
		EngagementRate float64 `bigquery:"engagementRate"`
	}

	out := make([]InfluencerInviteUnit, 0, 100)
	for {
		var r bqRow
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(500, gin.H{"message": "Iteration failed", "error": err.Error(), "sql": base})
			return
		}
		out = append(out, InfluencerInviteUnit{
			InfluencerItem: InfluencerItem{
				UserID:         r.UserID,
				Fullname:       r.Fullname,
				Username:       r.Username,
				URL:            r.URL,
				Picture:        r.Picture,
				Followers:      r.Followers,
				Views:          r.Views,
				Engagements:    r.Engagements,
				EngagementRate: r.EngagementRate,
				IsDiscover:     true,
			},
			InvitedAt: time.Now().UnixMilli(),
			Status:    "waiting",
		})
	}

	log.Println("Data Processed", out)

	c.JSON(http.StatusOK, gin.H{
		"message":     "Fetched influencer",
		"influencers": out,
	})
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

func InviteInfluencerOnDiscover(c *gin.Context) {
	var req struct {
		Influencers    []string `json:"influencers" binding:"required,min=1"`
		Collaborations []string `json:"collaborations" binding:"required,min=1"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request"})
		return
	}

	brandId := c.Param("brandId")
	managerId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Unauthorized", "error": "unauthorized"})
		return
	}

	brand := &trendlymodels.Brand{}
	err := brand.Get(brandId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching brand"})
		return
	}

	// Verify if enough credits are present
	if brand.Credits.Connection < (len(req.Influencers) * len(req.Collaborations)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not-enough-connection-credits", "message": "Not enough connection credits"})
		return
	}

	// Send Invitations
	invites := []trendlymodels.Invitation{}
	for _, infId := range req.Influencers {
		for _, collabId := range req.Collaborations {
			invite := trendlymodels.Invitation{
				UserID:     infId,
				IsDiscover: true,

				ManagerID:       managerId,
				CollaborationID: collabId,
				Status:          "waiting",
				Message:         "Invited via Discovery",
				TimeStamp:       time.Now().UnixMilli(),
			}
			_, err := invite.Create()
			if err != nil {
				log.Println("Error sending invite:", err.Error())
				continue
			}
			invites = append(invites, invite)
		}
	}

	creditUtilized := len(invites)

	// Do the calculation of reducing the connection credits here. If invite was already sent before, do not reduce the credit
	brand.Credits.Connection -= creditUtilized

	_, err = brand.Insert(brandId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error Updating brand"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "api is functional", "creditsUsed": creditUtilized, "invitationsSent": invites})
}
