package streamchat

import (
	"os"
	"time"

	stream "github.com/GetStream/stream-chat-go/v5"
)

var (
	streamClient = "xv7c4yzcux6y"
	streamSecret = "x5p7xg5gerzmdj7e4uagcz4rdnm8abj7ktuj9hhtx2kqzdzmm8gr7a38xrcpywvt"
	Client       *stream.Client
)

func init() {
	// instantiate your stream client using the API key and secret
	// the secret is only used server side and gives you full access to the API
	if os.Getenv("STREAM_CLIENT") != "" && os.Getenv("STREAM_SECRET") != "" {
		streamClient = os.Getenv("STREAM_CLIENT")
		streamSecret = os.Getenv("STREAM_SECRET")
	}
	client, err := stream.NewClient(streamClient, streamSecret, stream.WithTimeout(60*time.Second))
	if err != nil {
		panic(err.Error())
	}

	// client.UpdateChannelType(context.Background(), "messaging", map[string]interface{}{
	// 	"reminders":   true,
	// 	"read_events": true,
	// })

	// settings := &stream.AppSettings{
	// 	EnforceUniqueUsernames: "no",
	// 	RemindersInterval:      300,
	// }
	// _, err = client.UpdateAppSettings(context.Background(), settings)
	// if err != nil {
	// 	panic(err.Error())
	// }
	// log.Println("Stream Chat client initialized")

	Client = client
}
