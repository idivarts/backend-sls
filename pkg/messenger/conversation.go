package messenger

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

type ConversationData struct {
	Data []struct {
		Name         string       `json:"name"`
		Participants Participants `json:"participants"`
		ID           string       `json:"id"`
		Messages     struct {
			Data []Message `json:"data"`
		} `json:"messages"`
	} `json:"data"`
}

func GetConversationsByUserId(userID string) error {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/conversations?platform=%s&user_id=%s&access_token=%s", baseURL, apiVersion, pageID, platform, userID, pageAccessToken)

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

type ConversationMessagesData struct {
	Messages struct {
		Name string `json:"name"`
	} `json:"messages"`
	ID           string       `json:"id"`
	Participants Participants `json:"participants"`
}

func GetConversationMessages(conversationID string) error {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=name,participants&access_token=%s", baseURL, apiVersion, conversationID, pageAccessToken)

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

func GetAllConversationInfo() error {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/conversations?platform=%s&access_token=%s", baseURL, apiVersion, pageID, platform, pageAccessToken)

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
