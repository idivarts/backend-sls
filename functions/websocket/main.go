package main

import (
	"github.com/TrendsHub/th-backend/internal/websocket"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(websocket.Handler)
}
