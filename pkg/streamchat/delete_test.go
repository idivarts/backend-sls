package streamchat_test

import (
	"testing"

	"github.com/idivarts/backend-sls/pkg/streamchat"
)

func TestDelete(t *testing.T) {
	streamchat.DeleteAllChannels()
}
