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

type SenderAction string

const (
	MARK_SEEN  SenderAction = "mark_seen"
	TYPING_ON  SenderAction = "typing_on"
	TYPING_OFF SenderAction = "typing_off"
)

type ISendAction struct {
	Recipient Recipient    `json:"recipient"`
	Action    SenderAction `json:"sender_action"`
}
type IMessageResponse struct {
	RecipientID string `json:"recipient_id"`
	MessageID   string `json:"message_id"`
}

func GetRecepientIDFromParticipants(participants Participants, userName string) string {
	if len(participants.Data) > 2 {
		return "Multi user group not supported"
	}
	for i := 0; i < len(participants.Data); i++ {
		if participants.Data[i].Username != userName {
			return participants.Data[i].ID
		}
	}

	return "Not Found"
}
func SendTextMessage(recipientID string, msg string, pageAccessToken string) (*IMessageResponse, error) {
	message := ISendMessage{
		Recipient: Recipient{
			ID: recipientID,
		},
		Message: MessageUnit{
			Text: &msg,
		},
	}
	return sendMessage(message, pageAccessToken)
}

func SendAction(recipientID string, action SenderAction, pageAccessToken string) (*IMessageResponse, error) {
	message := ISendAction{
		Recipient: Recipient{
			ID: recipientID,
		},
		Action: action,
	}
	return sendMessage(message, pageAccessToken)
}

func sendMessage(message interface{}, pageAccessToken string) (*IMessageResponse, error) {
	// Convert the message struct to JSON
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	url := BaseURL + "/" + ApiVersion + "/me/messages?access_token=" + pageAccessToken
	fmt.Println(url)
	// Make the HTTP request
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
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

	rObj := &IMessageResponse{}
	err = json.Unmarshal(rData, rObj)
	if err != nil {
		return nil, err
	}
	return rObj, nil
}
