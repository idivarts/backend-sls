package wshandler

import (
	"log"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/apigatewaymanagementapi"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func SendToConnection(connectionID *string, data string) {
	_, err := apiClient.PostToConnection(&apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: connectionID,
		Data:         []byte(data),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == apigatewaymanagementapi.ErrCodeGoneException {
			if err := trendlymodels.DeleteWebsocketConnection(*connectionID); err != nil {
				log.Printf("Failed to delete stale connection: %v", err)
			}
		} else {
			log.Printf("Failed to send message to connection %s: %v", *connectionID, err)
		}
	}
}
