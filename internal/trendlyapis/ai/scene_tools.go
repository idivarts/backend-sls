package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
	"github.com/idivarts/backend-sls/pkg/scenegraph"
)

// The AI-Studio scene tools make the chat AI the editor: it emits a semantic
// scene document (generate_scene) and translates pinned comments into a fixed
// vocabulary of structured ops (apply_scene_edits). Both persist an immutable
// scene revision and update the content's sceneRef; the frontend renders the
// scene and the render worker produces the final PNG/MP4.

const (
	toolGenerateScene   = "generate_scene"
	toolApplySceneEdits = "apply_scene_edits"
)

// moduleHasStudio reports whether a module's chat may drive the scene editor.
func moduleHasStudio(module string) bool {
	return module == moduleContent
}

func sceneServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolGenerateScene,
			"Create an editable SCENE for this content's image or video. Prefer this over "+
				"generate_image when the user wants an on-brand designed post (headline + "+
				"visual + logo) rather than a raw picture. Elements live in semantic ZONES "+
				"(top/center/bottom), never x/y. Provide headline/subhead/cta and format, or a "+
				"full `scene` document following the scene schema. The scene renders on-brand "+
				"automatically.",
			openrouter.ObjectSchema(map[string]any{
				"docType":  openrouter.EnumProp("image or video.", []string{"image", "video"}),
				"format":   openrouter.EnumProp("Content format.", []string{"post", "reel", "story", "video", "carousel"}),
				"headline": openrouter.StringProp("The main headline text."),
				"subhead":  openrouter.StringProp("Optional supporting subhead."),
				"cta":      openrouter.StringProp("Optional call-to-action (video outro)."),
				"scene":    openrouter.StringProp("Optional: a full scene document as a JSON string, overriding the seeded scene."),
			}, []string{}),
		),
		openrouter.NewFunctionTool(
			toolApplySceneEdits,
			"Apply edits to the current scene from the user's pinned comments/instructions. "+
				"Translate each instruction into ONE op from the fixed vocabulary — do NOT "+
				"rewrite the whole scene. Ops: setText(id,text), moveToZone(id,zone), "+
				"setImage(id,src), restyle(id,style), delete(id), duplicate(id), "+
				"setBackground(background), addElement(element), setDuration(sceneId,seconds), "+
				"setTransition(a,b,type), setAnimation(id,type), reorderScene(sceneId,index), "+
				"setAudio(type,brief), enableCaptions(source), trim(id,in,out).",
			openrouter.ObjectSchema(map[string]any{
				"ops": openrouter.ArrayProp(
					"The ordered list of ops to apply, each an object with an `op` field plus its params.",
					openrouter.ObjectSchema(map[string]any{
						"op":   openrouter.StringProp("The op name."),
						"id":   openrouter.StringProp("Target element id."),
						"text": openrouter.StringProp("Text for setText."),
						"zone": openrouter.StringProp("Zone for moveToZone."),
					}, []string{"op"}),
				),
			}, []string{"ops"}),
		),
	}
}

type generateSceneArgs struct {
	Scene    json.RawMessage `json:"scene"`
	DocType  string          `json:"docType"`
	Format   string          `json:"format"`
	Headline string          `json:"headline"`
	Subhead  string          `json:"subhead"`
	Cta      string          `json:"cta"`
}

// runGenerateScene builds/accepts a scene, validates it, persists a revision and
// points the content at it. contentID is the chat's contextID.
func runGenerateScene(ctx context.Context, brandID, contentID, arguments string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	var a generateSceneArgs
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil
		}
	}

	var doc *scenegraph.Document
	if len(a.Scene) > 0 && !isJSONNull(a.Scene) {
		var d scenegraph.Document
		raw := unwrapJSONString(a.Scene)
		if err := json.Unmarshal(raw, &d); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "scene JSON invalid: " + err.Error()}), nil
		}
		doc = &d
	} else {
		size := scenegraph.SizePost
		switch a.Format {
		case "reel", "story":
			size = scenegraph.SizeReel
		case "video":
			size = scenegraph.SizeVideo
		}
		bk := brandKit(brandID)
		if a.DocType == "video" {
			doc = scenegraph.SeedVideoScene(size, a.Headline, a.Cta, bk)
		} else {
			doc = scenegraph.SeedImageScene(size, a.Headline, a.Subhead, bk)
		}
	}

	if err := scenegraph.ValidateDocument(doc); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error(),
			"note": "fix the scene to match the schema and try again"}), nil
	}

	revID, boxes, err := persistScene(brandID, contentID, doc, "generate", "")
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), err
	}
	return jsonResult(map[string]any{
		"ok":         true,
		"revisionId": revID,
		"elements":   len(doc.Elements),
		"boxes":      len(boxes),
		"note":       "scene created and shown to the user; describe it briefly and invite them to edit text or pin comments.",
	}), nil
}

