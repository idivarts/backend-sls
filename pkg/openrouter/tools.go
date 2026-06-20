package openrouter

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func NewFunctionTool(name, description string, params map[string]any) Tool {
	return Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  params,
		},
	}
}

func ObjectSchema(props map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

func StringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func NumberProp(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func EnumProp(description string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

// ArrayProp builds a JSON-schema array property whose elements use the given
// item schema (e.g. StringProp / EnumProp).
func ArrayProp(description string, items map[string]any) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": items}
}
