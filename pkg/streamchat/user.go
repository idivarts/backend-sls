package streamchat

import (
	"context"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
)

type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	IsManager bool   `json:"is_manager"`
}

func CreateOrUpdateUser(user User) (*stream_chat.UpsertUserResponse, error) {
	role := "user"
	if user.IsManager {
		role = "admin"
	}
	res, err := Client.UpsertUser(context.Background(), &stream_chat.User{
		ID:    user.ID,
		Name:  user.Name,
		Image: user.Image,
		Role:  role,
	})
	return res, err
}
