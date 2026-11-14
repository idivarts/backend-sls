package ai

// The Design Studio's own chat module.
//
// ── Why a module and not a new panel ──────────────────────────────────────
// AIChatPanel is already the conversation this studio wanted: streaming,
// a status line saying what the AI is doing, image attachments, threads
// that survive a reload. It runs on four surfaces. The studio's left-hand
// panel was a brief box and two buttons, which is the same conversation
// with everything interesting removed.
//
// So the work is a module, not a panel: a name both sides agree on, the
// right task behind it, the film's own shape in the prompt, and the tools
// that change the film rather than talk about it.
//
// ── ⚠️ The map that must agree with the frontend ─────────────────────────
// `taskForModule` in trendly-brands/hooks/use-ai-chat.ts decides which
// models the dropdown OFFERS; the switch in chat.go decides which task
// actually RUNS. They have disagreed before — the content surface showed
// the chat models while the backend ran design, so a model the task did
// not allow was silently swapped with the dropdown still naming it.
//
// ── ⚠️ And why this is not TaskChat ──────────────────────────────────────
// A design chat is a chat, so TaskChat is the obvious lane. It is the
// wrong one: openrouter.Resolve takes the FIRST allowed model the plan
// unlocks, TaskDesign leads with anthropic/claude-opus-4.8 and TaskChat
// leads with google/gemini-3.5-flash. Routing the studio to TaskChat
// would quietly drop it from Opus to Flash, with nothing on screen
// saying so and the films getting worse.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/studiojobs"
	"github.com/idivarts/backend-sls/pkg/mediastore"
	"github.com/idivarts/backend-sls/pkg/mysentry"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// moduleDesign is the Design Studio v3 surface: one conversation per film.
const moduleDesign = "design"

// moduleIsDesignStudio reports whether a conversation belongs to a v3 film.
//
// Separate from moduleHasStudio, which means "may drive the v2 HTML design
// editor" and is true only of the content surface. The two offer different
// tools against different documents, and one predicate for both is how the
// content chat would come to be handed a designId it has no business with.
func moduleIsDesignStudio(module string) bool {
	return module == moduleDesign
}

// TaskForModule is the lane a module's chat runs in.
//
// ── ⚠️ One function, three callers, and a frontend that must match ───────
// The task was picked independently in three places — creating a
// conversation, the websocket turn, and the HTTP turn — and two of them
// said TaskChat regardless of module. A design conversation was therefore
// CREATED with a chat model recorded on it and only switched to a design
// model when its first message arrived, so the dropdown opened naming a
// model the turn would not use.
//
// It mirrors taskForModule in trendly-brands/hooks/use-ai-chat.ts, which
// decides which models that dropdown OFFERS. The two have disagreed
// before, and the symptom is the worst kind: a model the task does not
// allow is silently swapped, and the dropdown goes on naming it.
func TaskForModule(module string) openrouter.TaskType {
	if moduleHasStudio(module) || moduleIsDesignStudio(module) {
		return openrouter.TaskDesign
	}
	return openrouter.TaskChat
}

const (
	toolReadHyperframesProject = "read_hyperframes_project"
	toolStartDesignAuthoring   = "start_design_authoring"
)

// designStudioTools are what the studio chat can do to the film.
//
// Read first, then write: write_hyperframes_project replaces the project
// WHOLE, so revising one headline without reading the current files first
// would throw away every other thing in the film. The tool description
// says so, because a model that has not been told will cheerfully write a
// one-scene reel over a six-scene one.
func designStudioTools() []openrouter.Tool {
	tools := []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolStartDesignAuthoring,
			"Write a film from scratch, for a design that has nothing in it yet. "+
				"Use this — NOT write_hyperframes_project — the first time a user "+
				"describes the film they want: it runs as a background job, so it "+
				"survives them closing the tab, and it finishes the film properly "+
				"(fetching the photographs and voiceovers it asked for, transcribing "+
				"captions, running the renderer's own check). It returns as soon as "+
				"the job is queued; say so and stop. The film appears beside the user "+
				"on its own.\n\n"+
				"Pass the user's own description as the brief, tidied but not "+
				"invented: it is stored on the design and is what they will see when "+
				"they come back to it.",
			openrouter.ObjectSchema(map[string]any{
				"designId": openrouter.StringProp("The design to write."),
				"brief": openrouter.StringProp(
					"What the film should be, in the user's own terms — length, " +
						"format, what it says, how it ends."),
			}, []string{"designId", "brief"}),
		),
		openrouter.NewFunctionTool(
			toolReadHyperframesProject,
			"Read the current files of a Design Studio v3 film. ALWAYS call this "+
				"before write_hyperframes_project when the film already exists: writing "+
				"replaces the project whole, so revising one line without reading first "+
				"throws away every other scene, every voiceover and every caption track. "+
				"Returns each file's path and content, plus the film's scenes and the "+
				"clips on its timeline.",
			openrouter.ObjectSchema(map[string]any{
				"designId": openrouter.StringProp("The design to read."),
			}, []string{"designId"}),
		),
	}
	return append(tools, hyperframesServerTools()...)
}

