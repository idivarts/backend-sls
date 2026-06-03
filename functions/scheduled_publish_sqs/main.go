package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/trendlyapis/publishing"
)

// handler consumes delayed-publish messages (delivered by the delayed_sqs Step
// Functions state machine when a scheduled time is reached) and publishes the
// referenced content. Errors are swallowed per-record because PublishContent
// already records publishError on the document, and we don't want one failing
// post to retry/replay the entire batch.
func handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, record := range sqsEvent.Records {
		var msg publishing.ScheduleMessage
		if err := json.Unmarshal([]byte(record.Body), &msg); err != nil {
			log.Println("scheduled_publish_sqs: bad message:", err, record.Body)
			continue
		}
		if msg.Action != "PUBLISH" {
			continue
		}
		if err := publishing.PublishContent(msg.BrandID, msg.ContentID); err != nil {
			log.Printf("scheduled_publish_sqs: publish failed for %s/%s: %v",
				msg.BrandID, msg.ContentID, err)
		}
	}
	return nil
}

func main() {
	lambda.Start(handler)
}
