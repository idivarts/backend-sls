package websocket

import dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"

var (
	dynamoClient = dynamodbhandler.Client
	tableName    = "websocketTable"
)
