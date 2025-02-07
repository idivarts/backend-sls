package messenger

import "os"

const (
	BaseURL             = "https://graph.facebook.com"
	ApiVersion          = "v19.0"
	TestPageAccessToken = "EAAGDG5jzw5QBO0LNmsZCk9BR8VsfYWgpEedm6WPdeXKLOEoVCt4RgXxLmeJHd45hv5vkNZBheZC4bIgMLD0hCkhcv6S3dedXjwTdlHyt3IJWMBQZCdJImXidYphUZCy7OwmNxmTCjYVutKjiiqrUyfzObw1TaVgCTRKIrsjlk60c6w3WAeYAsKCllZBPfK4Szv7gZDZD"
	// pageAccessToken     = "EAAGDG5jzw5QBO0LNmsZCk9BR8VsfYWgpEedm6WPdeXKLOEoVCt4RgXxLmeJHd45hv5vkNZBheZC4bIgMLD0hCkhcv6S3dedXjwTdlHyt3IJWMBQZCdJImXidYphUZCy7OwmNxmTCjYVutKjiiqrUyfzObw1TaVgCTRKIrsjlk60c6w3WAeYAsKCllZBPfK4Szv7gZDZD"
	// pageAccessTokenOld = "EAANaW51FjsgBOwj50iKNnsgxtFn2eSC9jgwlMHHT2JtafZAaulo3sYi2u87t3Lm8riYGguwahnhp6HYIAZCO0I2pMK1p0ZBmRGnww2BpgZAzCXZCSWiIevsJUy5qM5z6Mhw5LBcLouLZCqv1dCvAjSBZA3eqd11eoWAPXZAEE59ByytvyuR1xFlHQJI7y6WQKN97fwZDZD"
	platform     = "instagram"
	TestUserName = "trendshub.ai"
)

var (
	ClientID     = "425629530178452"
	ClientSecret = "0babd776dab621585c2370dccec78f2f"
)

func init() {
	clientId := os.Getenv("FB_CLIENT_ID")
	clientSecret := os.Getenv("FB_CLIENT_SECRET")

	if clientId != "" && clientSecret != "" {
		ClientID = clientId
		ClientSecret = clientSecret
	}
}
