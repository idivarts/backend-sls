package main

import (
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/scripts/socials-update-images/sui"
)

func main() {
	// os.Setenv("S3_BUCKET", "trendly-discovery-bucket")
	// os.Setenv("S3_URL", "https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com")

	executeOnAll()
}

func executeOnAll() {
	startExecutionTime := time.Now().UnixMicro()
	log.Println("Start Execution", startExecutionTime)

	socials, err := trendlybq.Socials{}.GetPaginatedFromFirestore(0, 700)
	if err != nil {
		log.Println("Error ", err.Error())
		return
	}
	for i, v := range socials {
		socials[i] = *sui.MoveImagesToS3(&v)
		socials[i].LastUpdateTime = time.Now().UnixMicro()

		socials[i].InsertToFirestore()
		log.Println("Done Social -", i, socials[i].LastUpdateTime, socials[i].ProfilePic)
	}

	log.Println("Total Socials", len(socials), startExecutionTime)
	err = trendlybq.Socials{}.InsertMultiple(socials)
	if err != nil {
		log.Println("Error While Inserting", err.Error())
		return
	}
	for _, v := range socials {
		v.UpdateMinified()
	}

	log.Println("Done All")
}
