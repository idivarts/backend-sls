package instagram_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/instagram"
)

const code = `AQAB6OfvQEP1tq_wXSHZXrnLnKNQL8tUmkYIPswgB9BSXO0bTkdyMIKs3eY5D4MBk3mlCAaTRHUlIkmfD7TlNN_q-P8YNm2lbLdKxD6zJLNYIZDloptz5wWIe6ghu0DIov6yuC9Fu84ELYSGszwtFYgWZK1ooUQ744EqoeZ0Umcij4Uese8LBjQGtBT8Y-EldfRTt-L4yC8qump9b9vbINairnuqrlpSeqnSRXQljC-n-w#_`
const accessToken = `IGAAQlA4RZBCmVBZAFBNX1VoN3lJeHBkRjAxNm1Qa2hkRmlrTjBQNGdxUlNWclZATTTM5cWxURGNHZA1lJZAEJWc3VGLWFmdlZAaMW9uTzhtZAmQ4VGk5NE9USHFrZA2VDeFRQdk84cUJYVS1WUnVDWFNvSTh1MEhSUEVlTXhERUxhZAXlnVGgxWThOXzgycVJR`

const longLivedAccessToken = `IGQWRNd2JEcFdHU2QzZAnpieE94cFhOdnU5YW1RalFoSE9GQTltdnB5cFU5UlU1OVlGTmlHMThhMkh2V2tWajJOU2JTV19HN3l4REFOMGs1ZA29ualRzLXhLaWpNQ1g4SlFwRmx5TUI4ZAkhUdwZDZD`

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
