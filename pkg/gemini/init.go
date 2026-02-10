package gemini

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/google/generative-ai-go/genai"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"google.golang.org/api/option"
)

var Client *genai.Client

type KeySecretJson struct {
	Gemini struct {
		APIKey string `json:"apiKey"`
	} `json:"gemini"`
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

	Client, err = genai.NewClient(context.Background(), option.WithAPIKey(secrets.Gemini.APIKey))

	if err != nil {
		log.Printf("Could not create Gemini client: %v", err)
		return
	}
}
