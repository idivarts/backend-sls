package trendlydiscovery

import sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"

func SendToSqs(socialId string) {
	sqshandler.SendToMessageQueue(socialId, 0)
}
