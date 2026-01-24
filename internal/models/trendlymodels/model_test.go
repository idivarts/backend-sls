package trendlymodels_test

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func TestCollabIds(t *testing.T) {
	ids, err := trendlymodels.GetCollabIDs(nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("All Collab Ids", ids)
}
