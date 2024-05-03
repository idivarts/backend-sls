package messenger

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const messageInfoFields = "id,created_time,from,to,message,attachment"

type Participants struct {
	Data []struct {
		Username string `json:"username"`
		ID       string `json:"id"`
	} `json:"data"`
}

type Message struct {
	ID          string       `json:"id"`
	CreatedTime CustomTime   `json:"created_time"`
	To          Participants `json:"to"`
	From        struct {
		Username string `json:"username"`
		ID       string `json:"id"`
	} `json:"from"`
	Message string `json:"message"`
}

func GetMessageInfo(messageID string) (*Message, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=%s&access_token=%s", baseURL, apiVersion, messageID, messageInfoFields, pageAccessToken)

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Print the response body
	fmt.Println(string(body))
	data := Message{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
