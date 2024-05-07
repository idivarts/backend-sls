package sqsevents

const (
	SEND_MESSAGE SQSEvents = "sendMessage"
	RUN_OPENAI   SQSEvents = "run"
)

type SQSEvents string
