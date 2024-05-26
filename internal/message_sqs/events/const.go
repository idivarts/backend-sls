package sqsevents

const (
	SEND_MESSAGE            SQSEvents = "sendMessage"
	RUN_OPENAI              SQSEvents = "run"
	REMINDER                SQSEvents = "reminder"
	CREATE_THREAD           SQSEvents = "createThread"
	CREATE_OR_UPDATE_THREAD SQSEvents = "createUpdateThread"
)

type SQSEvents string