// runReadHyperframesProject answers the read tool.
func runReadHyperframesProject(ctx context.Context, brandID, arguments string) (string, error) {
	var a struct {
		DesignID string `json:"designId"`
	}
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not read the arguments"}), nil
	}
	a.DesignID = strings.TrimSpace(a.DesignID)
	if a.DesignID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "say which design to read"}), nil
	}

	d, err := trendlymodels.GetBrandDesign(ctx, brandID, a.DesignID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not read the design"}), nil
	}
	if d == nil {
		return jsonResult(map[string]any{"ok": false, "error": "no such design"}), nil
	}
	if d.ProjectPrefix == "" {
		// Not an error: a design nobody has written yet is a fact about the
		// design, and saying so lets the model go straight to writing one
		// rather than retrying a read that cannot work.
		return jsonResult(map[string]any{
			"ok":    true,
			"empty": true,
			"note":  "This design has nothing in it yet. Write a project for it.",
		}), nil
	}

	keys, lerr := mediastore.ListPrefix(ctx, mediastore.TargetAttachment, d.ProjectPrefix)
	if lerr != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not read this film's files"}), nil
	}
	out := make([]map[string]any, 0, len(keys))
	total := 0
	for _, key := range keys {
		rel := strings.TrimPrefix(strings.TrimPrefix(key, d.ProjectPrefix), "/")
		// ── Source only ──────────────────────────────────────────────────
		// The prefix also holds the film's resolved media — a voiceover WAV,
		// a stock photograph, the player's own runtime. Handing a model the
		// bytes of a WAV is a context window spent on nothing it can read,
		// and the media is already described by the clip list below.
		if !isProjectSource(rel) {
			out = append(out, map[string]any{"path": rel, "asset": true})
			continue
		}
		raw, gerr := mediastore.Get(ctx, mediastore.TargetAttachment, key)
		if gerr != nil {
			out = append(out, map[string]any{"path": rel, "error": "could not be read"})
			continue
		}
		// ⚠️ Bounded. A film's index.html runs to tens of kilobytes and the
		// conversation history shares the same context; an unbounded project
		// is how a chat runs out of room mid-revision and the model writes a
		// SHORTER film to fit — which looks like it decided to cut a scene.
		if total+len(raw) > maxProjectChars {
			out = append(out, map[string]any{
				"path":     rel,
				"skipped":  true,
				"bytes":    len(raw),
				"whyShort": "the project is larger than one reply can carry",
			})
			continue
		}
		total += len(raw)
		out = append(out, map[string]any{"path": rel, "content": string(raw)})
	}

	return jsonResult(map[string]any{
		"ok":         true,
		"designId":   a.DesignID,
		"files":      out,
		"durationMs": d.DurationMs,
		"scenes":     d.Scenes,
		"clips":      d.Clips,
	}), nil
}

