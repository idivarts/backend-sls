package director

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ── The plan ────────────────────────────────────────────────────────────────
//
// What the planner must return. It is a storyboard plus a build graph: the
// scenes a person will review, and the asset requests a run will execute.
//
// Planning is text and costs almost nothing; execution costs real money and
// takes minutes. Those two facts are far enough apart that the plan is a
// reviewable artefact rather than a hidden chain of tool calls — so this
// struct is also the thing the storyboard UI renders and the thing we store
// for "why did it choose that".

// Plan is one director run, before anything is spent.
type Plan struct {
	// Summary is one or two sentences a person reads first: the shape of the
	// video and the thinking behind it.
	Summary          string    `json:"summary"`
	Canvas           Canvas    `json:"canvas"`
	TransitionPreset string    `json:"transitionPreset"`
	Audio            AudioPlan `json:"audio"`
	// Cast and Locations are the subjects that must stay the same across
	// clips. They are entities rather than phrases repeated in prompts
	// because that is the only way an anchor can be approved once and a look
	// can be guaranteed to travel.
	Cast          []CastMember   `json:"cast,omitempty"`
	Locations     []Location     `json:"locations,omitempty"`
	Scenes        []ScenePlan    `json:"scenes"`
	AssetRequests []AssetRequest `json:"assetRequests"`
	// EstCostUSD is the planner's own total. It is recomputed from the asset
	// requests during validation and disagreements are reported, because a
	// model adding up its own costs is a model that will get it wrong.
	EstCostUSD float64 `json:"estCostUsd"`
	// Notes is where the planner says what it could not do, what it would do
	// with more budget, and anything it assumed.
	Notes []string `json:"notes,omitempty"`
}

// CastMember is a person who appears in more than one place and must look
// like the same person in all of them.
type CastMember struct {
	ID string `json:"id"`
	// Description is who they are: age, wardrobe, how they are framed.
	Description string `json:"description"`
	// Look is the photographic invariant — lens, light, time of day, grade.
	// It is appended VERBATIM to every prompt that features this person, which
	// is what stops the light changing between beats.
	Look string `json:"look"`
	// AnchorMediaID is an approved still of them that already exists.
	AnchorMediaID string `json:"anchorMediaId,omitempty"`
	// AnchorAssetID is an image request that will produce that still. Either
	// this or AnchorMediaID is required the moment they appear in more than
	// one generated clip — and a chain does not count as an anchor.
	AnchorAssetID string `json:"anchorAssetId,omitempty"`
}

// Location is a place that must look like the same place across clips.
type Location struct {
	ID            string `json:"id"`
	Description   string `json:"description"`
	Look          string `json:"look"`
	AnchorMediaID string `json:"anchorMediaId,omitempty"`
	AnchorAssetID string `json:"anchorAssetId,omitempty"`
}

type Canvas struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Fps    int    `json:"fps"`
	Aspect string `json:"aspect"`
}

// AudioPlan is the document-level sound. Per-scene narration lives on the
// scene, because a line belongs to the scene it narrates and has to survive a
// reorder.
type AudioPlan struct {
	// MusicApproach is "none", "generate" or "reuse".
	MusicApproach string `json:"musicApproach"`
	MusicModel    string `json:"musicModel,omitempty"`
	MusicPrompt   string `json:"musicPrompt,omitempty"`
	MusicMediaID  string `json:"musicMediaId,omitempty"`
	VoiceModel    string `json:"voiceModel,omitempty"`
	// VoiceID is the one voice the whole document speaks in. Identity is the
	// easy half of voice consistency and this is all it takes.
	VoiceID   string `json:"voiceId,omitempty"`
	DuckMusic bool   `json:"duckMusic"`
}

// ScenePlan is one beat, fully specified.
type ScenePlan struct {
	ID string `json:"id"`
	// Beat is what this scene is FOR, in the plan's own words. It is what the
	// placeholder shows on the timeline while the scene is still building, and
	// it is the most useful single field for a person reviewing a storyboard.
	Beat     string `json:"beat"`
	Approach string `json:"approach"`
	// Why is the justification. Required for a generated video, because that
	// is the expensive choice and it has to beat a cheaper one on the record.
	Why          string      `json:"why,omitempty"`
	DurationMs   int         `json:"durationMs"`
	TransitionIn Transition2 `json:"transitionIn"`
	Layers       []LayerPlan `json:"layers"`
	// Narration is the line of script this scene speaks, if any.
	Narration string `json:"narration,omitempty"`
	// NarrationSource is WHERE THAT LINE CAME FROM, and it is required
	// whenever there is a line.
	//
	// ⚠️ This is enforcement, not metadata. The brand's first rule is that a
	// claim has to be one we can stand behind, and a planner asked for a hooky
	// opener will otherwise write a confident sentence nobody approved.
	// Inferring provenance from the text is too brittle, and a planner allowed
	// only to quote writes worse timing — so the planner DECLARES, and the
	// validator checks the declaration against the material it was given.
	//
	// script | caption | brand | paraphrase | illustrative
	NarrationSource string `json:"narrationSource,omitempty"`
	// NarrationBrandFactID names which documented fact, for source "brand".
	NarrationBrandFactID string `json:"narrationBrandFactId,omitempty"`
	// FitToNarration makes the scene's final duration follow its measured
	// voiceover rather than the planned guess. The commonest way a render
	// comes out wrong is a voice that overruns its picture.
	FitToNarration bool `json:"fitToNarration,omitempty"`
	// ContinuesPrevious declares that this scene carries on the previous one
	// in the same unbroken moment. It is what licenses chaining: without it a
	// chain is being used to hold a face across separate beats, which is the
	// one thing chaining is bad at.
	ContinuesPrevious bool `json:"continuesPrevious,omitempty"`
	// VoiceID overrides the document voice for a genuine second speaker.
	VoiceID string `json:"voiceId,omitempty"`
	// VoiceWhy is why this scene speaks in a different voice. Required with
	// VoiceID, so a drifting voice cannot be introduced by accident.
	VoiceWhy string `json:"voiceWhy,omitempty"`
}

