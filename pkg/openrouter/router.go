package openrouter

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

var DefaultModelFor = map[TaskType]string{
	TaskChat:       "openai/gpt-4o",
	TaskQuickEdit:  "openai/gpt-4o",
	TaskCaption:    "openai/gpt-4o",
	TaskHashtag:    "openai/gpt-4o",
	TaskStrategy:   "anthropic/claude-opus-4",
	TaskScript:     "anthropic/claude-opus-4",
	TaskMultimodal: "google/gemini-2.5-pro",
	TaskReasoning:  "openai/o3",
	TaskImage:      "google/gemini-2.5-flash-image-preview",
}

func ResolveModel(task TaskType, requested string) string {
	if requested != "" && IsKnownModel(requested) {
		return requested
	}
	if def, ok := DefaultModelFor[task]; ok {
		return def
	}
	return "openai/gpt-4o"
}
