package ai

import (
	"encoding/json"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// Client tools are function calls the model makes to render an answer control on
// the user's screen. Unlike server tools, they are NOT executed on the backend —
// when the model calls one, we turn the arguments into an AIControl, push it to
// the client over the websocket, and END the turn. The user's selection comes
// back as an ordinary follow-up message. These tools are available in EVERY
// module so any conversation can ask option- or input-based questions.
const (
	toolAskOptions = "ask_options"
	toolAskInput   = "ask_input"
)

// clientTools returns the answer-control tools attached to every chat request.
func clientTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolAskOptions,
			"Ask the user a question they answer by picking from a fixed set of options. "+
				"Use this whenever the answer is one of a small, known set (e.g. a category, "+
				"a yes/no, a tier). Prefer this over free text for constrained choices.",
			openrouter.ObjectSchema(map[string]any{
				"question": openrouter.StringProp("The question to show above the options."),
				"selectionType": openrouter.EnumProp(
					"\"single\" if the user may pick exactly one option, \"multi\" if several.",
					[]string{"single", "multi"},
				),
				"options": map[string]any{
					"type":        "array",
					"description": "The selectable options (2-8). Use short, human-readable labels.",
					"items":       map[string]any{"type": "string"},
				},
				"allowCustom": map[string]any{
					"type":        "boolean",
					"description": "If true, the user may also type their own answer instead of picking an option.",
				},
			}, []string{"question", "selectionType", "options"}),
		),
		openrouter.NewFunctionTool(
			toolAskInput,
			"Ask the user for a single typed, format-sensitive value (e.g. a phone "+
				"number, a website URL, an email). Use this instead of free text whenever "+
				"the answer needs a specific format — it renders the right keyboard and "+
				"validates the value before it is accepted.",
			openrouter.ObjectSchema(map[string]any{
				"question": openrouter.StringProp("The prompt to show above the input field."),
				"inputType": openrouter.EnumProp(
					"The kind of value being requested (drives keyboard + validation).",
					[]string{"text", "phone", "url", "email"},
				),
				"placeholder": openrouter.StringProp("Optional placeholder text for the field."),
				"optional": map[string]any{
					"type":        "boolean",
					"description": "If true, the user may skip this field.",
				},
			}, []string{"question", "inputType"}),
		),
	}
}

func isClientTool(name string) bool {
	return name == toolAskOptions || name == toolAskInput
}

// askOptionsArgs / askInputArgs mirror the tool parameter schemas above.
type askOptionsArgs struct {
	Question      string   `json:"question"`
	SelectionType string   `json:"selectionType"`
	Options       []string `json:"options"`
	AllowCustom   bool     `json:"allowCustom"`
}

type askInputArgs struct {
	Question    string `json:"question"`
	InputType   string `json:"inputType"`
	Placeholder string `json:"placeholder"`
	Optional    bool   `json:"optional"`
}

// buildControl converts a client tool call into an AIControl plus the question
// text that should be shown as the assistant message body. ok is false if the
// call is not a recognised client tool or its arguments are unusable.
func buildControl(call openrouter.ToolCall) (control *trendlymodels.AIControl, question string, ok bool) {
	switch call.Function.Name {
	case toolAskOptions:
		var a askOptionsArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &a); err != nil {
			return nil, "", false
		}
		if len(a.Options) == 0 {
			return nil, "", false
		}
		sel := a.SelectionType
		if sel != "multi" {
			sel = "single"
		}
		opts := make([]trendlymodels.AIControlOption, 0, len(a.Options))
		for _, o := range a.Options {
			o = strings.TrimSpace(o)
			if o == "" {
				continue
			}
			opts = append(opts, trendlymodels.AIControlOption{Label: o, Value: o})
		}
		if len(opts) == 0 {
			return nil, "", false
		}
		return &trendlymodels.AIControl{
			Kind:          "options",
			SelectionType: sel,
			Options:       opts,
			AllowCustom:   a.AllowCustom,
		}, a.Question, true

	case toolAskInput:
		var a askInputArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &a); err != nil {
			return nil, "", false
		}
		it := a.InputType
		switch it {
		case "phone", "url", "email", "text":
		default:
			it = "text"
		}
		return &trendlymodels.AIControl{
			Kind:        "input",
			InputType:   it,
			Placeholder: a.Placeholder,
			Optional:    a.Optional,
		}, a.Question, true
	}
	return nil, "", false
}
