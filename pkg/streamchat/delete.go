package streamchat

import (
	"context"
	"log"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
)

func DeleteAllChannels() {
	cids := []string{}
	channels, err := Client.QueryChannels(context.Background(), &stream_chat.QueryOption{})
	if err != nil {
		panic(err)
	}
	for i := range channels.Channels {
		ch := channels.Channels[i]
		cids = append(cids, ch.CID)
	}
	log.Println("Total Channels", len(cids))
	Client.DeleteChannels(context.Background(), cids, true)
}
