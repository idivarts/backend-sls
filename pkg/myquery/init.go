package myquery

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
)

var Client *bigquery.Client

func init() {
	ctx := context.Background()

	configFile := os.Getenv("FIREBASE_CONFIG_PATH")
	log.Println("Config File Path", configFile)
	if configFile == "" {
		configFile = "/Users/rsinha/iDiv/backend-sls/service-account.json"
	}
	sa := option.WithCredentialsFile(configFile)

	configData, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var config struct {
		ProjectID string `json:"project_id"`
	}

	if err := json.Unmarshal(configData, &config); err != nil {
		panic("error-parsing-bq")
	}

	projectID := config.ProjectID
	if projectID == "" {
		panic("no-project-id-bq")
	}

	// Optional: Path to your service account JSON key
	client, err := bigquery.NewClient(ctx, projectID, sa)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	Client = client
}
