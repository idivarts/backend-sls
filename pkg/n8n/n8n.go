package n8n

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func GetInfluencerList(fileUrl string) ([]N8NInfluencer, error) {
	resp, err := http.Get(fileUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	var influencerList []N8NInfluencer
	err = json.NewDecoder(resp.Body).Decode(&influencerList)
	if err != nil {
		return nil, fmt.Errorf("failed to parse json: %w", err)
	}

	return influencerList, nil
}
