package websocket

import (
	"os"

	dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"
)

var (
	dynamoClient = dynamodbhandler.Client
	tableName    = os.Getenv("WS_CONNECTION_TABLE")
)
