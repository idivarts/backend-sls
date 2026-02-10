package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/scripts/socials-add-entries/sui"
)

func main() {
	// Run as an AWS Lambda handler
	lambda.Start(handler)
}

func handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	var err error
	for _, message := range sqsEvent.Records {
		fmt.Printf("The message %s for event source %s = %s \n", message.MessageId, message.EventSource, message.Body)
		err = evaluateInput(message.Body)
	}
	return err
}
func evaluateInput(socialData string) error {
	var req sui.ScrapedSocial
	if err := json.Unmarshal([]byte(socialData), &req); err != nil {
		return err
	}

	log.Println("Evaluating input", req)
	if req.SocialType == "instagram" {
		return evaluateInstagram(req)
	}
	return nil
}
func evaluateInstagram(req sui.ScrapedSocial) error {
	log.Println("Evaluating instagram", req)

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
