package messenger_test

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/messenger"
)

func TestFacebookFetch(t *testing.T) {
	fb, err := messenger.GetFacebook("pageID", "EAAID6icQOs4BO6GdTXuC4GxBXyKuAmw9nZCnGtwPSOawq2EG4ra385MOL9Wu3esiZCwjqNM5FxiIgDSy55ZBwg9SXLFGoWtgjEcUnG5bIZAkRZCbLNOPTfwhZBwOLONahIlwTw5PZBOKqwpN0ZBoorfWoxaoYz9fJbgAtZC9C2NkKZBr2wVNhuWZBETdl2RBwZBhtmLO")
	if err != nil {
		t.Error(err)
	}

	// Convert fb struct to json and print it
	fbJSON, err := json.Marshal(fb)
	if err != nil {
		t.Error(err)
	}
	log.Println(string(fbJSON))
}

func TestInstaFetch(t *testing.T) {
	insta, err := messenger.GetInstagram("17841466618151294", "EAAID6icQOs4BO6GdTXuC4GxBXyKuAmw9nZCnGtwPSOawq2EG4ra385MOL9Wu3esiZCwjqNM5FxiIgDSy55ZBwg9SXLFGoWtgjEcUnG5bIZAkRZCbLNOPTfwhZBwOLONahIlwTw5PZBOKqwpN0ZBoorfWoxaoYz9fJbgAtZC9C2NkKZBr2wVNhuWZBETdl2RBwZBhtmLO")
	if err != nil {
		t.Error(err)
	}

	// Convert fb struct to json and print it
	instaJSON, err := json.Marshal(insta)
	if err != nil {
		t.Error(err)
	}
	log.Println(string(instaJSON))
}
