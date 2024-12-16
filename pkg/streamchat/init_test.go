package streamchat_test

import (
	"context"
	"testing"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
	"github.com/idivarts/backend-sls/pkg/streamchat"
)

func TestInit(t *testing.T) {
	_, err := streamchat.Client.UpsertUser(context.Background(), &stream_chat.User{
		ID:   "prE60KnuRNQyBvNP4di3HI5Emm62-test",
		Name: "Rahul Sinha",
	})
	if err != nil {
		t.Error(err)
	}
}
