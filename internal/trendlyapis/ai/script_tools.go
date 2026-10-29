package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// Script read/write for the content chat.
//
// The design surface has been able to edit its own document since the AI Studio
// shipped; the script never could. Asking the chat to tighten a hook produced a
// good revision in the conversation and nothing in the editor, leaving the user
// to copy it across by hand — which also made the virality panel's "Fix with AI"
// button only half a feature.
//
// These mirror get_design_html / apply_design_edits deliberately, including the
// snippet-edit model: a script is a few hundred words and re-typing all of it to
// change one line is how a scene quietly disappears.
const (
	toolGetScript    = "get_script"
	toolUpdateScript = "update_script"
)

type scriptEditArgs struct {
	Edits  []designEdit `json:"edits"`
	Script string       `json:"script"`
}

func scriptServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolGetScript,
			"Read the content's current script and timeline, verbatim. Call this BEFORE "+
				"update_script — the script is not in your context, and an edit anchored to "+
				"a remembered version will not apply.",
			openrouter.ObjectSchema(map[string]any{}, []string{}),
		),
		openrouter.NewFunctionTool(
			toolUpdateScript,
			"Rewrite the content's script and timeline. The change lands in the editor the "+
				"user is looking at, so make exactly the change they asked for.\n"+
				"PREFER `edits`: a list of {oldText, newText} replacements applied in order. "+
				"Each oldText must appear EXACTLY ONCE in the script — if a phrase repeats, "+
				"include enough surrounding text to make it unique. Send every change in ONE "+
				"call; they apply together or not at all.\n"+
				"The script is usually HTML from a rich-text editor. Anchor your oldText on "+
				"the exact characters get_script returned, tags included, and change only the "+
				"words — rewriting the markup loses the user's formatting.\n"+
				"Use `script` (a full replacement) only when the whole thing is being "+
				"rewritten, and then keep the same HTML structure.",
			openrouter.ObjectSchema(map[string]any{
				"edits": map[string]any{
					"type": "array",
					"description": "Targeted replacements, applied in order. Preferred over `script`. " +
						"Each oldText must match the current script exactly and occur exactly once.",
					"items": openrouter.ObjectSchema(map[string]any{
						"oldText": openrouter.StringProp("The exact existing text to replace, copied verbatim from get_script."),
						"newText": openrouter.StringProp("What to put in its place. An empty string deletes the snippet."),
					}, []string{"oldText", "newText"}),
				},
				"script": openrouter.StringProp(
					"A FULL replacement script. Use only when rewriting throughout; prefer `edits`.",
				),
			}, []string{}),
		),
	}
}

// scriptLocked reports whether the content may still be edited. A scheduled or
// published piece is read-only in the app, and the chat must not be a way around
// that — an AI edit to something already live would be invisible and unasked for.
func scriptLocked(status string) bool {
	switch status {
	case "scheduled", "publishing", "posted", "partially_failed":
		return true
	}
	return false
}

func runGetScript(ctx context.Context, brandID, contentID string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return jsonResult(map[string]any{"ok": false, "error": "content not found"}), nil
	}
	out := map[string]any{
		"ok":     true,
		"script": content.Script,
		"empty":  strings.TrimSpace(content.Script) == "",
		"locked": scriptLocked(content.Status),
	}
	// A readable rendering alongside the verbatim one. The model anchors its
	// edits on `script`, but reading tag soup to decide what to change is how it
	// starts editing the markup instead of the words.
	if plain := plainScript(content.Script); plain != content.Script {
		out["plain"] = plain
	}
	return jsonResult(out), nil
}

func runUpdateScript(ctx context.Context, brandID, contentID, arguments string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return jsonResult(map[string]any{"ok": false, "error": "content not found"}), nil
	}
	if scriptLocked(content.Status) {
		return jsonResult(map[string]any{
			"ok": false,
			"error": "this content is " + content.Status +
				" and is read-only. Tell the user it must be unscheduled before the script can change.",
		}), nil
	}

	var a scriptEditArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil
	}

	var next string
	switch {
	case len(a.Edits) > 0:
		patched, err := applySnippetEdits(content.Script, a.Edits)
		if err != nil {
			// Nothing was written — the model re-reads and fixes its anchor.
			return jsonResult(map[string]any{"ok": false, "error": err.Error()}), nil
		}
		next = patched
	case strings.TrimSpace(a.Script) != "":
		next = a.Script
	default:
		return jsonResult(map[string]any{
			"ok":    false,
			"error": "nothing to apply: send `edits` (preferred) or a full `script` replacement",
		}), nil
	}

	if strings.TrimSpace(plainScript(next)) == "" {
		return jsonResult(map[string]any{
			"ok":    false,
			"error": "that would leave the script empty. If the user asked to clear it, confirm with them first.",
		}), nil
	}

	if err := trendlymodels.UpdateContentFields(brandID, contentID,
		map[string]any{"script": next}); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), err
	}

	// The word count is the one number worth returning: it is how the model
	// notices it has accidentally rewritten the whole script when asked for one
	// line, and it costs nothing.
	words := len(strings.Fields(plainScript(next)))
	return jsonResult(map[string]any{
		"ok":        true,
		"words":     words,
		"applied":   len(a.Edits),
		"message":   fmt.Sprintf("Script updated (%d words). It is now live in the user's editor.", words),
		"contentId": contentID,
	}), nil
}

// scriptEditBrief is appended to the content chat's system prompt. The brief
// above it already carries the script's text, which is exactly why this is
// needed: a model that can see the script will happily "revise" it in the
// conversation and consider the job done.
const scriptEditBrief = "EDITING THE SCRIPT: when the user asks for any change to the script or " +
	"timeline — tighten the hook, cut a scene, change the CTA, rewrite a line — you must " +
	"actually apply it with update_script. Writing the revision out in your reply is NOT " +
	"doing it: the user is looking at the editor, and nothing you say in chat reaches it.\n" +
	"Call get_script first, every time. The script shown in the brief above may be the " +
	"user's unsaved draft, while update_script patches what is stored, so an edit anchored " +
	"on the brief can fail to apply. get_script returns the exact characters to anchor on.\n" +
	"Then reply with what you changed and why, in a sentence or two — not the whole script " +
	"again. They can see it."
