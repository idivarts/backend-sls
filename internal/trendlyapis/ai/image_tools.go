package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// moduleContent / moduleChat are the conversation modules in which the AI chat
// (AIChatPanel) can both see images (vision input) and produce them. Image
// generation is offered on the content-creation surfaces (content detail +
// calendar); the text-workflow modules (onboarding, strategy) deliberately do
// not get the generate_image tool so they stay focused.
// moduleContent is the content-detail chat module (moduleCalendar lives in
// calendar_tools.go). Both unlock image generation.
const moduleContent = "content"

const toolGenerateImage = "generate_image"

// moduleHasImageGen reports whether a module's chat may call generate_image.
func moduleHasImageGen(module string) bool {
	return module == moduleContent || module == moduleCalendar
}

// imageGenServerTools is the chat-side image tool, attached to content/calendar
// conversations. The model calls it to create or edit an image; the backend
// generates + uploads to S3 and returns the resulting URLs both as the tool
// result (so the model can keep talking about them) and to the caller (so they
// land on the assistant message's Images).
func imageGenServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolGenerateImage,
			"Generate or edit an image for the user. Call this when the user asks you to "+
				"create, generate, draw, design, or edit an image/visual/graphic. Provide a "+
				"detailed visual prompt. To EDIT or transform an existing image (one the user "+
				"attached this turn, or an image already on the content), pass its URL(s) in "+
				"inputImages for image-to-image. Do NOT call this for non-visual requests.",
			openrouter.ObjectSchema(map[string]any{
				"prompt": openrouter.StringProp("A detailed description of the image to generate or the edit to apply."),
				"aspectRatio": openrouter.EnumProp(
					"Aspect ratio of the image.",
					[]string{"1:1", "4:5", "16:9", "9:16"},
				),
				"inputImages": openrouter.ArrayProp(
					"Optional URLs of base/reference images to edit or build from (image-to-image). "+
						"Use URLs the user attached this turn or image URLs from the content context.",
					openrouter.StringProp("An https image URL."),
				),
			}, []string{"prompt"}),
		),
	}
}

type generateImageArgs struct {
	Prompt      string   `json:"prompt"`
	AspectRatio string   `json:"aspectRatio"`
	InputImages []string `json:"inputImages"`
}

// runChatImageTool executes a generate_image call inside the chat agentic loop.
// It returns the JSON tool result fed back to the model, the resulting image
// URLs (to attach to the assistant message), and any hard error. Plan/token
// gating is enforced here (STANDING RULE — backend gate), and an upgrade_required
// frame is pushed to the client so the UI can react in-context; the tool result
// also tells the model so it can explain to the user.
func runChatImageTool(ctx context.Context, brandID, orgID, connID, convID, requestedModel, arguments string) (string, []string, error) {
	var a generateImageArgs
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil, nil
		}
	}
	prompt := strings.TrimSpace(a.Prompt)
	if prompt == "" {
		return jsonResult(map[string]any{"ok": false, "error": "prompt is required"}), nil, nil
	}

	// Premium gate: image generation needs a plan that unlocks an image model.
	model, locked := pickModel(ctx, brandID, openrouter.TaskImage, requestedModel)
	if locked {
		wsSend(connID, map[string]any{
			"type":           "upgrade_required",
			"conversationId": convID,
			"task":           string(openrouter.TaskImage),
		})
		return jsonResult(map[string]any{
			"ok":    false,
			"error": "image generation needs a higher plan; tell the user to upgrade to generate images",
		}), nil, nil
	}
	if aiTokensExhausted(orgID) {
		wsSend(connID, map[string]any{
			"type":           "upgrade_required",
			"reason":         "tokens_exhausted",
			"conversationId": convID,
			"task":           string(openrouter.TaskImage),
		})
		return jsonResult(map[string]any{
			"ok":    false,
			"error": "the brand is out of AI tokens this month; tell the user to upgrade or add a top-up",
		}), nil, nil
	}

	size := aspectToSize(a.AspectRatio)
	resp, usage, err := openrouter.GenerateImage(ctx, openrouter.ImageRequest{
		Model:       model,
		Prompt:      prompt,
		Size:        size,
		N:           1,
		InputImages: cleanStrings(a.InputImages),
	})
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "image generation failed: " + err.Error()}), nil, err
	}
	if usage != nil {
		meterAIUsage(orgID, &openrouter.Usage{Cost: usage.Cost})
	}

	var urls []string
	for _, d := range resp.Data {
		url := ""
		if d.URL != "" {
			if s3url, uerr := uploadFromURL(d.URL, brandID); uerr == nil {
				url = s3url
			}
		} else if d.B64JSON != "" {
			if s3url, uerr := uploadBase64Image(d.B64JSON, brandID); uerr == nil {
				url = s3url
			}
		}
		if url != "" {
			urls = append(urls, url)
		}
	}
	if len(urls) == 0 {
		return jsonResult(map[string]any{"ok": false, "error": "image upload failed"}), nil, nil
	}

	// Push the images to the client immediately so the assistant bubble can show
	// them before the committed Firestore doc syncs.
	wsSend(connID, map[string]any{
		"type":           "chat_image",
		"conversationId": convID,
		"images":         urls,
	})

	return jsonResult(map[string]any{
		"ok":     true,
		"images": urls,
		"note":   fmt.Sprintf("%d image(s) generated and shown to the user; reference them naturally in your reply.", len(urls)),
	}), urls, nil
}
