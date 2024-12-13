package instagram_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/instagram"
)

const code = `AQBC7U30Jlct4s9m5uroY_0Yr5pYBYe1nthVK6NwivmAXdNG1iksmr78GnlfzcXoeSqPlGfnweOq1nRdruH4p6oCE20bLbzCNsvdY-Mpz5dVdjB8Y6-deam7oKt5wqNIGbFmy-Q1hij2Ug9tB2tiQ2X6Gc40taNvzXJYgXNhw9sYN6e0ltAJmg-TZtGnaIk66WXSv3qgmK4OdLFjQzA4AEJO2Hbr4Pi-dismd43ph9Q9Xg#_`
const accessToken = `IGAAQlA4RZBCmVBZAFBNX1VoN3lJeHBkRjAxNm1Qa2hkRmlrTjBQNGdxUlNWclZATTTM5cWxURGNHZA1lJZAEJWc3VGLWFmdlZAaMW9uTzhtZAmQ4VGk5NE9USHFrZA2VDeFRQdk84cUJYVS1WUnVDWFNvSTh1MEhSUEVlTXhERUxhZAXlnVGgxWThOXzgycVJR`

// const longLivedAccessToken = `IGQWRQZAnBGM3NKcDRTRk03MHJzTWJPMDR5NGFwemlDTWNZALTBxZA0Vsel9iclZAMV01reFM0Q3pnTXozMlAxR0ZA4c3ZAidG9JWi1hbXVnMUhxY2ZAKRF9qekxnTnlZAVW96aDRMX3VyX09jRWtPQQZDZD`

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