// Transition2 is the plan's transition (the catalogue type is Transition).
type Transition2 struct {
	Kind string `json:"kind"`
	Ms   int    `json:"ms"`
}

// LayerPlan is one layer of a scene. Exactly one of ReuseMediaID,
// AssetRequestID or HTMLBrief is set.
type LayerPlan struct {
	Kind string `json:"kind"` // media | html
	Slot string `json:"slot"`
	// ReuseMediaID points at something the brand already owns. This is the
	// cheapest possible layer and the planner is told to look for it first.
	ReuseMediaID string `json:"reuseMediaId,omitempty"`
	// AssetRequestID points at media this run will bring into existence.
	AssetRequestID string `json:"assetRequestId,omitempty"`
	// HTMLBrief is what a motion or composite layer should contain — the
	// instruction a scene generator will be given, not the HTML itself.
	HTMLBrief string `json:"htmlBrief,omitempty"`
	// TrimInMs and TrimOutMs take a window out of the source.
	//
	// These are what make the second consistency mechanism expressible: ONE
	// generation referenced by several scenes at different windows is exactly
	// consistent, because it is the same footage, and costs one generation
	// instead of three.
	TrimInMs  int     `json:"trimInMs,omitempty"`
	TrimOutMs int     `json:"trimOutMs,omitempty"`
	Volume    float64 `json:"volume,omitempty"`
}

