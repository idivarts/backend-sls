package instagram

import "os"

const (
	BaseURL    = "https://graph.instagram.com"
	ApiVersion = "v21.0"
)

var (
	ClientID     = "1166596944824933"
	ClientSecret = "e1003872fc1e9167220ea31d65e58d97"
)

func init() {
	clientId := os.Getenv("INSTA_CLIENT_ID")
	clientSecret := os.Getenv("INSTA_CLIENT_SECRET")

	if clientId != "" && clientSecret != "" {
		ClientID = clientId
		ClientSecret = clientSecret
	}
}
