package sqsevents

const (
	SEND_MESSAGE SQSEvents = "sendMessage"
	RUN_OPENAI   SQSEvents = "run"
	REMINDER     SQSEvents = "reminder"
)

type SQSEvents string
