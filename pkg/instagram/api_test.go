package instagram_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/instagram"
)

const code = `AQAB6OfvQEP1tq_wXSHZXrnLnKNQL8tUmkYIPswgB9BSXO0bTkdyMIKs3eY5D4MBk3mlCAaTRHUlIkmfD7TlNN_q-P8YNm2lbLdKxD6zJLNYIZDloptz5wWIe6ghu0DIov6yuC9Fu84ELYSGszwtFYgWZK1ooUQ744EqoeZ0Umcij4Uese8LBjQGtBT8Y-EldfRTt-L4yC8qump9b9vbINairnuqrlpSeqnSRXQljC-n-w#_`
const accessToken = `IGAAQlA4RZBCmVBZAFBNX1VoN3lJeHBkRjAxNm1Qa2hkRmlrTjBQNGdxUlNWclZATTTM5cWxURGNHZA1lJZAEJWc3VGLWFmdlZAaMW9uTzhtZAmQ4VGk5NE9USHFrZA2VDeFRQdk84cUJYVS1WUnVDWFNvSTh1MEhSUEVlTXhERUxhZAXlnVGgxWThOXzgycVJR`

const longLivedAccessToken = `IGAAQlA4RZBCmVBZAE9FZA2tfV2ZAUdkNUT0NBUnI4M1BBVzNnemVLeDB6UWx2Mm9qYk5uNTJ1V0tmVk1mdGp6WGxPWkxMckFIQTRaem9tWVBQRG5KNm5GSlVOU0RWcjg3ZAHRYX291SVpUalZAyRmJER1ZAlOUln`

func TestToken(t *testing.T) {
	accessToken, err := instagram.GetAccessTokenFromCode(code, "https://be.trendly.pro/instagram/auth")
	if err != nil {
		t.Error(err)
	}
	log.Println("Access Token:", accessToken.AccessToken)

	llToken, err := instagram.GetLongLivedAccessToken(accessToken.AccessToken)
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Long Lived Access Token:", llToken.AccessToken)
}

func TestAccessToken(t *testing.T) {
	llToken, err := instagram.GetLongLivedAccessToken(accessToken)
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Long Lived Access Token:", llToken.AccessToken)
}

func TestGetUser(t *testing.T) {
	iData, err := instagram.GetInstagram("me", longLivedAccessToken)
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Instagram Data:", iData)
}

func TestInstaInsights(t *testing.T) {
	iData, err := instagram.GetInsights(longLivedAccessToken, []string{"impressions"}, "day", instagram.InsightParams{})
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Instagram Data:", iData)
}
