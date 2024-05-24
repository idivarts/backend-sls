package messenger

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const webhook_events = "messages,message_echoes"

func SubscribeApp(doSubsription bool, pageAccessToken string) error {
	// Convert the message struct to JSON
	fields := ""
	if doSubsription {
		fields = webhook_events
	}
	url := baseURL + "/" + apiVersion + "/me/subscribed_apps?subscribed_fields=" + fields + "&access_token=" + pageAccessToken
	fmt.Println(url)
	// Make the HTTP request
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	defer resp.Body.Close()

	rData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// rStr := string(rData)
	fmt.Println("Status Code", resp.StatusCode, string(rData))

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

type SubscribedAppData struct {
	Data []struct {
		Link             string   `json:"link"`
		Name             string   `json:"name"`
		ID               string   `json:"id"`
		SubscribedFields []string `json:"subscribed_fields"`
	} `json:"data"`
}

func GetSubscribedApps(pageAccessToken string) (*SubscribedAppData, error) {
	// Convert the message struct to JSON
	url := baseURL + "/" + apiVersion + "/me/subscribed_apps?access_token=" + pageAccessToken
	fmt.Println(url)
	// Make the HTTP request
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	rData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// rStr := string(rData)
	fmt.Println("Status Code", resp.StatusCode, string(rData))

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	rObj := &SubscribedAppData{}
	err = json.Unmarshal(rData, rObj)
	if err != nil {
		return nil, err
	}
	return rObj, nil
}
