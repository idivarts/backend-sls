package instainterfaces

import (
	"encoding/json"
	"fmt"
)

type InstagramMessage struct {
	Object string  `json:"object"`
	Entry  []Entry `json:"entry"`
}

type Entry struct {
	ID        string      `json:"id"`   // ID of your Instagram Professional account
	Time      int64       `json:"time"` // Unix timestamp of the event
	Messaging []Messaging `json:"messaging"`
}

type MessageType string

const (
	MessageTypeMessage  MessageType = "message"
	MessageTypeReaction MessageType = "reaction"
	MessageTypePostback MessageType = "postback"
	MessageTypeReferral MessageType = "referral"
	MessageTypeRead     MessageType = "read"
)

type Messaging struct {
	Sender    Sender    `json:"sender"`
	Recipient Recipient `json:"recipient"`
	Timestamp int64     `json:"timestamp"`

	// Additional field to indicate the type of message
	Type MessageType `json:"type,omitempty"`

	// The below entry can be omitted as well
	Message  *Message  `json:"message,omitempty"`
	Reaction *Reaction `json:"reaction,omitempty"`
	Postback *Postback `json:"postback,omitempty"`
	Referral *Referral `json:"referral,omitempty"`
	Read     *Read     `json:"read,omitempty"`
}

type Sender struct {
	ID string `json:"id"` // Instagram-scoped ID for the customer who sent the message
}

type Recipient struct {
	ID string `json:"id"` // ID of your Instagram Professional account
}

type Message struct {
	Mid           string       `json:"mid"`            // ID of the message sent to your business
	Text          string       `json:"text"`           // Included when a customer sends a message containing text
	IsDeleted     bool         `json:"is_deleted"`     // Included when a customer deletes a message
	IsEcho        bool         `json:"is_echo"`        // Included when your business sends a message to the customer
	IsUnsupported bool         `json:"is_unsupported"` // Included when a customer sends a message with unsupported media
	QuickReply    QuickReply   `json:"quick_reply"`
	Attachments   []Attachment `json:"attachments"`
	Referral      Referral     `json:"referral"`
	ReplyTo       ReplyTo      `json:"reply_to"`
}

type QuickReply struct {
	Payload string `json:"payload"` // The payload with the option selected by the customer
}

type Attachment struct {
	Type    string `json:"type"` // Can be audio, file, image (image or sticker), share, story_mention, or video
	Payload struct {
		URL string `json:"url"`
	} `json:"payload"`
}

type Referral struct {
	Product        Product        `json:"product"`
	Ref            string         `json:"ref"`   // REF-DATA-IN-AD-IF-SPECIFIED
	AdID           int            `json:"ad_id"` // AD-ID
	Source         string         `json:"source"`
	Type           string         `json:"type"`
	AdsContextData AdsContextData `json:"ads_context_data"`
}

type Product struct {
	ID string `json:"id"`
}

type AdsContextData struct {
	AdTitle  string `json:"ad_title"`  // TITLE-FOR-THE-AD
	PhotoURL string `json:"photo_url"` // IMAGE-URL-THAT-WAS-CLICKED
	VideoURL string `json:"video_url"` // THUMBNAIL-URL-FOR-THE-AD-VIDEO
}

type ReplyTo struct {
	Mid string `json:"mid"` // MESSAGE-ID
}

type Reaction struct {
	Mid      string `json:"mid"`                // ID of the message sent to your business
	Action   string `json:"action"`             // "react" or "unreact"
	Reaction string `json:"reaction,omitempty"` // optional, to unreact if there is no reaction field
	Emoji    string `json:"emoji,omitempty"`    // optional, to unreact if there is no emoji field
}

type Postback struct {
	Mid     string `json:"mid"`     // ID for the message sent to your business
	Title   string `json:"title"`   // SELECTED-ICEBREAKER-REPLY-OR-CTA-BUTTON
	Payload string `json:"payload"` // The payload with the option selected by the customer
}

type Read struct {
	Mid string `json:"mid"` // ID of the message that was read
}

func (message *InstagramMessage) CalcualateMessageType() {

	for i := 0; i < len(message.Entry); i++ {
		for j := 0; j < len(message.Entry[i].Messaging); j++ {
			// Unmarshal the JSON into a Messaging struct
			msg := &message.Entry[i].Messaging[j]

			// Check which optional field is populated
			switch {
			case msg.Message != nil:
				msg.Type = MessageTypeMessage
			case msg.Reaction != nil:
				msg.Type = MessageTypeReaction
			case msg.Postback != nil:
				msg.Type = MessageTypePostback
			case msg.Referral != nil:
				msg.Type = MessageTypeReferral
			case msg.Read != nil:
				msg.Type = MessageTypeRead
			default:
				// None of the optional fields are populated
			}
		}
	}
}

func NewWebHook(jsonString string) (*InstagramMessage, error) {
	var fbMessage InstagramMessage
	err := json.Unmarshal([]byte(jsonString), &fbMessage)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}
	fbMessage.CalcualateMessageType()
	return &fbMessage, nil
}
