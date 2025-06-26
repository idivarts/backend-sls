package matchmaking_test

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/matchmaking"
)

func TestInfluencers(t *testing.T) {
	ids, err := matchmaking.RunBQ()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ids)
}
