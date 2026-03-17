package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	sui "github.com/idivarts/backend-sls/internal/utilities/scrapping-utility"
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
		return sui.EvaluateInstagram(req)
	}
	return nil
}
