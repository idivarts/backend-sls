package apify

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/idivarts/backend-sls/pkg/myutil"
)

const (
	ApifyBaseURL = "https://api.apify.com/v2"
)

var (
	ApifyToken = "<YOUR_API_TOKEN>"
)

type KeySecretJson struct {
	Apify struct {
		Token string `json:"token"`
	} `json:"apify"`
}

func init() {
	basePath := "."
	if myutil.IsTest() {
		basePath = "/Users/rsinha/iDiv/backend-sls/"
	}
	path := filepath.Join(basePath, "key-secrets.json")
	file, err := os.Open(path)
	if err != nil {
		log.Printf("could not open key-secrets.json: %v", err)
		return
	}
	defer file.Close()

	var secrets KeySecretJson
	if err := json.NewDecoder(file).Decode(&secrets); err != nil {
		log.Printf("could not decode key-secrets.json: %v", err)
		return
	}

	ApifyToken = secrets.Apify.Token
}
