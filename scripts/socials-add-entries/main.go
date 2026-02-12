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
	"github.com/idivarts/backend-sls/internal/openai/deduce"
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
		return EvaluateInstagram(req)
	}
	return nil
}
func EvaluateInstagram(req sui.ScrapedSocial) error {
	log.Println("Evaluating instagram", req)

	social := &trendlyrdb.Socials{}
	posts := []trendlyrdb.InstagramPost{}
	var instagramRaw interface{}

	if req.UseDatabase {
		err := social.GetInstagram(req.Username)
		if err != nil {
			return err
		}
		posts, err = trendlyrdb.InstagramPost{}.GetBySocialID(social.ID, 30)
		if err != nil {
			return err
		}
		instagramRaw = struct {
			*trendlyrdb.Socials
			Posts []trendlyrdb.InstagramPost `json:"reels"`
		}{
			Socials: social,
			Posts:   posts,
		}
	} else {
		// -> Calling api to scrape data
		instagramData, err := apify.GetInstagram([]string{req.Username}, true)
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

		social, posts = sui.TranslateInstagram(instagram, req)

		// -> Download all the images
		social, posts = sui.MoveImagesToS3(social, posts)
		instagramRaw = instagram
	}
	// -> Send Raw for estimations (with Bias input which were sent manually)
	enrichPayload := map[string]interface{}{
		"profile": instagramRaw,
	}
	if len(req.Manual.Niches) > 0 || req.Manual.QualityScore > 0 {
		bias := map[string]interface{}{}
		if len(req.Manual.Niches) > 0 {
			bias["suggestedNiches"] = req.Manual.Niches
		}
		if req.Manual.QualityScore > 0 {
			bias["suggestedQualityScore"] = req.Manual.QualityScore
		}
		enrichPayload["bias"] = bias
	}

	enrichJSON, err := json.Marshal(enrichPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal enrichment payload: %w", err)
	}

	enriched, err := deduce.EnrichInfluencer(string(enrichJSON))
	if err != nil {
		log.Println("Enrichment failed, continuing without AI fields:", err)
	} else {
		social.Gender = enriched.Gender
		social.Location = enriched.Location
		social.Niches = enriched.Niches
		if req.Manual.QualityScore > 0 {
			social.QualityScore = req.Manual.QualityScore
		} else {
			social.QualityScore = enriched.Quality
		}
	}

	// -> Save updated data in mysql
	err = social.Insert()
	if err != nil {
		return err
	}
	err = trendlyrdb.InstagramPost{}.InsertMultiple(posts)
	if err != nil {
		return err
	}

	log.Println("Instagram data saved successfully", social.ID)

	return nil
}
