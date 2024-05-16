package messenger

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type ConversationData struct {
	Data []ConversationMessagesData `json:"data"`
}
type ConversationMessagesData struct {
	Name         string       `json:"name"`
	Participants Participants `json:"participants"`
	ID           string       `json:"id"`
	Messages     struct {
		Data []Message `json:"data"`
	} `json:"messages"`
}

func GetConversationsByUserId(userID string) (*ConversationData, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/me/conversations?platform=%s&fields=name,id,messages{%s}&user_id=%s&access_token=%s", baseURL, apiVersion, platform, messageInfoFields, userID, pageAccessToken)

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Print the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Print the response body
	// fmt.Println(string(body))
	data := ConversationData{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		fmt.Print(err.Error())
		return nil, err
	}

	return &data, nil
}

func GetConversationMessages(conversationID string) (*ConversationMessagesData, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=name,id,messages{%s}&access_token=%s", baseURL, apiVersion, conversationID, messageInfoFields, pageAccessToken)

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
	// fmt.Println(string(body))
	data := ConversationMessagesData{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		fmt.Print(err.Error())
		return nil, err
	}

	return &data, nil
}

func GetAllConversationInfo() (*ConversationData, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/me/conversations?platform=%s&fields=id,name,participants&access_token=%s", baseURL, apiVersion, platform, pageAccessToken)

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Print the response body
	// fmt.Println(string(body))

	data := ConversationData{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
