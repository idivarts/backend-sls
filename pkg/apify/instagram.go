package apify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const InstagramActorID = "shu8hvrXbJbY3Eb9W"

func GetInstagram(usernames []string) error {
	urls := make([]string, len(usernames))
	for i, username := range usernames {
		urls[i] = fmt.Sprintf("https://www.instagram.com/%s/", username)
	}

	input := InstagramScraperInput{
		DirectUrls:   urls,
		ResultsType:  "details",
		ResultsLimit: 30,
		// SearchType:    "hashtag",
		// SearchLimit:   1,
		AddParentData: false,
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	url := fmt.Sprintf("%s/acts/%s/runs?token=%s", ApifyBaseURL, InstagramActorID, ApifyToken)

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

	return nil
}
