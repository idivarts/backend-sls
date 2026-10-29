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
	// TaskDesign authors and inspects HTML designs. It is split out from
	// TaskChat because laying out a frame and timing a scene are spatial
	// reasoning, not conversation: on a fast chat model the text is fine and the
	// design overflows its slide. Every model allowed for it must be
	// vision-capable, since the design loop renders the result and looks at it.
	TaskDesign TaskType = "design"
	// TaskScore judges a piece against the virality rubric, one dimension per
	// call. It is split out from TaskChat because the work is the opposite of
	// conversation: short, structured, repeated five times per run, and valued
	// for CONSISTENCY rather than flair. A cheap fast model used at low
	// temperature scores a rubric more reproducibly than a reasoning model
	// does, and run-to-run stability is the thing the score lives or dies on.
	TaskScore TaskType = "score"
)

// AllTaskTypes is the canonical list of tasks, used when seeding/validating config.
var AllTaskTypes = []TaskType{
	TaskChat, TaskQuickEdit, TaskCaption, TaskHashtag, TaskStrategy,
	TaskScript, TaskMultimodal, TaskReasoning, TaskImage, TaskDesign, TaskScore,
}