type applyEditsArgs struct {
	Ops json.RawMessage `json:"ops"`
}

// runApplySceneEdits applies AI-authored ops to the current scene and stores a
// new revision (parented to the prior one, so the user can revert).
func runApplySceneEdits(ctx context.Context, brandID, contentID, arguments string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content.SceneRef == nil {
		return jsonResult(map[string]any{"ok": false, "error": "no scene to edit; call generate_scene first"}), nil
	}
	cur, err := trendlymodels.GetSceneRevision(brandID, contentID, content.SceneRef.RevisionID)
	if err != nil || cur == nil || cur.Graph == nil {
		return jsonResult(map[string]any{"ok": false, "error": "current scene not found"}), nil
	}

	var a applyEditsArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil
	}
	ops, err := scenegraph.ParseOps(a.Ops)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), nil
	}
	if problems := scenegraph.ValidateOps(cur.Graph, ops); len(problems) > 0 {
		return jsonResult(map[string]any{"ok": false, "error": "invalid ops", "problems": problems}), nil
	}

	applied, err := scenegraph.Apply(cur.Graph, ops)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), nil
	}
	diff := make([]string, 0, len(applied.Results))
	pending := 0
	for _, r := range applied.Results {
		diff = append(diff, r.Describe)
		if r.Pending {
			pending++
		}
	}
	revID, _, err := persistScene(brandID, contentID, applied.Document, "edit", cur.ID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), err
	}
	return jsonResult(map[string]any{
		"ok":         true,
		"revisionId": revID,
		"applied":    len(diff),
		"pending":    pending,
		"diff":       diff,
		"note":       "edits applied and re-rendered; the user can revert to the previous revision.",
	}), nil
}

// persistScene writes an immutable revision, computes hit-box layout, points the
// content at it, and enqueues a render (best-effort). Returns the revision id +
// layout boxes.
func persistScene(brandID, contentID string, doc *scenegraph.Document, origin, parentRev string) (string, []scenegraph.LayoutBox, error) {
	revID, err := trendlymodels.CreateSceneRevision(brandID, contentID, &trendlymodels.ContentSceneRevision{
		Graph:            doc,
		ParentRevisionID: parentRev,
		Origin:           origin,
	})
	if err != nil {
		return "", nil, err
	}
	layout := scenegraph.ComputeLayout(doc)
	ref := &trendlymodels.ContentSceneRef{
		RevisionID: revID,
		DocType:    string(doc.Type),
		Boxes:      layout.Boxes,
	}
	if err := trendlymodels.SetContentSceneRef(brandID, contentID, ref); err != nil {
		return "", nil, err
	}
	enqueueRender(brandID, contentID, revID, string(doc.Type))
	return revID, layout.Boxes, nil
}

// brandKit assembles a scene brand kit from the brand doc (logo from the brand
// image); colors/fonts fall back to Trendly brand tokens.
func brandKit(brandID string) scenegraph.BrandKit {
	bk := scenegraph.BrandKit{}
	if b, err := loadBrand(brandID); err == nil && b != nil && b.Image != nil {
		bk.LogoURL = *b.Image
	}
	return bk
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

// unwrapJSONString handles the case where the model passes the scene as a JSON
// STRING (escaped) rather than a nested object.
func unwrapJSONString(raw json.RawMessage) json.RawMessage {
	t := strings.TrimSpace(string(raw))
	if len(t) > 0 && t[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return json.RawMessage(s)
		}
	}
	return raw
}
