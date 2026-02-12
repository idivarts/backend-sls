package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/idivarts/backend-sls/scripts/socials-add-entries/sui"
)

// TestEvaluateInstagram calls evaluateInstagram directly with a ScrapedSocial,
// exercising the full pipeline: Apify scrape -> translate -> S3 upload -> Gemini enrich -> DB save.
func TestEvaluateInstagram(t *testing.T) {
	os.Setenv("S3_BUCKET", "trendly-discovery-bucket")
	os.Setenv("S3_URL", "https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com")

	req := sui.ScrapedSocial{
		SocialType: "instagram",
		Username:   "virat.kohli", // replace with any public username for testing
	}
	req.Manual.Niches = []string{"cricket", "sports", "fitness"}
	req.Manual.QualityScore = 8

	err := evaluateInstagram(req)
	if err != nil {
		t.Fatalf("evaluateInstagram failed: %v", err)
	}
	t.Log("evaluateInstagram completed successfully")
}

// TestHandlerSQSEvent simulates a full SQS event as AWS Lambda would receive
// it, verifying the end-to-end flow from message ingestion to DB persistence.
func TestHandlerSQSEvent(t *testing.T) {
	os.Setenv("S3_BUCKET", "trendly-discovery-bucket")
	os.Setenv("S3_URL", "https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com")

	body := sui.ScrapedSocial{
		SocialType: "instagram",
		Username:   "virat.kohli", // replace with any public username for testing
	}
	body.Manual.Niches = []string{"cricket", "sports", "fitness"}
	body.Manual.QualityScore = 8

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal SQS body: %v", err)
	}

	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId:   "test-message-001",
				EventSource: "aws:sqs",
				Body:        string(bodyJSON),
			},
		},
	}

	err = handler(context.Background(), sqsEvent)
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	t.Log("SQS handler completed successfully")
}
