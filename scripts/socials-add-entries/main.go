package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
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
	// social := &trendlyrdb.Socials{}
	// err := social.GetByIdFromFirestore(socialId)
	// if err != nil {
	// 	log.Println("Error in getting social by id:", socialId, " error:", err.Error())
	// 	return err
	// }

	// social = sui.MoveImagesToS3(social)
	// social.LastUpdateTime = time.Now().UnixMicro()

	// social.InsertToFirestore(true)

	return nil
}