// AssetRequest is a piece of media that does not exist yet: the "micro media"
// a plan declares and a run produces.
//
// It is a node rather than a side effect for three reasons: two scenes wanting
// the same clip produce one request, a request can resolve to an existing
// library row for nothing, and the media it makes records what made it.
type AssetRequest struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // image | video | voice | music | stock
	// Intent is what it is for, in words — "person laughing, mid-shot, warm
	// light". Separate from Prompt because the intent survives a model change.
	Intent    string `json:"intent"`
	Model     string `json:"model,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	DurationS int    `json:"durationS,omitempty"`
	Aspect    string `json:"aspect,omitempty"`
	// StartFrameFromAssetID chains this generation off another request's last
	// frame. This is how a run exceeds the per-call duration ceiling, and it
	// is the only reason a node is ever sequential rather than parallel.
	StartFrameFromAssetID string   `json:"startFrameFromAssetId,omitempty"`
	RefMediaIDs           []string `json:"refMediaIds,omitempty"`
	// ReuseCandidateIDs are library rows the planner thinks might serve
	// instead. The run tries these before spending.
	ReuseCandidateIDs []string `json:"reuseCandidateIds,omitempty"`
	// CastIDs and LocationID say whose look this inherits. A generation that
	// features a cast member must name them, or nothing can check that the
	// person stays the same.
	CastIDs    []string `json:"castIds,omitempty"`
	LocationID string   `json:"locationId,omitempty"`
	// Framing is the shot — "over-shoulder mid", "wide", "macro". Two requests
	// sharing a cast member, a place AND a framing are one generation and two
	// trims, which is both cheaper and exactly consistent.
	Framing string `json:"framing,omitempty"`
	// NativeAudio lets a video model emit its own sound. Absent means off,
	// which is the safe answer: a generated clip's own voice under our
	// narration is two voices at once, different in every clip.
	NativeAudio bool `json:"nativeAudio,omitempty"`
	// SceneID anchors a voice or sfx request to the scene it belongs to.
	//
	// Required for voice, and the reason is the whole audio design: narration
	// generated as ONE file for the whole video is narration anchored to
	// absolute time, so reordering two scenes desynchronises every word after
	// them. A line per scene moves with its scene and dies with it. Both
	// models tried here produced a single blob until the schema refused it.
	SceneID    string  `json:"sceneId,omitempty"`
	EstCostUSD float64 `json:"estCostUsd"`
}

// Problem is one reason a plan cannot be executed as written.
type Problem struct {
	Where   string `json:"where"`
	Message string `json:"message"`
	// Fatal problems stop a run. Non-fatal ones are shown on the storyboard
	// and are usually the planner being sloppy about something recoverable.
	Fatal bool `json:"fatal"`
}

func (p Problem) String() string {
	sev := "warn"
	if p.Fatal {
		sev = "FATAL"
	}
	return fmt.Sprintf("[%s] %s: %s", sev, p.Where, p.Message)
}

// Validate is the gate between a plan and any spending.
//
// This is the part that makes handing a model a capability manifest worth
// anything: the manifest tells it what it may ask for, and this refuses the
// plan when it asked anyway. Every rule here corresponds to a real failure —
// a provider 422, an unrenderable timeline, an empty wallet halfway through.
func (p Plan) Validate(m Manifest) []Problem {
	var out []Problem
	add := func(where, msg string, fatal bool) {
		out = append(out, Problem{Where: where, Message: msg, Fatal: fatal})
	}

	caps := m.Capabilities
	isCarousel := m.Content.Format == "carousel" || m.Content.Format == "post"

	// ── Canvas ──
	if p.Canvas.Width <= 0 || p.Canvas.Height <= 0 {
		add("canvas", "width and height are required", true)
	}
	if !isCarousel && p.Canvas.Fps <= 0 {
		add("canvas", "a video needs an fps", true)
	}

	// ── Transition preset ──
	if p.TransitionPreset != "" {
		known := false
		for _, tp := range caps.TransitionPresets {
			if tp.Name == p.TransitionPreset {
				known = true
			}
		}
		if !known {
			add("transitionPreset", fmt.Sprintf("%q is not a preset", p.TransitionPreset), false)
		}
	}

	// ── Asset requests ──
	byID := map[string]AssetRequest{}
	var computed float64
	for i, ar := range p.AssetRequests {
		where := fmt.Sprintf("assetRequests[%d] %s", i, ar.ID)
		if ar.ID == "" {
			add(where, "an asset request needs an id so a layer can point at it", true)
			continue
		}
		if _, dup := byID[ar.ID]; dup {
			add(where, "duplicate asset request id", true)
		}
		byID[ar.ID] = ar
		if ar.Intent == "" {
			add(where, "no intent — a request with no stated purpose cannot be reviewed or reused", false)
		}

		switch ar.Kind {
		case "image", "video", "music", "voice":
			mm, ok := m.ModelByID(ar.Model)
			if !ok {
				add(where, fmt.Sprintf("model %q is not enabled for this workspace", ar.Model), true)
				break
			}
			if mm.Kind != ar.Kind {
				add(where, fmt.Sprintf("%s is a %s model, not %s", mm.ID, mm.Kind, ar.Kind), true)
			}
			if ar.Kind == "video" || ar.Kind == "music" {
				if ar.DurationS <= 0 {
					add(where, "a generated clip needs a duration", true)
				} else if ok, why := mm.AcceptsDuration(ar.DurationS); !ok {
					// The single most valuable check in this function: this is
					// the 422 that would otherwise arrive minutes later,
					// after the wallet was charged.
					add(where, fmt.Sprintf("%ds refused — %s", ar.DurationS, why), true)
				}
			}
			if ar.Kind == "video" && ar.Aspect != "" && len(mm.AspectRatios) > 0 {
				if !contains(mm.AspectRatios, ar.Aspect) {
					add(where, fmt.Sprintf("%s does not take %s (takes %v)", mm.ID, ar.Aspect, mm.AspectRatios), true)
				}
			}
			if ar.StartFrameFromAssetID != "" && !mm.StartFrame {
				add(where, fmt.Sprintf("%s takes no start frame, so it cannot be chained", mm.ID), true)
			}
			if len(ar.RefMediaIDs) > 0 && !mm.ImageRefs {
				add(where, fmt.Sprintf("%s takes no reference images", mm.ID), false)
			}
		case "stock":
			// Stock is a search, not a generation: no model, no cost.
		default:
			add(where, fmt.Sprintf("unknown asset kind %q", ar.Kind), true)
		}

		for _, rid := range ar.ReuseCandidateIDs {
			item, ok := m.MediaByID(rid)
			if !ok {
				add(where, fmt.Sprintf("reuse candidate %q is not in the library", rid), false)
				continue
			}
			if item.NoAIInput && len(ar.RefMediaIDs) > 0 {
				add(where, fmt.Sprintf("%s may not be fed to a model (licence)", rid), true)
			}
		}
		for _, rid := range ar.RefMediaIDs {
			// A reference is either a library row, or an EARLIER request in
			// this plan whose output the run resolves before this one submits.
			// The second case is how an anchor still that does not exist yet
			// gets referenced by the clips it holds together.
			if item, ok := m.MediaByID(rid); ok {
				if item.NoAIInput {
					add(where, fmt.Sprintf("%s may not be fed to a model (licence)", rid), true)
				}
				continue
			}
			if prior, ok := byID[rid]; ok && prior.ID != ar.ID {
				if prior.Kind != "image" && prior.Kind != "video" {
					add(where, fmt.Sprintf("reference %q is a %s request, which produces nothing to look at", rid, prior.Kind), true)
				}
				continue
			}
			add(where, fmt.Sprintf("reference %q is neither in the library nor an earlier request", rid), true)
		}
		computed += ar.EstCostUSD
	}

	// Chaining must point backwards, or the run graph has a cycle and nothing
	// can start.
	seen := map[string]bool{}
	for _, ar := range p.AssetRequests {
		if ar.StartFrameFromAssetID != "" {
			if ar.StartFrameFromAssetID == ar.ID {
				add("assetRequests "+ar.ID, "chained to itself", true)
			} else if !seen[ar.StartFrameFromAssetID] {
				add("assetRequests "+ar.ID,
					fmt.Sprintf("chains off %q, which is not an earlier request — a chained beat can only follow one already made", ar.StartFrameFromAssetID), true)
			}
		}
		seen[ar.ID] = true
	}

	// ── Scenes ──
	if len(p.Scenes) == 0 {
		add("scenes", "a plan with no scenes is not a plan", true)
	}
	used := map[string]bool{}
	totalMs := 0
	for i, s := range p.Scenes {
		where := fmt.Sprintf("scenes[%d] %s", i, s.ID)
		if s.ID == "" {
			add(where, "a scene needs an id", true)
		}
		if s.Beat == "" {
			add(where, "no beat — the placeholder on the timeline has nothing to say while it builds", false)
		}

		src, ok := sourceByKind(caps, s.Approach)
		if !ok {
			add(where, fmt.Sprintf("unknown approach %q", s.Approach), true)
		} else if !src.Available {
			add(where, fmt.Sprintf("%q is not available yet", s.Approach), true)
		}
		// Cost discipline as a rule, not a hope — but only where money is
		// actually spent. A video scene that REUSES footage the brand already
		// owns is the cheapest thing in the product, and demanding it justify
		// itself produced exactly what you would expect: "no cheaper approach
		// needed; using owned footage". A validator that asks for a
		// justification where none is owed teaches people to write filler.
		if s.Approach == "video" && s.Why == "" && generatesVideo(s, byID) {
			add(where, "a generated video scene with no justification — say which cheaper approach it beat", false)
		}

		if isCarousel {
			if s.DurationMs != 0 {
				add(where, "a carousel slide has no duration", false)
			}
			if s.TransitionIn.Kind != "" && s.TransitionIn.Kind != "cut" {
				add(where, "a carousel has no transitions", false)
			}
		} else {
			if s.DurationMs <= 0 {
				add(where, "a video scene needs a duration", true)
			}
			totalMs += s.DurationMs
			if i == 0 && s.TransitionIn.Kind != "" && s.TransitionIn.Kind != "cut" {
				add(where, "the first scene always enters on a cut", false)
			}
			if s.TransitionIn.Kind != "" {
				t, ok := transitionByKind(caps, s.TransitionIn.Kind)
				if !ok {
					add(where, fmt.Sprintf("unknown transition %q", s.TransitionIn.Kind), true)
				} else if !t.Available {
					add(where, fmt.Sprintf("transition %q is not available yet", s.TransitionIn.Kind), false)
				}
			}
			// The half-the-shorter-neighbour clamp. Without it the playhead
			// stops being monotonic and "which scene is at t" is a coin flip.
			if i > 0 && s.TransitionIn.Ms > 0 {
				shorter := s.DurationMs
				if p.Scenes[i-1].DurationMs < shorter {
					shorter = p.Scenes[i-1].DurationMs
				}
				if s.TransitionIn.Ms*2 > shorter {
					add(where, fmt.Sprintf("a %dms transition is more than half of the shorter neighbour (%dms)", s.TransitionIn.Ms, shorter), true)
				}
				totalMs -= s.TransitionIn.Ms
			}
		}

		if len(s.Layers) == 0 && s.Approach != "empty" {
			add(where, "no layers", true)
		}
		if len(s.Layers) > caps.MaxLayersPerScene {
			add(where, fmt.Sprintf("%d layers, limit is %d", len(s.Layers), caps.MaxLayersPerScene), true)
		}
		lastHTML := -1
		for j, l := range s.Layers {
			lw := fmt.Sprintf("%s layer[%d]", where, j)
			if _, ok := slotByName(caps, l.Slot); !ok {
				add(lw, fmt.Sprintf("unknown slot %q", l.Slot), true)
			}
			set := 0
			if l.ReuseMediaID != "" {
				set++
				if _, ok := m.MediaByID(l.ReuseMediaID); !ok {
					add(lw, fmt.Sprintf("reuses %q, which is not in the library", l.ReuseMediaID), true)
				}
			}
			if l.AssetRequestID != "" {
				set++
				if _, ok := byID[l.AssetRequestID]; !ok {
					add(lw, fmt.Sprintf("points at asset request %q, which does not exist", l.AssetRequestID), true)
				}
				used[l.AssetRequestID] = true
			}
			if l.HTMLBrief != "" {
				set++
				lastHTML = j
			}
			switch {
			case set == 0:
				add(lw, "a layer must reuse media, request media, or carry an HTML brief", true)
			case set > 1:
				add(lw, "a layer sets more than one source", true)
			}
			if l.Kind == "html" && l.HTMLBrief == "" {
				add(lw, "an html layer needs a brief", true)
			}
			if l.Kind == "media" && l.HTMLBrief != "" {
				add(lw, "a media layer cannot carry an HTML brief", true)
			}
		}
		// The writing goes on top of the PICTURE. Only a full-frame media
		// layer above an HTML layer is suspicious: a logo in a corner or a
		// badge in the lower third sitting above the type is a deliberate,
		// ordinary choice, and warning about it every time — which the first
		// run of this validator did — teaches people to ignore warnings.
		if lastHTML >= 0 {
			for _, l := range s.Layers[lastHTML+1:] {
				if l.Kind == "media" && l.Slot == "full" {
					add(where, "a full-frame media layer sits above the type, which will cover it", false)
					break
				}
			}
		}
	}

	// An anchor still is pointed at by a cast member rather than by a layer,
	// so mark it used before the orphan check or every anchor reads as waste.
	for _, c := range p.Cast {
		if c.AnchorAssetID != "" {
			used[c.AnchorAssetID] = true
		}
	}
	for _, l := range p.Locations {
		if l.AnchorAssetID != "" {
			used[l.AnchorAssetID] = true
		}
	}
	for id := range byID {
		ar := byID[id]
		// Music belongs to the document and voice to a scene (checked below),
		// so neither is pointed at by a layer. Everything else must be.
		if !used[id] && ar.Kind != "music" && ar.Kind != "voice" {
			add("assetRequests "+id, "nothing uses this request", false)
		}
	}

	// ── Consistency ──
	//
	// Everything in this block exists because a face that changes between
	// clips is the fastest way to lose a viewer, and because the first run of
	// this planner reached for chaining to prevent it — which is the one
	// mechanism that cannot do the job.
	out = append(out, p.validateConsistency(m, byID)...)

	// ── Audio ──
	switch p.Audio.MusicApproach {
	case "", "none":
	case "generate":
		if _, ok := m.ModelByID(p.Audio.MusicModel); !ok {
			add("audio", fmt.Sprintf("music model %q is not enabled", p.Audio.MusicModel), true)
		}
	case "reuse":
		if _, ok := m.MediaByID(p.Audio.MusicMediaID); !ok {
			add("audio", fmt.Sprintf("music %q is not in the library", p.Audio.MusicMediaID), true)
		}
	default:
		add("audio", fmt.Sprintf("unknown music approach %q", p.Audio.MusicApproach), true)
	}
	if p.Audio.VoiceModel != "" {
		if mm, ok := m.ModelByID(p.Audio.VoiceModel); !ok {
			add("audio", fmt.Sprintf("voice model %q is not enabled", p.Audio.VoiceModel), true)
		} else if mm.Kind != "voice" {
			add("audio", fmt.Sprintf("%s is not a voice model", mm.ID), true)
		}
	}
	// Narration is per scene, and so are the requests that produce it.
	voiceFor := map[string][]string{}
	sceneIDs := map[string]bool{}
	for _, s := range p.Scenes {
		sceneIDs[s.ID] = true
	}
	for _, ar := range p.AssetRequests {
		if ar.Kind != "voice" {
			continue
		}
		if ar.SceneID == "" {
			add("assetRequests "+ar.ID, "a voice request must name the scene it narrates — one line per scene, or a reorder desynchronises the whole video", true)
			continue
		}
		if !sceneIDs[ar.SceneID] {
			add("assetRequests "+ar.ID, fmt.Sprintf("narrates scene %q, which does not exist", ar.SceneID), true)
			continue
		}
		voiceFor[ar.SceneID] = append(voiceFor[ar.SceneID], ar.ID)
	}
	for _, s := range p.Scenes {
		if s.Narration != "" && p.Audio.VoiceModel == "" {
			add("audio", "scenes carry narration but no voice model is chosen", true)
			break
		}
	}
	for _, s := range p.Scenes {
		switch n := len(voiceFor[s.ID]); {
		case s.Narration != "" && n == 0:
			add("scenes "+s.ID, "narrates but has no voice request of its own", true)
		case s.Narration == "" && n > 0:
			add("scenes "+s.ID, fmt.Sprintf("has a voice request (%s) but nothing to say", voiceFor[s.ID][0]), true)
		case n > 1:
			add("scenes "+s.ID, fmt.Sprintf("%d voice requests for one scene", n), true)
		}
		if s.FitToNarration && s.Narration == "" {
			add("scenes "+s.ID, "fitToNarration with nothing to narrate", false)
		}
		// A voice that overruns its picture is, by the plan's own reckoning,
		// the commonest way a render comes out wrong. Nothing checked it.
		if s.Narration != "" && !s.FitToNarration && s.DurationMs > 0 {
			need := NarrationMs(s.Narration)
			if need > s.DurationMs*115/100 {
				add("scenes "+s.ID, fmt.Sprintf(
					"the line needs about %.1fs to say and the scene is %.1fs. Lengthen the scene, cut the line, or set fitToNarration",
					float64(need)/1000, float64(s.DurationMs)/1000), true)
			}
		}
	}

	// ── Length and money ──
	if !isCarousel && m.Content.TargetSeconds > 0 {
		want := m.Content.TargetSeconds * 1000
		// 25% either way. A planner that lands within a quarter of the asked
		// length has understood the brief; one that does not has not.
		effective := p.EffectiveTotalMs()
		if effective > want*125/100 || effective < want*75/100 {
			note := ""
			if effective != totalMs {
				note = fmt.Sprintf(" — planned %.1fs, but scenes set to fit their narration will stretch it", float64(totalMs)/1000)
			}
			add("scenes", fmt.Sprintf("total %.1fs against a %ds target, transitions subtracted%s", float64(effective)/1000, m.Content.TargetSeconds, note), false)
		}
	}
	// The cap and the wallet are judged on OUR arithmetic, never the plan's.
	// Checking them against the model's self-reported figures meant a plan
	// that under-priced a clip could pass a cap it actually blew.
	_, derived := p.DerivedCosts(m)
	// The plan's own total is informational: a long way off means the planner
	// does not understand what it is spending, which is worth seeing even
	// though nothing depends on it.
	if p.EstCostUSD > 0 && derived > 0 {
		if r := p.EstCostUSD / derived; r > 1.25 || r < 0.75 {
			add("estCostUsd", fmt.Sprintf("the plan prices itself at $%.2f; the registry prices it at $%.2f", p.EstCostUSD, derived), false)
		}
	}
	if m.Budget.RunCapUSD > 0 && derived > m.Budget.RunCapUSD {
		add("estCostUsd", fmt.Sprintf("$%.2f is over the $%.2f run cap", derived, m.Budget.RunCapUSD), true)
	}
	if m.Budget.WalletUSD > 0 && derived > m.Budget.WalletUSD {
		// A run the wallet cannot finish must never start: a half-generated
		// video and an empty wallet is the worst outcome available.
		add("estCostUsd", fmt.Sprintf("$%.2f is more than the $%.2f wallet", derived, m.Budget.WalletUSD), true)
	}
	if m.Budget.Mode == "economy" {
		for _, ar := range p.AssetRequests {
			if ar.Kind == "video" {
				add("assetRequests "+ar.ID, "economy mode does not generate video", true)
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Fatal && !out[j].Fatal })
	return out
}

// Fatal reports whether any problem stops the run.
func Fatal(ps []Problem) bool {
	for _, p := range ps {
		if p.Fatal {
			return true
		}
	}
	return false
}

// TotalMs is the real length: durations minus every overlap. Nothing should
// add the durations up.
func (p Plan) TotalMs() int {
	total := 0
	for i, s := range p.Scenes {
		total += s.DurationMs
		if i > 0 {
			total -= s.TransitionIn.Ms
		}
	}
	return total
}

// CostUSD is the sum of what the run will actually submit.
func (p Plan) CostUSD() float64 {
	var c float64
	for _, ar := range p.AssetRequests {
		c += ar.EstCostUSD
	}
	return c
}

// generatesVideo reports whether any of a scene's layers pull from a video
// generation, as opposed to footage the brand already owns.
func generatesVideo(s ScenePlan, byID map[string]AssetRequest) bool {
	for _, l := range s.Layers {
		if l.AssetRequestID == "" {
			continue
		}
		if byID[l.AssetRequestID].Kind == "video" {
			return true
		}
	}
	return false
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func sourceByKind(c StudioCapabilities, kind string) (SceneSource, bool) {
	for _, s := range c.Sources {
		if s.Kind == kind {
			return s, true
		}
	}
	return SceneSource{}, false
}

func transitionByKind(c StudioCapabilities, kind string) (Transition, bool) {
	for _, t := range c.Transitions {
		if t.Kind == kind {
			return t, true
		}
	}
	return Transition{}, false
}

func slotByName(c StudioCapabilities, name string) (LayerSlot, bool) {
	for _, s := range c.Slots {
		if s.Name == name {
			return s, true
		}
	}
	return LayerSlot{}, false
}

// validateConsistency enforces the rules that keep one person, one voice and
// one place across several generated clips.
//
// It is a separate pass because it is the only part of validation that reasons
// about relationships BETWEEN requests rather than about one request at a
// time: who appears in what, which clip continues which, and which two clips
// are secretly the same shot.
func (p Plan) validateConsistency(m Manifest, byID map[string]AssetRequest) []Problem {
	var out []Problem
	add := func(where, msg string, fatal bool) {
		out = append(out, Problem{Where: where, Message: msg, Fatal: fatal})
	}

	cast := map[string]CastMember{}
	for _, c := range p.Cast {
		if c.ID == "" {
			add("cast", "a cast member needs an id", true)
			continue
		}
		if _, dup := cast[c.ID]; dup {
			add("cast "+c.ID, "duplicate cast id", true)
		}
		cast[c.ID] = c
	}
	locs := map[string]Location{}
	for _, l := range p.Locations {
		if l.ID == "" {
			add("locations", "a location needs an id", true)
			continue
		}
		locs[l.ID] = l
	}

	// Which scene (by index) does each asset request appear in, and does that
	// scene narrate?
	sceneOf := map[string][]int{}
	for i, sc := range p.Scenes {
		for _, l := range sc.Layers {
			if l.AssetRequestID != "" {
				sceneOf[l.AssetRequestID] = append(sceneOf[l.AssetRequestID], i)
			}
		}
	}

	// Count the GENERATED clips each cast member appears in. A reused library
	// clip is not a consistency risk — it is the same file every time.
	clipsPerCast := map[string][]string{}
	for _, ar := range p.AssetRequests {
		if ar.Kind != "video" && ar.Kind != "image" {
			continue
		}
		for _, cid := range ar.CastIDs {
			if _, ok := cast[cid]; !ok {
				add("assetRequests "+ar.ID, fmt.Sprintf("features cast %q, which is not declared", cid), true)
				continue
			}
			// The anchor itself does not count as an appearance to hold
			// consistent — it IS the thing consistency is measured against.
			if cast[cid].AnchorAssetID == ar.ID {
				continue
			}
			if ar.Kind == "video" {
				clipsPerCast[cid] = append(clipsPerCast[cid], ar.ID)
			}
		}
		if ar.LocationID != "" {
			if _, ok := locs[ar.LocationID]; !ok {
				add("assetRequests "+ar.ID, fmt.Sprintf("set in location %q, which is not declared", ar.LocationID), true)
			}
		}
	}

	// Rule: more than one generated clip of a person needs an anchor, and a
	// chain is explicitly not one.
	for cid, clips := range clipsPerCast {
		c := cast[cid]
		if len(clips) < 2 {
			continue
		}
		anchored := false
		switch {
		case c.AnchorMediaID != "":
			if _, ok := m.MediaByID(c.AnchorMediaID); !ok {
				add("cast "+cid, fmt.Sprintf("anchor %q is not in the library", c.AnchorMediaID), true)
			} else {
				anchored = true
			}
		case c.AnchorAssetID != "":
			a, ok := byID[c.AnchorAssetID]
			switch {
			case !ok:
				add("cast "+cid, fmt.Sprintf("anchor request %q does not exist", c.AnchorAssetID), true)
			case a.Kind != "image":
				add("cast "+cid, "an anchor must be an image — a still is cheap to make and easy for a person to approve, which a clip is not", true)
			default:
				anchored = true
			}
		}
		if !anchored {
			add("cast "+cid, fmt.Sprintf("appears in %d generated clips with no anchor still. Chaining is not an anchor: it makes each clip continue the one before, which is wrong for separate beats and compounds drift", len(clips)), true)
		}
		// The anchor must actually be used, or it is decoration.
		if anchored {
			for _, id := range clips {
				ar := byID[id]
				if !hasRef(ar, c) {
					add("assetRequests "+id, fmt.Sprintf("features %s but does not reference the anchor still — pass it in refMediaIds or as the start frame", cid), true)
				}
			}
		}
		if len(clips) > 3 {
			add("cast "+cid, fmt.Sprintf("appears in %d generated clips. Consider ONE longer generation trimmed into several scenes instead: it is exactly consistent and costs one generation", len(clips)), false)
		}
		// The look has to travel verbatim, or the light changes between beats.
		if c.Look != "" {
			for _, id := range clips {
				if ar := byID[id]; ar.Prompt != "" && !strings.Contains(ar.Prompt, c.Look) {
					add("assetRequests "+id, fmt.Sprintf("prompt does not carry %s's look verbatim (%q)", cid, shortenLook(c.Look)), true)
				}
			}
		}
	}

	// Rule: a place in more than one generated clip needs its own anchor, for
	// the same reason a person does. The first run with anchors reused the
	// founder's still as the desk's plate, which happened to be the same
	// frame; nothing checked that it was.
	clipsPerLoc := map[string][]string{}
	for _, ar := range p.AssetRequests {
		if ar.Kind != "video" || ar.LocationID == "" {
			continue
		}
		if l, ok := locs[ar.LocationID]; ok && l.AnchorAssetID == ar.ID {
			continue
		}
		clipsPerLoc[ar.LocationID] = append(clipsPerLoc[ar.LocationID], ar.ID)
	}
	for lid, clips := range clipsPerLoc {
		if len(clips) < 2 {
			continue
		}
		l := locs[lid]
		if l.AnchorMediaID == "" && l.AnchorAssetID == "" {
			add("locations "+lid, fmt.Sprintf("appears in %d generated clips with no anchor. A place drifts between clips exactly as a face does", len(clips)), true)
		}
	}

	// Rule: a chain is only licensed where the scenes are declared continuous.
	for _, ar := range p.AssetRequests {
		if ar.StartFrameFromAssetID == "" {
			continue
		}
		idxs := sceneOf[ar.ID]
		if len(idxs) == 0 {
			continue
		}
		i := idxs[0]
		if i == 0 || !p.Scenes[i].ContinuesPrevious {
			add("assetRequests "+ar.ID,
				"chains off another clip, but its scene does not declare continuesPrevious. A chain continues the previous shot in the same instant; to hold a subject across SEPARATE beats, anchor on a still instead", true)
		}
	}

	// Rule: no second voice from the video model under our narration.
	for _, ar := range p.AssetRequests {
		if !ar.NativeAudio {
			continue
		}
		for _, i := range sceneOf[ar.ID] {
			if p.Scenes[i].Narration != "" {
				add("assetRequests "+ar.ID,
					fmt.Sprintf("native audio is on in scene %s, which narrates — the model's own voice under ours is two voices at once, and a different one in every clip", p.Scenes[i].ID), true)
				break
			}
		}
	}

	// Rule: every narrated line says where it came from.
	//
	// ⚠️ The brand's first rule is that a claim has to be one we can stand
	// behind. A planner asked for a hooky opener will write a confident
	// sentence nobody approved, and the only way to catch that without
	// forbidding good writing is to make the planner DECLARE its source and
	// then check the declaration.
	//
	// `illustrative` is allowed and is the honest label for an invented
	// scenario — it costs the scene an on-screen marker saying so. What is not
	// allowed is an undeclared line, or a whole plan of invented ones.
	illustrative := 0
	narrated := 0
	for _, sc := range p.Scenes {
		if strings.TrimSpace(sc.Narration) == "" {
			continue
		}
		narrated++
		switch sc.NarrationSource {
		case "script", "caption", "paraphrase":
		case "brand":
			if sc.NarrationBrandFactID == "" {
				add("scenes "+sc.ID,
					"attributes its line to the brand without naming which documented fact", true)
			}
		case "illustrative":
			illustrative++
		case "":
			add("scenes "+sc.ID,
				"has a narrated line with no narrationSource. Say where it came from: the script, the caption, a documented brand fact, a paraphrase of one of those, or an invented illustration", true)
		default:
			add("scenes "+sc.ID,
				fmt.Sprintf("narrationSource %q is not one of script, caption, brand, paraphrase, illustrative", sc.NarrationSource), true)
		}

		// Invented specifics are the ones that do damage. A number, a price or
		// a named third party has to trace to real material.
		if sc.NarrationSource == "illustrative" || sc.NarrationSource == "paraphrase" {
			if n := inventedSpecific(sc.Narration); n != "" {
				add("scenes "+sc.ID,
					fmt.Sprintf("the line contains %s, which has to come from the script, the caption or a documented brand fact — not from a paraphrase or an illustration", n), true)
			}
		}
	}
	// Refused only once there are SEVERAL lines and all of them are invented.
	// With a single narrated line, "all of them" is trivially true — and one
	// illustrative line in a short reel is the legitimate case this label
	// exists for, so it earns a glance rather than a refusal.
	switch {
	case narrated > 1 && illustrative == narrated:
		add("scenes", "every narrated line is an invented illustration. That is a video about nobody's product", true)
	case illustrative > 1:
		add("scenes", fmt.Sprintf("%d lines are invented illustrations. One scenario reads as an example; several read as a story we made up", illustrative), false)
	}

	// Rule: one voice for the document; a second speaker has to be declared.
	//
	// ⚠️ NOT FATAL, deliberately. It used to be, and it was the one refusal a
	// user could do nothing about: voice ids live in a provider account, so
	// "fix it and plan again" asked for something nobody holds, and a plan
	// that was otherwise fine died on a blank field. EnsureVoice now fills it
	// — from the studio's own voice, the catalogue, or the configured
	// fallback — and runs before this. So reaching here means every one of
	// those was empty too, which is an environment problem rather than a bad
	// plan, and refusing the plan would not fix it.
	if p.Audio.VoiceModel != "" && p.Audio.VoiceID == "" {
		add("audio", "no voice could be chosen, so each narrated line may be read in a different voice", false)
	}
	for _, sc := range p.Scenes {
		if sc.VoiceID != "" && sc.VoiceID != p.Audio.VoiceID && sc.VoiceWhy == "" {
			add("scenes "+sc.ID, "overrides the document voice with no reason given", true)
		}
	}

	// Rule: two generations of the same subject, place and framing are one
	// generation and two trims.
	type shot struct{ cast, loc, framing string }
	seen := map[shot]string{}
	for _, ar := range p.AssetRequests {
		if ar.Kind != "video" || ar.Framing == "" || len(ar.CastIDs) == 0 {
			continue
		}
		k := shot{cast: strings.Join(ar.CastIDs, "+"), loc: ar.LocationID, framing: ar.Framing}
		if prev, ok := seen[k]; ok {
			add("assetRequests "+ar.ID,
				fmt.Sprintf("same cast, place and framing as %s. That is one generation placed twice with different trims — exactly consistent, and half the cost", prev), false)
			continue
		}
		seen[k] = ar.ID
	}

	// A trim window has to fit the clip it takes from, or the scene plays
	// black at its tail.
	for i, sc := range p.Scenes {
		for j, l := range sc.Layers {
			if l.AssetRequestID == "" || l.TrimOutMs == 0 {
				continue
			}
			ar, ok := byID[l.AssetRequestID]
			if !ok || ar.DurationS == 0 {
				continue
			}
			if l.TrimOutMs > ar.DurationS*1000 {
				add(fmt.Sprintf("scenes[%d] %s layer[%d]", i, sc.ID, j),
					fmt.Sprintf("trims to %dms of a %ds clip", l.TrimOutMs, ar.DurationS), true)
			}
			if l.TrimInMs >= l.TrimOutMs {
				add(fmt.Sprintf("scenes[%d] %s layer[%d]", i, sc.ID, j), "trim in is not before trim out", true)
			}
		}
	}

	return out
}

// hasRef reports whether a request actually uses a cast member's anchor,
// either as a reference image or as its start frame.
func hasRef(ar AssetRequest, c CastMember) bool {
	for _, r := range ar.RefMediaIDs {
		if r == c.AnchorMediaID && r != "" {
			return true
		}
	}
	if c.AnchorAssetID != "" && ar.StartFrameFromAssetID == c.AnchorAssetID {
		return true
	}
	// A request may also name the anchor request among its references by id;
	// the run resolves those to the media the anchor produced.
	for _, r := range ar.RefMediaIDs {
		if c.AnchorAssetID != "" && r == c.AnchorAssetID {
			return true
		}
	}
	return false
}

func shortenLook(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:39] + "\u2026"
}

// ── What things actually cost, and how long speech actually takes ───────────
//
// Both of these were the planner's job and should never have been. Across
// every run so far the model got its own arithmetic wrong — "plan says $4.98,
// its requests add to $4.89" — and worse, the run cap was being checked
// against the model's self-reported figures, so a plan that under-reported a
// clip at a cent would have sailed past a cap it actually blew. We know the
// per-second and per-call prices; we should be the ones multiplying.

// SpeechCharsPerSecond is how fast narration is spoken, for estimating how
// long a line takes before it has been generated.
//
// About 150 words a minute, at roughly five characters and a space per word.
// It is an estimate and it is only used to catch a line that cannot possibly
// fit its scene; the real length is measured once the audio exists, which is
// what fitToNarration then uses.
const SpeechCharsPerSecond = 14

// NarrationMs is how long a scene's line is likely to take to say.
func NarrationMs(line string) int {
	n := len(strings.TrimSpace(line))
	if n == 0 {
		return 0
	}
	return n * 1000 / SpeechCharsPerSecond
}

// NarrationTailMs is the breath left after a line before the scene cuts.
const NarrationTailMs = 300

// EffectiveDurationMs is how long a scene will really be on screen.
//
// A scene set to follow its narration is only as long as the planned guess
// until the voice is measured; after that it is the line plus a breath. Using
// the guess for the total length of a video whose every scene fits its
// narration is how a 30-second brief quietly becomes a 44-second render.
func (p Plan) EffectiveDurationMs(i int) int {
	if i < 0 || i >= len(p.Scenes) {
		return 0
	}
	sc := p.Scenes[i]
	if !sc.FitToNarration || sc.Narration == "" {
		return sc.DurationMs
	}
	want := NarrationMs(sc.Narration) + NarrationTailMs
	if want > sc.DurationMs {
		return want
	}
	return sc.DurationMs
}

// EffectiveTotalMs is the real length: effective durations minus the overlaps.
func (p Plan) EffectiveTotalMs() int {
	total := 0
	for i := range p.Scenes {
		total += p.EffectiveDurationMs(i)
		if i > 0 {
			total -= p.Scenes[i].TransitionIn.Ms
		}
	}
	return total
}

// DerivedCosts prices every request from the model registry rather than from
// the plan's own numbers, and returns the per-request map and the total.
//
// A voice request is priced on the characters of the line it speaks, which is
// why it needs the scenes: ElevenLabs bills speech per character, and a plan
// that writes 0.00 for narration is not wrong so much as unanswerable.
func (p Plan) DerivedCosts(m Manifest) (map[string]float64, float64) {
	chars := map[string]int{}
	for _, sc := range p.Scenes {
		chars[sc.ID] = len(strings.TrimSpace(sc.Narration))
	}

	per := map[string]float64{}
	var total float64
	for _, ar := range p.AssetRequests {
		mm, ok := m.ModelByID(ar.Model)
		if !ok {
			// An unknown model is already a fatal problem elsewhere; pricing
			// it at zero here would quietly make the plan look affordable.
			continue
		}
		var c float64
		switch ar.Kind {
		case "video", "music":
			c = mm.EstCost(ar.DurationS, 0)
		case "image":
			c = mm.EstCost(0, 0)
		case "voice":
			c = mm.EstCost(0, chars[ar.SceneID])
		}
		per[ar.ID] = c
		total += c
	}
	return per, total
}

// inventedSpecific reports the kind of specific in a line that must not be
// invented, or "".
//
// Deliberately narrow. It is looking for the three things whose invention does
// real damage — a figure, a price, and a named third party — and not trying to
// judge prose. A false positive here costs the planner one repair round; a
// false negative ships a made-up statistic.
func inventedSpecific(line string) string {
	if currencyRe.MatchString(line) {
		return "a price"
	}
	if percentRe.MatchString(line) {
		return "a percentage"
	}
	for _, m := range figureRe.FindAllString(line, -1) {
		// Plain small numbers are ordinary speech ("three things", "day one"),
		// so only a figure big enough to read as a statistic counts.
		//
		// ⚠️ But a bare four-digit number is usually a YEAR, and a year is not
		// a claim. "Back in 2024 we had no idea" was being refused as an
		// invented figure — fatally, so one ordinary line about when something
		// happened killed the whole plan.
		if looksLikeYear(m) {
			continue
		}
		return "a figure"
	}
	return ""
}

// looksLikeYear reports whether a matched figure is really a date.
//
// Deliberately narrow: a BARE four-digit number between 1900 and 2099. A
// thousands separator ("1,200") or five digits ("10000") is never a year, so
// those still count.
//
// The trade-off, stated plainly: a year-shaped statistic slips through —
// "1999 customers" reads as a year to this check. That is the right way round.
// Missing one rare phrasing costs a warning nobody sees; refusing every line
// that mentions a year makes the whole provenance rule unusable, and a rule
// people have to work around teaches them to distrust all of it.
func looksLikeYear(m string) bool {
	if len(m) != 4 {
		return false
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return false
	}
	return n >= 1900 && n <= 2099
}

var (
	currencyRe = regexp.MustCompile(`[$£€₹]\s?\d`)
	percentRe  = regexp.MustCompile(`\d+(\.\d+)?\s?%`)
	// Four digits or more, or a number with a thousands separator: 1,200 and
	// 10000 read as claims where "three" does not.
	figureRe = regexp.MustCompile(`\b(\d{1,3}(,\d{3})+|\d{4,})\b`)
)
