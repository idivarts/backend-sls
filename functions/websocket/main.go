package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/websocket"
)

func main() {
	lambda.Start(websocket.Handler)
}
