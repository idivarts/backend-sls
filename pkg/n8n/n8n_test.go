package n8n

import (
	"testing"
)

func TestGetInfluencerList(t *testing.T) {
	url := "https://api.apify.com/v2/datasets/1vX9FW3yaOzkrFeBT/items"
	list, err := GetInfluencerList(url)
	if err != nil {
		t.Fatalf("Failed to get influencer list: %v", err)
	}

	if len(list) == 0 {
		t.Error("Expected non-empty influencer list")
	}

	t.Logf("Successfully fetched %d influencers", len(list))

	// Check the first influencer to ensure parsing worked
	if len(list) > 0 {
		if list[0].Username == "" {
			t.Error("First influencer username is empty")
		}
		t.Logf("First influencer: %s", list[0].Username)
	}
}
