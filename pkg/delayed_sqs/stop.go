package delayedsqs

import (
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sfn"
)

func StopExecutions(execArns *string) {
	if execArns == nil {
		return
	}
	_, err := svc.StopExecution(&sfn.StopExecutionInput{
		Cause:        aws.String("No longer needed to execute this state"),
		Error:        aws.String("error.noLongerNeeded"),
		ExecutionArn: execArns,
	})
	if err != nil {
		log.Println("Issue while close Execution", *execArns, err.Error())
	}

}
