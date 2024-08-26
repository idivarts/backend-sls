package wshandler

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/apigatewaymanagementapi"
)

func SendToConnection(connectionID *string, data string) {
	_, err := apiClient.PostToConnection(&apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: connectionID,
		Data:         []byte(data),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == apigatewaymanagementapi.ErrCodeGoneException {
			_, err := firestoreClient.Collection("websockets").Doc(*connectionID).Delete(context.Background())
			// .DeleteItem(&dynamodb.DeleteItemInput{
			// 	TableName: aws.String(tableName),
			// 	Key: map[string]*dynamodb.AttributeValue{
			// 		"connectionId": {
			// 			S: connectionID,
			// 		},
			// 	},
			// })
			if err != nil {
				log.Printf("Failed to delete stale connection: %v", err)
			}
		} else {
			log.Printf("Failed to send message to connection %s: %v", *connectionID, err)
		}
	}
}
