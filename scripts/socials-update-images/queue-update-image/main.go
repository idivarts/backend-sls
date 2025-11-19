package main

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/scripts/socials-update-images/sui"
)

func main() {
	// Run as an AWS Lambda handler
	lambda.Start(handler)
}

func handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	var err error
	for _, message := range sqsEvent.Records {
		fmt.Printf("The message %s for event source %s = %s \n", message.MessageId, message.EventSource, message.Body)
		err = uploadImage(message.Body)
	}
	return err
}

func uploadImage(socialId string) error {
	social := &trendlybq.Socials{}
	err := social.GetByIdFromFirestore(socialId)
	if err != nil {
		return err
	}

	social = sui.MoveImagesToS3(social)
	social.LastUpdateTime = time.Now().UnixMicro()

	social.InsertToFirestore()

	return nil
}
