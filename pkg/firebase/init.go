package firebaseapp

import (
	"context"
	"encoding/json"
	"log"
	"os"

	// firebase "firebase.google.com/go"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

var (
	FirebaseApp *firebase.App
	ConfigFile  string
	ProjectID   string
)

func init() {
	// Use a service account
	ctx := context.Background()
	ConfigFile = os.Getenv("FIREBASE_CONFIG_PATH")
	log.Println("Config File Path", ConfigFile)
	if ConfigFile == "" {
		ConfigFile = "service-account.json"
	}

	var err error
	ProjectID, err = readProjectID(ConfigFile)
	if err != nil {
		log.Fatalln(err)
		panic(err.Error())
	}

	sa := option.WithCredentialsFile(ConfigFile)
	log.Println("Coming here", sa)
	FirebaseApp, err = firebase.NewApp(ctx, nil, sa)
	if err != nil {
		log.Fatalln(err)
		panic(err.Error())
	}
	log.Println("Success Connection")
}

func readProjectID(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var sa struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(data, &sa); err != nil {
		return "", err
	}
	return sa.ProjectID, nil
}