// runStartDesignAuthoring queues the authoring job for an empty design.
//
// ── Why a job and not a tool that writes ─────────────────────────────────
// write_hyperframes_project writes the project inside this turn, which is
// right for a revision: it is one model call and the user is watching. A
// first authoring is not that. It fetches photographs and voiceovers,
// transcribes captions and runs the renderer's own check, and a tool call
// dies with the socket — so every disconnect would be a lost film the org
// has already paid a frontier call for. The queue is what makes that
// survivable, and ExecuteDesignAuthorJob's idempotency guard exists for
// exactly the redelivery that follows.
//
// The guards are the ones AuthorDesign uses, in the same order and for the
// same reason: this is the last place that can tell the user anything. Once
// the message is on the queue the only surface left is an authorError on
// the document, which reads as a fault rather than as a wall.
func runStartDesignAuthoring(ctx context.Context, brandID, arguments string) (string, error) {
	var a struct {
		DesignID string `json:"designId"`
		Brief    string `json:"brief"`
	}
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not read the arguments"}), nil
	}
	a.DesignID = strings.TrimSpace(a.DesignID)
	brief := strings.TrimSpace(a.Brief)
	if a.DesignID == "" || brief == "" {
		return jsonResult(map[string]any{"ok": false, "error": "say which design, and what the film should be"}), nil
	}

	d, err := trendlymodels.GetBrandDesign(ctx, brandID, a.DesignID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not read the design"}), nil
	}
	if d == nil {
		return jsonResult(map[string]any{"ok": false, "error": "no such design"}), nil
	}
	if d.AuthorStatus == trendlymodels.DesignAuthorQueued ||
		d.AuthorStatus == trendlymodels.DesignAuthorWriting {
		// Not an error. Telling the model it is already running stops it
		// queueing a second one, which would have two jobs writing the same
		// film and the second overwriting the first.
		return jsonResult(map[string]any{
			"ok":   true,
			"note": "This film is already being written. Tell the user it is on its way.",
		}), nil
	}
	if !studiojobs.Configured() {
		// ── Say that it is permanent, because the model will not guess ──
		// This read "writing is not available right now", and the model did
		// the natural thing with an unexplained failure: called it a
		// transient service issue and invited a retry. It is not transient.
		// It means this stage's chat function was never given
		// STUDIO_JOB_QUEUE_URL, so every retry fails identically and the
		// user re-sends the same prompt until they give up — which is
		// exactly what happened on dev.
		mysentry.Capture(fmt.Errorf("studio job queue not configured"), map[string]string{
			"op": "design.chat.author.unconfigured", "designId": a.DesignID,
		})
		return jsonResult(map[string]any{
			"ok":    false,
			"error": "writing_unconfigured",
			"say": "Writing films is not switched on in this environment. Retrying " +
				"will not help — say so plainly and do not offer to try again.",
		}), nil
	}

	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	if trendlymodels.TokensExhausted(orgID) {
		return jsonResult(map[string]any{
			"ok": false, "error": "upgrade_required", "reason": "tokens_exhausted",
			"say": "They are out of AI tokens this month and need a top-up before a film can be written.",
		}), nil
	}
	if model, locked := PickModelFor(ctx, brandID, openrouter.TaskDesign); locked || model == "" {
		return jsonResult(map[string]any{
			"ok": false, "error": "upgrade_required", "reason": "plan_locked",
			"say": "Writing films is on the paid plans.",
		}), nil
	}

	// Stored with the status rather than only carried on the queue message,
	// so the app can show what this design was asked to be.
	if qerr := trendlymodels.QueueBrandDesignAuthoring(ctx, brandID, a.DesignID, brief); qerr != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not start writing"}), nil
	}
	if eerr := studiojobs.Enqueue(studiojobs.Message{
		Type:     studiojobs.JobDesignAuthor,
		BrandID:  brandID,
		DesignID: a.DesignID,
		Brief:    brief,
	}); eerr != nil {
		mysentry.Capture(eerr, map[string]string{"op": "design.chat.author.enqueue", "designId": a.DesignID})
		_ = trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, a.DesignID,
			"We couldn't start writing this design. Please try again.")
		return jsonResult(map[string]any{"ok": false, "error": "could not start writing"}), nil
	}

	return jsonResult(map[string]any{
		"ok":     true,
		"status": "queued",
		"note": "Writing has started and will carry on even if they close the tab. " +
			"Say so in one short sentence and STOP — do not describe the film you " +
			"have not written yet, and do not call any other tool.",
	}), nil
}

// maxProjectChars bounds what one read hands back.
const maxProjectChars = 120_000

