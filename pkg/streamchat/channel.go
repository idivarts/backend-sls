package streamchat

import (
	"context"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
)

func SendSystemMessage(channelId, message string) error {
	channel := Client.Channel("messaging", channelId)
	_, err := channel.SendMessage(context.Background(), &stream_chat.Message{
		Text: message,
		Type: stream_chat.MessageTypeSystem,
	}, "system")

	if err != nil {
		return err
	}
	return nil
}
