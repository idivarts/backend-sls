package facebook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ConversationData struct {
	Data   []ConversationMessagesData `json:"data"`
	Paging struct {
		Cursors struct {
			After string `json:"after"`
		} `json:"cursors"`
		Next string `json:"next"`
	} `json:"paging"`
}
type ConversationMessagesData struct {
	Name         string       `json:"name"`
	Participants Participants `json:"participants"`
	ID           string       `json:"id"`
	Messages     *struct {
		Data []Message `json:"data"`
	} `json:"messages,omitempty"`
}
type ConversationPaginatedMessageData struct {
	Data   []Message `json:"data"`
	Paging struct {
		Cursors struct {
			After string `json:"after"`
		} `json:"cursors"`
		Next string `json:"next"`
	} `json:"paging"`
}

// GetConversationsByUserId looks up the conversation(s) a given external user
// has with the connected Page on the given Meta platform. Pass
// `PlatformMessenger` for Facebook Page Messenger threads or `PlatformInstagram`
// for IG Direct threads via a linked IG Business Account (the IG-via-Page
// legacy flow — IG-Login accounts use pkg/instagram instead).
func GetConversationsByUserId(userID, pageAccessToken, platform string) (*ConversationData, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/me/conversations?platform=%s&fields=id,participants&user_id=%s&access_token=%s", BaseURL, ApiVersion, platform, userID, pageAccessToken)

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

func GetConversationById(conversationID string, pageAccessToken string) (*ConversationMessagesData, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=name,id,participants&access_token=%s", BaseURL, ApiVersion, conversationID, pageAccessToken)

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

// GetConversationsPaginated lists DM conversations for a page-token-backed
// account on the given Meta platform: pass `PlatformMessenger` for Facebook
// Page Messenger threads, or `PlatformInstagram` for IG Direct threads on a
// linked IG Business Account (IG-Login accounts use pkg/instagram instead).
// The fetch retries with a shrinking page size on Meta's transient "Please
// reduce the amount of data you're asking for" 500s — see GraphGetRetry.
func GetConversationsPaginated(after string, limit int, pageAccessToken, platform string) (*ConversationData, error) {
	body, err := GraphGetRetry(func(l int) string {
		return fmt.Sprintf("%s/%s/me/conversations?platform=%s&fields=id,participants&limit=%d&access_token=%s&after=%s", BaseURL, ApiVersion, platform, l, pageAccessToken, after)
	}, limit)
	if err != nil {
		return nil, err
	}

	data := ConversationData{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
