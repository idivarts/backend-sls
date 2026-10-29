package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
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
func runChatImageTool(ctx context.Context, brandID, orgID, contentID, connID, convID, requestedModel, arguments string) (string, []string, error) {
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
	// Fold the brand's imagery style (mood, treatment, palette) and its "avoid"
	// list into the prompt so generated visuals stay on-brand.
	prompt = designSystemImageryGuidance(loadDesignSystem(brandID), prompt)

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

	// Heartbeat before the slow, silent image-gen call: it lets the client show
	// "generating an image…" and keeps its streaming watchdog from tripping
	// during the gap (no token deltas flow while the image renders).
	wsSend(connID, map[string]any{
		"type":           "image_status",
		"conversationId": convID,
		"state":          "generating",
	})

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

	// A generated image belongs on the canvas, not in a chat bubble.
	//
	// Loose attachments are indistinguishable from an upload in the data, so an
	// image saved that way is publishable but can never be iterated on — you
	// cannot put a headline over it, animate it, or reuse it as a frame. Wrapping
	// it in a design document puts it in the design lane, where every one of
	// those is an ordinary edit.
	placed := placeImagesOnCanvas(brandID, contentID, urls)

	// Push the images to the client immediately so the assistant bubble can show
	// them before the committed Firestore doc syncs.
	wsSend(connID, map[string]any{
		"type":           "chat_image",
		"conversationId": convID,
		"images":         urls,
	})

	note := fmt.Sprintf("%d image(s) generated and shown to the user; reference them naturally in your reply.", len(urls))
	if placed {
		note += " They are now on the content's canvas as a design, so the user can caption or restyle them — " +
			"call " + toolGetDesignHTML + " before any edit."
	}
	return jsonResult(map[string]any{
		"ok":     true,
		"images": urls,
		"placed": placed,
		"note":   note,
	}), urls, nil
}

// placeImagesOnCanvas wraps freshly generated images in a design document so
// they land in the design lane. Reports whether it did.
//
// It deliberately does NOT touch a content that already has a design: once a
// canvas exists, "generate an image" almost always means "make me something to
// USE in this design", and silently replacing the user's laid-out work with a
// bare photo would destroy it. In that case the images stay in the chat, where
// the model can place them with apply_design_edits instead.
func placeImagesOnCanvas(brandID, contentID string, urls []string) bool {
	if contentID == "" || len(urls) == 0 {
		return false
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return false
	}
	if content.DesignRef != nil && content.DesignRef.RevisionID != "" {
		return false
	}
	format := content.ContentFormat
	// A live or text post has no media area at all, so a design document on one
	// would be invisible in the app and would still claim the design lane.
	if format == trendlymodels.ContentFormatLive || format == trendlymodels.ContentFormatText {
		return false
	}
	// Only a carousel has more than one slide. Writing N slides onto a single
	// post would publish the first and silently lose the rest.
	slides := len(urls)
	if format != trendlymodels.ContentFormatCarousel {
		slides = 1
		urls = urls[:1]
	}
	w, h := sizeForFormatPx(string(format))
	if _, err := persistDesign(
		brandID, contentID, imageDesignHTML(urls, w, h),
		w, h, slides, 0, "image", "generate", "",
	); err != nil {
		log.Printf("ai image-gen canvas placement: %v", err)
		return false
	}
	return true
}

// imageDesignHTML builds a minimal design document whose only content is the
// generated photography, one slide per image.
//
// The image is a real <img> rather than a CSS background: the render worker
// captures each [data-slide] as an element, and a background-image on a sized
// slide and an <img> with object-fit:cover produce the same pixels — but only
// the <img> carries a data-el the model can address later ("swap the hero
// photo") without rewriting the stylesheet.
func imageDesignHTML(urls []string, w, h int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div data-carousel style="width:%dpx">`, w)
	for i, u := range urls {
		fmt.Fprintf(&b,
			`<section data-slide style="position:relative;width:%dpx;height:%dpx;overflow:hidden;background:#000">`+
				`<img data-el="photo-%d" src="%s" alt="" style="position:absolute;inset:0;width:100%%;height:100%%;object-fit:cover">`+
				`</section>`,
			w, h, i+1, html.EscapeString(u))
	}
	b.WriteString(`</div>`)
	return wrapHTML(b.String())
}
