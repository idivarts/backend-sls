package wshandler

import (
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

func Broadcast(data string, tableName string) error {
	connections, err := dynamoClient.Scan(&dynamodb.ScanInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		log.Printf("Failed to scan connections: %v", err)
		return err
	}

	for _, item := range connections.Items {
		connectionID := item["connectionId"].S
		SendToConnection(connectionID, data, tableName)
	}
	return nil
}
