package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	streamsqs "github.com/idivarts/backend-sls/internal/stream_sqs"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

func main() {
	// Non-Gin entry point, so it never passes through the Sentry middleware on
	// the shared Gin engine. Wrap captures errors and panics and — the part
	// that matters — flushes before Lambda freezes the environment.
	mysentry.Init()
	lambda.Start(mysentry.Wrap(streamsqs.Handler))
}
