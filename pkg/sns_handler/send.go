package snshandler

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
)

var svc *sns.SNS

func init() {
	// Create a new AWS session using default credentials
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	// Create an SQS service client
	svc = sns.New(sess)
}

func Send(message string, delayInSeconds int64) error {
	// Get environment variable value by key
	envValue := os.Getenv("SEND_MESSAGE_QUEUE_ARN")

	// Check if the environment variable is set
	if envValue == "" {
		fmt.Println("Environment variable is not set")
		return fmt.Errorf("Error")
	} else {
		fmt.Println("Environment variable value:", envValue)
	}
	// Specify the ARN of your SNS topic
	topicARN := "your-sns-topic-arn"

	// Create message attributes including the delay
	messageAttributes := map[string]*sns.MessageAttributeValue{
		"DelaySeconds": {
			DataType:    aws.String("Number"),
			StringValue: aws.String(fmt.Sprintf("%d", delayInSeconds)),
		},
	}
	// Publish the message to the SNS topic
	result, err := svc.Publish(&sns.PublishInput{
		Message:           aws.String(message),
		TopicArn:          aws.String(topicARN),
		MessageAttributes: messageAttributes,
	})

	if err != nil {
		fmt.Println("Error publishing message to SNS:", err)
		return err
	}

	fmt.Println("Message sent to SNS successfully", result.GoString())
	return nil
}
