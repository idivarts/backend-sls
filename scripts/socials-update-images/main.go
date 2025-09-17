package main

import (
	"context"
	"log"
	"os"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/pkg/myquery"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
	"google.golang.org/api/iterator"
)

func main() {
	os.Setenv("SEND_MESSAGE_QUEUE_ARN", "ScrapeImageQueue")

	executeOnAll()
	// err := sqshandler.SendToMessageQueue("5ed2ac8d-c4cf-5519-92f1-f93232dbcf16", 0)
	// if err != nil {
	// 	log.Println("Error ", err.Error())
	// }
}

func executeOnAll() {
	q := myquery.Client.Query(`
    SELECT id
    FROM ` + trendlybq.SocialsFullTableName + `
	WHERE NOT STARTS_WITH(profile_pic, "https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com")
    LIMIT 500
	OFFSET 0
`)

	it, err := q.Read(context.Background())
	if err != nil {
		log.Println("Error ", err.Error())
		return
	}

	i := 0
	for {
		data := &trendlybq.Socials{}
		err = it.Next(data)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Println("Error ", err.Error())
			continue
		}
		err := sqshandler.SendToMessageQueue(data.ID, 0)
		if err != nil {
			log.Println("Error ", data.ID, err.Error())
			continue
		}
		log.Println("Done ", i, data.ID)
		i++
	}
	log.Println("Done All")

}
