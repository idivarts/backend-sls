package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
)

const API_KEY_BASE64 = "TG1kMDAwQWNXSHliQVFlcEVyRXBrTUxUYkZVUmVEZFg="
const INDIA_LOCATION_ID = 304716 // TODO: Verify this ID via /instagram/locations; use the correct India country/location ID

// --- Structs that match the JSON body ---
type SearchRequest struct {
	Page              int    `json:"page"`              // Page number (0..665). Default: 0
	CalculationMethod string `json:"calculationMethod"` // How to compute averages for likes/comments/shares. Enum: "median" (default), "average"
	Sort              Sort   `json:"sort"`              // Sorting options (field/value/direction)
	Filter            Filter `json:"filter"`            // Filters for influencer and audience
}

type Sort struct {
	Field     string `json:"field"`     // Sorting field. Enum: "engagements", "followers", "engagementRate", "keywords", "audienceGeo", "audienceLang", "audienceGender", "audienceAge", "relevance", "followersGrowth", "audienceInterest", "reelsPlays" (some require corresponding filters)
	Value     int    `json:"value"`     // Optional numeric value used by some sorts (e.g., audience or keyword relevance). Example: 123
	Direction string `json:"direction"` // Sort direction. Enum: "asc", "desc"
}

type Filter struct {
	Influencer InfluencerFilter `json:"influencer"` // Influencer-level filters (profile, content, metrics)
	Audience   *AudienceFilter  `json:"audience"`   // Audience-level filters (demographics/interests) with weights
}

type InfluencerFilter struct {
	// LastPosted        int             `json:"lastposted"`          // Days since last post; must be >= 30
	// Language          string          `json:"language"`            // Influencer language code (use /instagram/languages), e.g., "en"
	// Gender            string          `json:"gender"`              // Influencer gender. Enum: "MALE", "FEMALE", "KNOWN", "UNKNOWN"
	// Age               Range           `json:"age"`                 // Influencer age bucket range. Allowed values for Min/Max: 18, 25, 35, 45, 65
	// Bio               string          `json:"bio"`                 // Search by bio/full name text
	// FollowersGrowth   GrowthRate      `json:"followersGrowthRate"` // Followers growth rate over interval
	// HasYouTube        bool            `json:"hasYouTube"`          // Whether influencer has a YouTube channel
	// HasContactDetails []ContactFilter `json:"hasContactDetails"`   // Contact channels presence filter
	// AccountTypes      []int           `json:"accountTypes"`        // Instagram account type IDs. 1=Regular, 2=Business, 3=Creator
	// Brands            []int           `json:"brands"`              // Brand IDs array (use /instagram/brands)
	// Interests         []int           `json:"interests"`           // Interest IDs array (use /instagram/interests)
	// Keywords          string          `json:"keywords"`            // Phrase contained in captions
	// TextTags          []TextTag       `json:"textTags"`            // Posts containing specific hashtags/mentions
	// HasSponsoredPosts bool        `json:"hasSponsoredPosts"` // Only influencers with sponsored posts if true
	// IsVerified       bool        `json:"isVerified"`       // Only verified accounts if true
	// FilterOperations []Operation `json:"filterOperations"` // Logical operators to combine filters; affects allowed sort fields
	// Engagements    Range   `json:"engagements"`    // Engagements count range (rounded). Tip: set min=0 to include hidden likes

	// look-a-likes and relevance
	// AudienceRelevance []string `json:"audienceRelevance"` // Similarity of influencer’s audience to given @usernames
	// Relevance         []string `json:"relevance"`         // Content/topic relevance tokens. Mix hashtags and @usernames. Max 100 usernames. E.g., ["#cars", "@topgear"]

	Followers      Range   `json:"followers"`      // Followers count filter. Values are rounded: <5k to nearest 1k, >=5k to nearest 5k
	EngagementRate float64 `json:"engagementRate"` // Minimum engagement rate (e.g., 0.02 for 2%)
	Location       []int   `json:"location"`       // Influencer locations by ID array (use /instagram/locations)
	ReelsPlays     Range   `json:"reelsPlays"`     // Reels plays count range (rounded to nearest 1k)
}

type Range struct {
	Min *int `json:"min"` // Minimum value
	Max *int `json:"max"` // Maximum value
}

type GrowthRate struct {
	Interval string  `json:"interval"` // Required. Time interval enum: "i1month","i2months","i3months","i4months","i5months","i6months"
	Value    float64 `json:"value"`    // Growth rate threshold (e.g., 0.01 for +1%)
	Operator string  `json:"operator"` // Required. Comparison operator enum: "gte","gt","lt","lte"
}

type ContactFilter struct {
	ContactType  string `json:"contactType"`  // Channel enum: "bbm","email","facebook","instagram","itunes","kakao","kik","lineid","linktree","pinterest","sarahah","sayat","skype","snapchat","tiktok","tumblr","twitchtv","twitter","vk","wechat","youtube"
	FilterAction string `json:"filterAction"` // Condition enum: "must" (include), "should" (prefer), "not" (exclude)
}

type TextTag struct {
	Type  string `json:"type"`  // Tag type enum: "hashtag" or "mention"
	Value string `json:"value"` // The hashtag/mention value (without #/@ for value, per docs wording)
}

type Operation struct {
	Operator string `json:"operator"` // Logical operator enum: "and","or","not". Using "or" requires at least two filters.
	Filter   string `json:"filter"`   // Target filter key. Enum: "followers","engagements","engagementRate","lastposted","bio","keywords","relevance","language","gender","age","location","isVerified","interests","brands","accountTypes","hasSponsoredPosts","textTags". Note: If "or" or "not" are used on a filter, you cannot sort by that field.
}

