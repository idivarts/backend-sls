package main

import (
	"log"
	"os"

	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

func main() {
	os.Setenv("SEND_MESSAGE_QUEUE_ARN", "ScrapeImageQueue")
	err := sqshandler.SendToMessageQueue("5ed2ac8d-c4cf-5519-92f1-f93232dbcf16", 0)

	if err != nil {
		log.Println("Error ", err.Error())
	}
}
