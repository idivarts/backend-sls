package sqshandler

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

var svc *sqs.SQS

func init() {
	// Create a new AWS session using default credentials
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	// Create an SQS service client
	svc = sqs.New(sess)
}

func SendToMessageQueue(message string, delayInSeconds int64) error {
	// Get environment variable value by key
	envValue := os.Getenv("SEND_MESSAGE_QUEUE_ARN")

	// Check if the environment variable is set
	if envValue == "" {
		fmt.Println("Environment variable is not set")
		return fmt.Errorf("Error")
	} else {
		fmt.Println("Environment variable value:", envValue)
	}
	// Specify the URL of your SQS queue
	queueURL := envValue

	// Send the message to the SQS queue
	sendMessageInput := &sqs.SendMessageInput{
		QueueUrl:     &queueURL,
		MessageBody:  &message,
		DelaySeconds: &delayInSeconds,
	}

	_, err := svc.SendMessage(sendMessageInput)
	if err != nil {
		fmt.Println("Error sending message to SQS:", err)
		return err
	}

	fmt.Println("Message sent to SQS successfully")
	return nil
}
