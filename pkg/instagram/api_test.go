package instagram_test

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/instagram"
)

func prettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

const code = `AQAB6OfvQEP1tq_wXSHZXrnLnKNQL8tUmkYIPswgB9BSXO0bTkdyMIKs3eY5D4MBk3mlCAaTRHUlIkmfD7TlNN_q-P8YNm2lbLdKxD6zJLNYIZDloptz5wWIe6ghu0DIov6yuC9Fu84ELYSGszwtFYgWZK1ooUQ744EqoeZ0Umcij4Uese8LBjQGtBT8Y-EldfRTt-L4yC8qump9b9vbINairnuqrlpSeqnSRXQljC-n-w#_`
const accessToken = `IGAAQlA4RZBCmVBZAFBNX1VoN3lJeHBkRjAxNm1Qa2hkRmlrTjBQNGdxUlNWclZATTTM5cWxURGNHZA1lJZAEJWc3VGLWFmdlZAaMW9uTzhtZAmQ4VGk5NE9USHFrZA2VDeFRQdk84cUJYVS1WUnVDWFNvSTh1MEhSUEVlTXhERUxhZAXlnVGgxWThOXzgycVJR`

const longLivedAccessToken = `IGAAQlA4RZBCmVBZAE56R2NZARmNwalB2UHdVRGtpc3pUSlVLNmZAiM0lBaU5YdzRyZA1ZAkbmZAjd0RocmwtZAjNNU3ZA5Rmt4bzZAFaTM0V2pQQXd6d3gydElDOUZAGQ1dvZAkZAHMlFRa3p3WTIzVGc0V2lLWnRxeExB`
const facebookToken = `EAAID6icQOs4BO7wIl6hDNTBdRWmMhHgnoeF4AgZA5D96CIOBl7WlTeFslrMZC4OtZA44cgeRd4jxJXarkARDwZCjHZArvv1pgC8QA9EBXnARFbrPk1wulK8zaJM4FfMZAnAnwBhPhr4PRbMdEMMWeGQvuLvHKZBjUQGpV54HX5awZCpW2YupSfrfljbgrMFiq0bN`
const pageId = `17841466618151294`

func TestToken(t *testing.T) {
	accessToken, err := instagram.GetAccessTokenFromCode(code, "https://be.trendly.now/instagram/auth")
	if err != nil {
		t.Error(err)
	}
	log.Println("Access Token:", prettyJSON(accessToken))

	llToken, err := instagram.GetLongLivedAccessToken(accessToken.AccessToken)
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Long Lived Access Token:", prettyJSON(llToken))
}

func TestAccessToken(t *testing.T) {
	llToken, err := instagram.GetLongLivedAccessToken(accessToken)
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Long Lived Access Token:", prettyJSON(llToken))
}

func TestGetUser(t *testing.T) {
	iData, err := instagram.GetInstagram("me", longLivedAccessToken)
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Instagram Data:", prettyJSON(iData))
}

func TestInstaInsights(t *testing.T) {
	iData, err := instagram.GetInsights("me", longLivedAccessToken, []instagram.InsightMetric{instagram.MetricReach, instagram.MetricReplies}, instagram.PeriodDay, instagram.InsightParams{})
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Instagram Data:", prettyJSON(iData))
}

func TestMedia(t *testing.T) {
	iData, err := instagram.GetMedia("me", longLivedAccessToken, instagram.IGetMediaParams{GraphType: 1, TopComments: true})
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Instagram Data:", prettyJSON(iData))
}

func TestMediaFromFBGraph(t *testing.T) {
	iData, err := instagram.GetMedia("", facebookToken, instagram.IGetMediaParams{
		GraphType: 0,
		PageID:    pageId,
	})
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Instagram Data:", prettyJSON(iData))
}
