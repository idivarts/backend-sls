package hubspot

import (
	"encoding/base64"
	"log"
)

var apiKey = ""

func init() {
	base64key := "cGF0LW5hMS04YmU4ZmViNi03Nzg2LTQ3NzYtYWE5MC02Y2E3ZTg0NDBiNzk="
	// base64key := os.Getenv("HUBSPOT_API_KEY")
	// Decode the string
	decodedBytes, err := base64.StdEncoding.DecodeString(base64key)
	if err != nil {
		log.Fatalf("Failed to decode base64 string: %v", err)
	}

	// Convert the bytes to string
	apiKey = string(decodedBytes)
}
