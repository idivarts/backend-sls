package streamchat

import (
	"context"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
)

func CreateUser() {
	// admin, user
	Client.UpsertUser(context.Background(), &stream_chat.User{
		ID: "user1",
	})
}
