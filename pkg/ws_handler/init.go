package wshandler

import (
	dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigatewaymanagementapi"
)

var (
	dynamoClient = dynamodbhandler.Client
	apiClient    *apigatewaymanagementapi.ApiGatewayManagementApi
)

func init() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	apiClient = apigatewaymanagementapi.New(sess)
}
