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

	// A queue ARN is not a secret, but "Environment variable value: ..." next
	// to the same line that printed an API key is the pattern worth retiring,
	// not just the one instance of it.
	if envValue == "" {
		return fmt.Errorf("sqs: SEND_MESSAGE_QUEUE_ARN is not set")
	}

	return SendToQueue(envValue, message, delayInSeconds)
}

// SendToQueue sends a message to an explicit SQS queue URL. Use this when a caller
// has its own queue (rather than the shared SEND_MESSAGE_QUEUE_ARN one).
func SendToQueue(queueURL, message string, delayInSeconds int64) error {
	if queueURL == "" {
		return fmt.Errorf("SendToQueue: empty queue URL")
	}

	sendMessageInput := &sqs.SendMessageInput{
		QueueUrl:     &queueURL,
		MessageBody:  &message,
		DelaySeconds: &delayInSeconds,
	}

	if _, err := svc.SendMessage(sendMessageInput); err != nil {
		fmt.Println("Error sending message to SQS:", err)
		return err
	}

	fmt.Println("Message sent to SQS successfully")
	return nil
}
