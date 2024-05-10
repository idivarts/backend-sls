package delayedsqs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sfn"
)

var svc *sfn.SFN
var stateMachineARN = os.Getenv("DELAY_STATE_FUNCTION")

func init() {
	// Create a new AWS session using default credentials
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	// Create an SQS service client
	svc = sfn.New(sess)
}

type SFNMessage struct {
	TopicARN     string `json:"topic"`
	DelaySeconds int64  `json:"delay_seconds"`
	Message      string `json:"message"`
}

func Send(message string, delayInSeconds int64) (*sfn.StartExecutionOutput, error) {

	// Specify the ARN of your Step Functions state machine
	topicARN := os.Getenv("SEND_MESSAGE_QUEUE_ARN")

	// Specify input data for the state machine (if needed)
	inputObj := SFNMessage{
		TopicARN:     topicARN,
		DelaySeconds: delayInSeconds,
		Message:      message,
	}
	input, err := json.Marshal(&inputObj)
	if err != nil {
		return nil, err
	}

	log.Println("Input Object", inputObj)

	// Execute the state machine
	result, err := svc.StartExecutionWithContext(context.TODO(), &sfn.StartExecutionInput{
		StateMachineArn: aws.String(stateMachineARN),
		Input:           aws.String(string(input)),
	})

	// svc.StopExecution(&sfn.StopExecutionInput{

	// })
	if err != nil {
		fmt.Println("Error starting execution:", err)
		return nil, err
	}

	fmt.Println("Execution started successfully:", *result.ExecutionArn)
	return result, nil
}
