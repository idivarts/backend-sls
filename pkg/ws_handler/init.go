package wshandler

import (
	"os"

	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigatewaymanagementapi"
)

var (
	firestoreClient = firestoredb.Client
	apiClient       *apigatewaymanagementapi.ApiGatewayManagementApi
	tableName       = os.Getenv("WS_CONNECTION_TABLE")
)

func init() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	apiClient = apigatewaymanagementapi.New(sess, aws.NewConfig().WithEndpoint(os.Getenv("WS_GATEWAY_ENDPOINT")))
}
