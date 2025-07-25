package matchmaking_test

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/matchmaking"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func TestInfluencers(t *testing.T) {
	// AND location in ("Delhi")
	// AND category in ("Fashion / Beauty", "Food")
	// AND language in ("English", "Hindi")

	// ids, err := matchmaking.RunBQ2("Kolkata")
	ids, err := matchmaking.RunBQ(trendlymodels.BrandPreferences{
		Locations:            []string{"Kolkata"},
		InfluencerCategories: []string{"Fashion / Beauty", "Food"},
		Languages:            []string{"English", "Hindi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ids)
}
