package apify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const InstagramActorID = "shu8hvrXbJbY3Eb9W"

func GetInstagram(usernames []string, includeReels bool) ([]InstagramInfluencer, error) {
	urls := make([]string, len(usernames))
	for i, username := range usernames {
		urls[i] = fmt.Sprintf("https://www.instagram.com/%s/", username)
	}

	input := InstagramScraperInput{
		DirectUrls:    urls,
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

	if includeReels {
		results, err = getInstagramReels(results)
		if err != nil {
			return nil, fmt.Errorf("failed to get instagram reels: %w", err)
		}
	}

	return results, nil
}

func getInstagramReels(influencers []InstagramInfluencer) ([]InstagramInfluencer, error) {
	urls := make([]string, len(influencers))
	for i, influencer := range influencers {
		urls[i] = fmt.Sprintf("https://www.instagram.com/%s/", influencer.Username)
	}

	input := InstagramScraperInput{
		DirectUrls:    urls,
		ResultsType:   "reels",
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

	var results []InstagramPosts
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	influencerMap := make(map[string]*InstagramInfluencer)
	for i, inf := range influencers {
		influencers[i].Reels = []InstagramPosts{}
		influencerMap[inf.Username] = &influencers[i]
	}

	for _, post := range results {
		if influencer, ok := influencerMap[post.OwnerUsername]; ok {
			influencer.Reels = append(influencer.Reels, post)
		}
	}

	return influencers, nil
}
