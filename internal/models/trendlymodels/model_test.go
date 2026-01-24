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

// func TestScriptToSetAllLive(t *testing.T) {
// 	ids, err := trendlymodels.GetCollabIDs(nil, 1000)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	t.Log("All Collab Ids", ids)
// 	for _, v := range ids {
// 		firestoredb.Client.Collection("collaborations").Doc(v).Update(context.Background(), []firestore.Update{
// 			firestore.Update{Path: "isLive", Value: true},
// 		})
// 		t.Log("Updated Collab", v)
// 	}
// 	t.Log("Updated All", ids)
// }
