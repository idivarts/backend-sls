package wshandler

import (
	"os"

	dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigatewaymanagementapi"
)

var (
	dynamoClient = dynamodbhandler.Client
	apiClient    *apigatewaymanagementapi.ApiGatewayManagementApi
	tableName    = os.Getenv("WS_CONNECTION_TABLE")
)

func init() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	apiClient = apigatewaymanagementapi.New(sess, aws.NewConfig().WithEndpoint(os.Getenv("WS_GATEWAY_ENDPOINT")))
}
