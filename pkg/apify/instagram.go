package apify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const InstagramActorID = "shu8hvrXbJbY3Eb9W"

func GetInstagram(username string, highValueInfluencer bool) (*InstagramInfluencer, error) {
	profileURL := fmt.Sprintf("https://www.instagram.com/%s/", username)

	input := InstagramScraperInput{
		DirectUrls:    []string{profileURL},
		ResultsType:   "details",
		ResultsLimit:  30,
		AddParentData: false,
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	// Using the Run Actor synchronously and get dataset items endpoint
	url := fmt.Sprintf("%s/acts/%s/run-sync-get-dataset-items?token=%s", ApifyBaseURL, InstagramActorID, ApifyToken)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("apify api returned non-ok status: %s", resp.Status)
	}

	var results []InstagramInfluencer
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no instagram data found for username: %s", username)
	}

	influencer := &results[0]

	videoCount := 0
	for _, post := range influencer.LatestPosts {
		if post.Type == "Video" {
			videoCount += 1
		}
	}

	scrapeCount := 0
	if videoCount < 6 {
		scrapeCount = 6
	}
	if highValueInfluencer {
		scrapeCount = 20
	}

	if scrapeCount > 0 {
		if err := getInstagramReels(influencer, 30); err != nil {
			return nil, fmt.Errorf("failed to get instagram reels: %w", err)
		}
	}

	return influencer, nil
}

func getInstagramReels(influencer *InstagramInfluencer, count int) error {
	profileURL := fmt.Sprintf("https://www.instagram.com/%s/", influencer.Username)

	input := InstagramScraperInput{
		DirectUrls:         []string{profileURL},
		ResultsType:        "reels",
		ResultsLimit:       count,
		AddParentData:      false,
		OnlyPostsNewerThan: "31 days",
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	// Using the Run Actor synchronously and get dataset items endpoint
	url := fmt.Sprintf("%s/acts/%s/run-sync-get-dataset-items?token=%s", ApifyBaseURL, InstagramActorID, ApifyToken)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("apify api returned non-ok status: %s", resp.Status)
	}

	var results []InstagramPosts
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	influencer.Reels = make([]InstagramPosts, 0, len(results))
	for _, post := range results {
		if post.OwnerUsername == influencer.Username {
			influencer.Reels = append(influencer.Reels, post)
		}
	}

	return nil
}