type AudienceFilter struct {
	// Location    []WeightedField `json:"location"`    // Audience location IDs with weight (default weight 0.2)
	// Language    WeightedField   `json:"language"`    // Audience language code with weight (default 0.2)
	// Interests   []WeightedField `json:"interests"`   // Audience interest IDs with weight (default 0.3)
	// Age         []WeightedField `json:"age"`         // Audience age groups with weight (default 0.3). ID enum: "13-17","18-24","25-34","35-44","45-64","65-"

	Gender      *WeightedField `json:"gender"`      // Audience gender with weight (default 0.5). ID enum: "MALE","FEMALE"
	AgeRange    *WeightedAge   `json:"ageRange"`    // Alternate way to specify a continuous audience age range with weight (cannot combine with Age)
	Credibility float64        `json:"credibility"` // Audience credibility (1 - fake followers). E.g., 0.75 = 25% fake
}

type WeightedField struct {
	ID     interface{} `json:"id"`     // The ID value: number for locations/interests, string for language (e.g., "en") or gender (e.g., "MALE") or age bucket (e.g., "18-24")
	Weight float64     `json:"weight"` // Weight threshold (fraction between 0 and 1)
}

type WeightedAge struct {
	Min    string  `json:"min"`    // Min audience age. Enum: "13","18","25","35","45","65"
	Max    string  `json:"max"`    // Max audience age. Enum: "17","24","34","44","64"
	Weight float64 `json:"weight"` // Weight threshold (default 0.3)
}

// --- Function to call the API ---
func searchInfluencers(token string, reqBody SearchRequest) error {
	url := "https://api.modash.io/v1/instagram/search"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	fmt.Println("Request Body:", string(bodyBytes))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody := new(bytes.Buffer)
	respBody.ReadFrom(resp.Body)
	fmt.Println("Response Body:", respBody.String())
	// Save the response body to a file for inspection
	file, err := os.Create("response.json")
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(respBody.Bytes())
	if err != nil {
		return err
	}

	fmt.Println("Status:", resp.Status)
	return nil
}

// "filterOperations": [
//         {
//           "operator": "and",
//           "filter": "followers"
//         }
//       ]

func main() {
	tokenBytes, err := base64.StdEncoding.DecodeString(API_KEY_BASE64)
	if err != nil {
		panic(err)
	}

	token := string(tokenBytes)

	// _ = []*AudienceFilter{
	// 	{
	// 		Gender: WeightedField{
	// 			ID:     "MALE",
	// 			Weight: 0.6,
	// 		},
	// 		AgeRange: WeightedAge{
	// 			Min:    "18",
	// 			Max:    "24",
	// 			Weight: 0.6,
	// 		},
	// 		Credibility: 0.75,
	// 	},
	// 	{
	// 		Gender: WeightedField{
	// 			ID:     "MALE",
	// 			Weight: 0.6,
	// 		},
	// 		AgeRange: WeightedAge{
	// 			Min:    "25",
	// 			Max:    "35",
	// 			Weight: 0.6,
	// 		},
	// 		Credibility: 0.75,
	// 	},
	// 	{
	// 		Gender: WeightedField{
	// 			ID:     "MALE",
	// 			Weight: 0.6,
	// 		},
	// 		AgeRange: WeightedAge{
	// 			Min:    "36",
	// 			Max:    "65",
	// 			Weight: 0.6,
	// 		},
	// 		Credibility: 0.75,
	// 	},
	// 	{
	// 		Gender: WeightedField{
	// 			ID:     "FEMALE",
	// 			Weight: 0.6,
	// 		},
	// 		AgeRange: WeightedAge{
	// 			Min:    "18",
	// 			Max:    "24",
	// 			Weight: 0.6,
	// 		},
	// 		Credibility: 0.75,
	// 	},
	// 	{
	// 		Gender: WeightedField{
	// 			ID:     "FEMALE",
	// 			Weight: 0.6,
	// 		},
	// 		AgeRange: WeightedAge{
	// 			Min:    "25",
	// 			Max:    "35",
	// 			Weight: 0.6,
	// 		},
	// 		Credibility: 0.75,
	// 	},
	// 	{
	// 		Gender: WeightedField{
	// 			ID:     "FEMALE",
	// 			Weight: 0.6,
	// 		},
	// 		AgeRange: WeightedAge{
	// 			Min:    "36",
	// 			Max:    "65",
	// 			Weight: 0.6,
	// 		},
	// 		Credibility: 0.75,
	// 	},
	// }

	// Example minimal request
	reqBody := SearchRequest{
		Page:              0,
		CalculationMethod: "median", // keep defaults for medians
		Sort: Sort{
			Field:     "engagementRate", // sort by follower count
			Value:     0,                // optional; unused for this sort
			Direction: "desc",
		},
		Filter: Filter{
			Influencer: InfluencerFilter{
				Followers:      Range{Min: aws.Int(10000), Max: aws.Int(100000)},
				EngagementRate: 0.02, // 2%
				Location:       []int{INDIA_LOCATION_ID},
				ReelsPlays:     Range{Min: aws.Int(100000)},
			},
			Audience: &AudienceFilter{
				Credibility: 0.75,
			},
			// Audience: audienceFilters[0],
		},
	}

	if err := searchInfluencers(token, reqBody); err != nil {
		panic(err)
	}
}
