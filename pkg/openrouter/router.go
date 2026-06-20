package openrouter

// TaskType identifies an AI task/module. Each task maps to an ordered list of
// allowed models in the registry (see models.go). The model data + per-task
// lists now live in Firestore (collection "ai_config"); this file only declares
// the task identifiers used across the AI handlers.
type TaskType string

const (
	TaskChat       TaskType = "chat"
	TaskQuickEdit  TaskType = "quick_edit"
	TaskCaption    TaskType = "caption"
	TaskHashtag    TaskType = "hashtag"
	TaskStrategy   TaskType = "strategy"
	TaskScript     TaskType = "script"
	TaskMultimodal TaskType = "multimodal"
	TaskReasoning  TaskType = "reasoning"
	TaskImage      TaskType = "image"
)

// AllTaskTypes is the canonical list of tasks, used when seeding/validating config.
var AllTaskTypes = []TaskType{
	TaskChat, TaskQuickEdit, TaskCaption, TaskHashtag, TaskStrategy,
	TaskScript, TaskMultimodal, TaskReasoning, TaskImage,
}
