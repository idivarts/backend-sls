package messenger

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const messageInfoFields = "id,created_time,from,to,message"

type Participants struct {
	Data []struct {
		Username string `json:"username"`
		ID       string `json:"id"`
	} `json:"data"`
}

type Message struct {
	ID          string       `json:"id"`
	CreatedTime time.Time    `json:"created_time"`
	To          Participants `json:"to"`
	From        struct {
		Username string `json:"username"`
		ID       string `json:"id"`
	} `json:"from"`
	Message string `json:"message"`
}

func GetMessageInfo(messageID string) error {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=%s&access_token=%s", baseURL, apiVersion, messageID, messageInfoFields, pageAccessToken)

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Print the response body
	fmt.Println(string(body))

	return nil
}
