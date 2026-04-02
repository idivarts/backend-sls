package gemini_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/pkg/apify"
	"github.com/idivarts/backend-sls/pkg/gemini"
)

func TestDeduce(t *testing.T) {
	data, err := trendlybq.SocialsN8N{}.GetPaginated(10010, 1)
	if err != nil {
		t.Fatal(err)
	}
	influencer := data[0]

	t.Log("UserName: ", influencer.Username, "\n", "Gender: ", influencer.Gender, "\n", "Location: ", influencer.Location, "\n", "Niches: ", strings.Join(influencer.Niches, ", "), "\n", "Quality: ", influencer.QualityScore, "\n")

	// influencer.Gender = ""
	// influencer.Location = ""
	// influencer.Niches = nil
	// influencer.QualityScore = 0

	incluencerScrapped, err := apify.GetInstagram(influencer.Username, false)
	if err != nil {
		t.Fatal(err)
	}

	jsonData, err := json.Marshal(incluencerScrapped)
	if err != nil {
		t.Fatal(err)
	}

	output, err := gemini.EnrichInfluencer(string(jsonData))

	if err != nil {
		t.Fatal(err)
	}

	t.Log("UserName: ", influencer.Username, "\n", "Gender: ", output.Gender, "\n", "Location: ", output.Location, "\n", "Niches: ", strings.Join(output.Niches, ", "), "\n", "Quality: ", output.Quality, "\n")
}
