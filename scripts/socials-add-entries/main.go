package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/pkg/apify"
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

	// -> Calling api to scrape data
	instagramData, err := apify.GetInstagram([]string{req.Username})
	if err != nil {
		return err
	}
	log.Println("Instagram data", instagramData)

	if len(instagramData) == 0 {
		return errors.New("no instagram data found")
	}
	instagram := instagramData[0]
	if instagram.Username != req.Username {
		return errors.New("instagram username mismatch")
	}

	social, posts := sui.TranslateInstagram(instagram, req)

	err = social.Insert()
	if err != nil {
		return err
	}
	err = trendlyrdb.InstagramPost{}.InsertMultiple(posts)
	if err != nil {
		return err
	}

	// -> Download all the images

	// -> Send Raw for estimations (with Bias input which were sent manually)

	// -> Translate the data in Socials Data

	// -> Save translated data in mysql instantly

	return nil
}
