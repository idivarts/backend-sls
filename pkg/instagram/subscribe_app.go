package instagram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Subscribed webhook fields for an Instagram-Login account. `messages` powers
// DMs (inbound, echoes, unsend, edits); `comments`/`mentions` power the comment
// inbox. Meta ignores fields not applicable to the subscription target.
const webhook_events = "messages,comments,mentions"

// SubscribeApp subscribes (or unsubscribes) the app to webhooks for an Instagram
// account connected via the Instagram Login API. This is the IG analogue of
// messenger.SubscribeApp — an app-level dashboard subscription is not enough;
// each IG account must subscribe the app on its own via the Graph API, using the
// IG user access token.
//
//	POST/DELETE https://graph.instagram.com/{version}/me/subscribed_apps
func SubscribeApp(doSubscription bool, igAccessToken string) error {
	url := BaseURL + "/" + ApiVersion + "/me/subscribed_apps?subscribed_fields=" + webhook_events + "&access_token=" + igAccessToken

	var resp *http.Response
	var err error
	if doSubscription {
		resp, err = http.Post(url, "application/json", nil)
	} else {
		var req *http.Request
		req, err = http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			return err
		}
		resp, err = (&http.Client{}).Do(req)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	rData, _ := io.ReadAll(resp.Body)
	fmt.Println("instagram subscribe_app status", resp.StatusCode, string(rData))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

type SubscribedAppData struct {
	Data []struct {
		Name             string   `json:"name"`
		ID               string   `json:"id"`
		SubscribedFields []string `json:"subscribed_fields"`
	} `json:"data"`
}

// GetSubscribedApps lists the apps subscribed to an IG account's webhooks (used
// to confirm a connection is wired up).
func GetSubscribedApps(igAccessToken string) (*SubscribedAppData, error) {
	url := BaseURL + "/" + ApiVersion + "/me/subscribed_apps?access_token=" + igAccessToken
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	rObj := &SubscribedAppData{}
	if err := json.Unmarshal(rData, rObj); err != nil {
		return nil, err
	}
	return rObj, nil
}
