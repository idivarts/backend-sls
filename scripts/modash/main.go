package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

const API_KEY_BASE64 = "TG1kMDAwQWNXSHliQVFlcEVyRXBrTUxUYkZVUmVEZFg="

// --- Structs that match the JSON body ---
type SearchRequest struct {
	Page              int    `json:"page"`
	CalculationMethod string `json:"calculationMethod"`
	Sort              Sort   `json:"sort"`
	Filter            Filter `json:"filter"`
}

type Sort struct {
	Field     string `json:"field"`
	Value     int    `json:"value"`
	Direction string `json:"direction"`
}

type Filter struct {
	Influencer InfluencerFilter `json:"influencer"`
	Audience   AudienceFilter   `json:"audience"`
}

type InfluencerFilter struct {
	Followers struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"followers"`
	EngagementRate    float64         `json:"engagementRate"`
	Location          []int           `json:"location"`
	Language          string          `json:"language"`
	LastPosted        int             `json:"lastposted"`
	Relevance         []string        `json:"relevance"`
	AudienceRelevance []string        `json:"audienceRelevance"`
	Gender            string          `json:"gender"`
	Age               Range           `json:"age"`
	FollowersGrowth   GrowthRate      `json:"followersGrowthRate"`
	Bio               string          `json:"bio"`
	HasYouTube        bool            `json:"hasYouTube"`
	HasContactDetails []ContactFilter `json:"hasContactDetails"`
	AccountTypes      []int           `json:"accountTypes"`
	Brands            []int           `json:"brands"`
	Interests         []int           `json:"interests"`
	Keywords          string          `json:"keywords"`
	TextTags          []TextTag       `json:"textTags"`
	ReelsPlays        Range           `json:"reelsPlays"`
	IsVerified        bool            `json:"isVerified"`
	HasSponsoredPosts bool            `json:"hasSponsoredPosts"`
	Engagements       Range           `json:"engagements"`
	FilterOperations  []Operation     `json:"filterOperations"`
}

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type GrowthRate struct {
	Interval string  `json:"interval"`
	Value    float64 `json:"value"`
	Operator string  `json:"operator"`
}

type ContactFilter struct {
	ContactType  string `json:"contactType"`
	FilterAction string `json:"filterAction"`
}

type TextTag struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Operation struct {
	Operator string `json:"operator"`
	Filter   string `json:"filter"`
}

type AudienceFilter struct {
	Location    []WeightedField `json:"location"`
	Language    WeightedField   `json:"language"`
	Gender      WeightedField   `json:"gender"`
	Age         []WeightedField `json:"age"`
	AgeRange    WeightedAge     `json:"ageRange"`
	Interests   []WeightedField `json:"interests"`
	Credibility float64         `json:"credibility"`
}

type WeightedField struct {
	ID     interface{} `json:"id"`
	Weight float64     `json:"weight"`
}

type WeightedAge struct {
	Min    string  `json:"min"`
	Max    string  `json:"max"`
	Weight float64 `json:"weight"`
}

// --- Function to call the API ---
func searchInfluencers(token string, reqBody SearchRequest) error {
	url := "https://api.modash.io/v1/instagram/search"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

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

	fmt.Println("Status:", resp.Status)
	return nil
}

func main() {
	tokenBytes, err := base64.StdEncoding.DecodeString(API_KEY_BASE64)
	if err != nil {
		panic(err)
	}

	token := string(tokenBytes)

	// Example minimal request
	reqBody := SearchRequest{
		Page:              0,
		CalculationMethod: "median",
		Sort: Sort{
			Field:     "followers",
			Value:     123,
			Direction: "desc",
		},
		Filter: Filter{
			Influencer: InfluencerFilter{
				Language:   "en",
				IsVerified: true,
			},
		},
	}

	if err := searchInfluencers(token, reqBody); err != nil {
		panic(err)
	}
}