// isProjectSource reports whether a file in the prefix is part of the
// project the model writes, rather than media or the player beside it.
func isProjectSource(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, ".media/") || strings.HasPrefix(rel, "assets/") {
		return false
	}
	// player.html is written BY us beside the project, from the project —
	// handing it back would invite the model to edit the wrapper instead of
	// the film, and the next write would overwrite its edit anyway.
	if rel == "player.html" {
		return false
	}
	switch {
	case strings.HasSuffix(rel, ".html"),
		strings.HasSuffix(rel, ".css"),
		strings.HasSuffix(rel, ".js"),
		strings.HasSuffix(rel, ".json"):
		return true
	}
	return false
}

// designStudioBrief is the film, as the chat is told about it.
//
// A REFERENCE, deliberately: the shape of the film and what is selected,
// never the HTML. The content surface learned this the hard way — a design
// inlined into the system prompt truncates a long conversation, and the
// model then reasons about half a film. The HTML comes from the read tool,
// when the model actually needs it.
func designStudioBrief(brandID, designID string) string {
	if brandID == "" || designID == "" {
		return ""
	}
	d, err := trendlymodels.GetBrandDesign(context.Background(), brandID, designID)
	if err != nil || d == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("THE FILM ON SCREEN\n")
	fmt.Fprintf(&sb, "designId: %s\n", designID)
	if name := strings.TrimSpace(d.Name); name != "" {
		fmt.Fprintf(&sb, "name: %s\n", name)
	}
	if d.ProjectPrefix == "" {
		sb.WriteString("This design has NOTHING IN IT YET.\n" +
			"The user's first message is the brief. Call start_design_authoring " +
			"with it — not write_hyperframes_project — then say in one sentence " +
			"that it is being written, and stop.\n")
		return sb.String()
	}
	if d.DurationMs > 0 {
		fmt.Fprintf(&sb, "length: %.1fs\n", float64(d.DurationMs)/1000)
	}
	if d.Format.Width > 0 && d.Format.Height > 0 {
		fmt.Fprintf(&sb, "frame: %dx%d at %dfps\n", d.Format.Width, d.Format.Height, d.Format.FPS)
	}
	if len(d.Clips) > 0 {
		sb.WriteString("timeline (row · what · when):\n")
		for _, c := range d.Clips {
			label := c.Label
			if label == "" {
				label = c.ElementID
			}
			fmt.Fprintf(&sb, "  %d · %s · %s · %.1fs at %.1fs · #%s\n",
				c.TrackIndex, c.Kind, label,
				float64(c.DurationMs)/1000, float64(c.StartMs)/1000, c.ElementID)
		}
	} else if len(d.Scenes) > 0 {
		sb.WriteString("scenes:\n")
		for _, s := range d.Scenes {
			fmt.Fprintf(&sb, "  %s · %.1fs at %.1fs\n",
				s.ID, float64(s.DurationMs)/1000, float64(s.StartMs)/1000)
		}
	}
	sb.WriteString("\nTo change it: read_hyperframes_project first, then " +
		"write_hyperframes_project with the WHOLE project, the changed file(s) " +
		"rewritten and every other file passed back unchanged. Writing saves a " +
		"revision, so a change the user dislikes is one click back.\n")
	return sb.String()
}

// designStudioInstructions is how the studio chat is asked to behave.
//
// Short on purpose. The authoring contract is already in the write tool's
// description, which is where it belongs: a contract repeated in the
// system prompt is a contract that drifts from the one being enforced.
const designStudioInstructions = "\n\nYou are working inside the Design Studio on ONE film, with the user " +
	"watching it play beside this conversation. Keep replies short and about " +
	"their film. When they ask for a change, make it — read the project, write " +
	"it back, and say in a sentence what you changed. Do not describe a change " +
	"you have not made. If what they want is unclear enough that guessing would " +
	"waste a render, ask with ask_options rather than guessing.\n" +
	"The user may have something SELECTED on screen; when they do it is named " +
	"in their message, and 'make this bigger' means that element and nothing " +
	"else.\n" +
	"A film with nothing in it yet is written with start_design_authoring, " +
	"which runs as a job: it returns the moment it is queued, so say it is on " +
	"its way and stop. A film that already exists is changed by reading it and " +
	"writing it back."
