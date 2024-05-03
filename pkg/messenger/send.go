package messenger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Recipient struct {
	ID string `json:"id"`
}

type DefaultAction struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Button struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Payload string `json:"payload,omitempty"`
}

type Element struct {
	Title         string        `json:"title"`
	ImageURL      string        `json:"image_url"`
	Subtitle      string        `json:"subtitle"`
	DefaultAction DefaultAction `json:"default_action"`
	Buttons       []Button      `json:"buttons"`
}

type Payload struct {
	TemplateType string    `json:"template_type"`
	Elements     []Element `json:"elements"`
}

type Attachment struct {
	Type    string  `json:"type"`
	Payload Payload `json:"payload"`
}

type MessageUnit struct {
	Text       *string     `json:"text,omitempty"`
	Attachment *Attachment `json:"attachment,omitempty"`
}

type ISendMessage struct {
	Recipient Recipient   `json:"recipient"`
	Message   MessageUnit `json:"message"`
}

func GetRecepientIDFromParticipants(participants Participants) string {
	if len(participants.Data) > 2 {
		return "Multi user group not supported"
	}
	for i := 0; i < len(participants.Data); i++ {
		if participants.Data[i].Username != MyUserName {
			return participants.Data[i].ID
		}
	}

	return "Not Found"
}
func SendTextMessage(recipientID string, msg string) error {
	message := ISendMessage{
		Recipient: Recipient{
			ID: recipientID,
		},
		Message: MessageUnit{
			Text: &msg,
		},
	}
	return sendMessage(message)
}
func sendMessage(message ISendMessage) error {
	// Convert the message struct to JSON
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	url := baseURL + "/" + apiVersion + "/me/messages?access_token=" + pageAccessToken
	fmt.Println(url)
	// Make the HTTP request
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	defer resp.Body.Close()

	rData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Println("Status Code", resp.StatusCode, string(rData))

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
