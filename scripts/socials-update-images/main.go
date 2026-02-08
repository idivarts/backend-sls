package main

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/scripts/socials-update-images/sui"
)

func main() {
	// Run as an AWS Lambda handler
	lambda.Start(handler)
}

func handler(ctx context.Context) (string, error) {
	start := time.Now().UnixMicro()
	log.Println("Lambda invocation start", start)
	executeOnAll()
	log.Println("Lambda invocation end", time.Now().UnixMicro())
	return "ok", nil
}

func executeOnAll() {
	startExecutionTime := time.Now().UnixMicro()
	log.Println("Start Execution", startExecutionTime)

	socials, err := trendlybq.SocialsN8N{}.GetPaginatedFromFirestore(0, 0)
	if err != nil {
		log.Println("Error ", err.Error())
		return
	}

	for i, v := range socials {
		socials[i] = *sui.MoveImagesToS3(&v)
		socials[i].LastUpdateTime = time.Now().UnixMicro()

		// Not saving to firestore as thats redundant. We anyway would be remiving all images url from the current export
		// socials[i].InsertToFirestore()
		log.Println("Done Social -", i, socials[i].LastUpdateTime, socials[i].ProfilePic)
	}

	log.Println("Total Socials", len(socials), startExecutionTime)
	err = trendlybq.SocialsN8N{}.InsertMultiple(socials)
	if err != nil {
		log.Println("Error While Inserting", err.Error())
		return
	}
	for _, v := range socials {
		v.UpdateMinified()
	}
	log.Println("Done All")
}
