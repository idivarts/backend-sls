package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const messageInfoFields = "id,created_time,from,to,message,attachments"

type Participants struct {
	Data []struct {
		Username string `json:"username"`
		ID       string `json:"id"`
	} `json:"data"`
}

type VideoData struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	URL        string `json:"url"`
	PreviewURL string `json:"preview_url"`
}

type ImageData struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	MaxWidth   int    `json:"max_width"`
	MaxHeight  int    `json:"max_height"`
	URL        string `json:"url"`
	PreviewURL string `json:"preview_url"`
}

type Cursors struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

type Paging struct {
	Cursors Cursors `json:"cursors"`
	Next    string  `json:"next"`
}

type DataItem struct {
	ImageData *ImageData `json:"image_data,omitempty"`
	VideoData *VideoData `json:"video_data,omitempty"`
}

type Attachments struct {
	Data   []DataItem `json:"data"`
	Paging Paging     `json:"paging"`
}

type Message struct {
	ID          string       `json:"id"`
	CreatedTime CustomTime   `json:"created_time"`
	To          Participants `json:"to"`
	From        struct {
		Username string `json:"username"`
		ID       string `json:"id"`
	} `json:"from"`
	Message     string       `json:"message"`
	Attachments *Attachments `json:"attachments,omitempty"`
}

func GetMessageInfo(messageID string, pageAccessToken string) (*Message, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=%s&access_token=%s", BaseURL, ApiVersion, messageID, messageInfoFields, pageAccessToken)

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

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
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

func GetMessagesWithPagination(conversationID string, after string, limit int, pageAccessToken string) (*ConversationPaginatedMessageData, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/messages?fields=%s&limit=%d&after=%s&access_token=%s", BaseURL, ApiVersion, conversationID, messageInfoFields, limit, after, pageAccessToken)

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
	data := ConversationPaginatedMessageData{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		fmt.Print(err.Error())
		return nil, err
	}

	return &data, nil
}
