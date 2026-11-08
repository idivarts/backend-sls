// Package director is the AI director: it plans a whole studio document —
// every scene, its approach, its layers, and the media that has to be brought
// into existence for any of it to work — from a brief.
//
// The package is deliberately split so that the expensive thinking is testable
// without any of the infrastructure:
//
//	capabilities.go  what the studio can express, and what tools exist
//	manifest.go      everything the planner is given, assembled
//	prompt.go        how the manifest is put to a model
//	plan.go          the plan it must return, and what makes one invalid
//	storyboard.go    the plan as a human reads it
//
// Nothing here talks to Firestore, OpenRouter or S3. `cmd/director-lab` wires
// it to a model and prints the result; the HTTP handler that will eventually
// live in internal/trendlyapis/studio wires the same functions to the real
// manifest sources. That is the point: the planner can be judged before a line
// of it is deployed.
package director

// ── Why a catalogue at all ──────────────────────────────────────────────────
//
// A planner that is told what to do in prose will cheerfully plan a 15-second
// Veo clip, a seven-layer scene and a transition nobody implemented. None of
// those fail at planning time; they fail minutes later, after money has been
// spent, as a provider 422 or a render that cannot be assembled.
//
// So the planner is handed a MACHINE-READABLE description of what this product
// can actually express, and the same description is what the UI and the agent
// tools are built from. One registry, three consumers. When a phase lands and
// the studio can do something new, this file changes and the planner gets
// better with no prompt rewrite.

// SceneSource is a kind a scene's primary layer can be.
type SceneSource struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	// CostNote is how the planner should think about reaching for it. Money is
	// the planner's main constraint, so it is stated per source rather than
	// left to be inferred.
	CostNote string `json:"costNote"`
	// Available is false for a source whose phase has not landed. A planner
	// must be able to plan well against a half-built studio, because it will
	// ship before the studio is finished.
	Available bool `json:"available"`
}

// LayerSlot is a named position in a scene. The planner picks a slot, never
// coordinates: free geometry is where a model crops a face at the chin, and
// where preview and render drift apart.
type LayerSlot struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Transition is one way a scene may enter.
type Transition struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	TypicalMs   int    `json:"typicalMs"`
	Available   bool   `json:"available"`
}

// TransitionPreset is the document-level rhythm. Setting transitions per
// boundary is what makes a timeline look accidental; a preset is what makes a
// generated one look deliberate.
type TransitionPreset struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// StudioCapabilities is the whole of what a plan may ask for.
type StudioCapabilities struct {
	Version           string             `json:"version"`
	Sources           []SceneSource      `json:"sources"`
	Slots             []LayerSlot        `json:"slots"`
	MaxLayersPerScene int                `json:"maxLayersPerScene"`
	Transitions       []Transition       `json:"transitions"`
	TransitionPresets []TransitionPreset `json:"transitionPresets"`
	AudioLanes        []string           `json:"audioLanes"`
	// Rules are the constraints that are not expressible as a field. They are
	// phrased as instructions because that is how they reach the model.
	Rules []string `json:"rules"`
}

// DefaultCapabilities is the studio as this plan specifies it. `Available`
// tracks the phase plan, so flipping one flag is how the planner is told that
// a capability has landed.
func DefaultCapabilities() StudioCapabilities {
	return StudioCapabilities{
		Version:           "studio-v2-0.1",
		MaxLayersPerScene: 4,
		Sources: []SceneSource{
			{
				Kind:        "motion",
				Description: "A self-contained HTML/CSS animated scene we author. Brand-accurate, instant to edit, text is editable by clicking it.",
				CostNote:    "Cheapest moving picture available. Costs a render, not a model call. Prefer it for anything type-led: hooks, lists, stats, quotes, calls to action.",
				Available:   true,
			},
			{
				Kind:        "composite",
				Description: "A still HTML canvas on which real media (photos, logos, screenshots) is arranged with type over it.",
				CostNote:    "Costs a render, not a model call. This is how a real photo gets a brand headline over it. Re-arranging is free.",
				Available:   true,
			},
			{
				Kind:        "image",
				Description: "A single still filling the frame, optionally with a slow Ken Burns push so it is not visually dead.",
				CostNote:    "Free when the still already exists in the media library. A generated still costs one image call.",
				Available:   true,
			},
			{
				Kind:        "video",
				Description: "Real moving footage: an upload, a stock clip, or a generated clip.",
				CostNote:    "The most expensive thing in the product when generated. Must be justified against a cheaper approach.",
				Available:   true,
			},
			{
				Kind:        "empty",
				Description: "A deliberate placeholder that holds a position on the timeline.",
				CostNote:    "Free. Use it when a beat is needed but you are not confident what fills it.",
				Available:   true,
			},
		},
		Slots: []LayerSlot{
			{Name: "full", Description: "Fills the frame. The default, and the only slot a single-layer scene uses."},
			{Name: "left", Description: "Left half, for a vertical split screen."},
			{Name: "right", Description: "Right half, for a vertical split screen."},
			{Name: "top", Description: "Top half, for a horizontal split."},
			{Name: "bottom", Description: "Bottom half, for a horizontal split."},
			{Name: "q1", Description: "Top-left quarter of a four-up grid."},
			{Name: "q2", Description: "Top-right quarter of a four-up grid."},
			{Name: "q3", Description: "Bottom-left quarter of a four-up grid."},
			{Name: "q4", Description: "Bottom-right quarter of a four-up grid."},
			{Name: "pip-br", Description: "Small inset, bottom right. A reaction or a face over a wider shot."},
			{Name: "lower-third", Description: "A band across the lower third. Where a name, a stat or a caption lives."},
		},
		Transitions: []Transition{
			{Kind: "cut", Description: "No transition. Correct far more often than people expect.", TypicalMs: 0, Available: true},
			{Kind: "dissolve", Description: "Cross-fade both ways. The universal smoother.", TypicalMs: 300, Available: true},
			{Kind: "dip", Description: "Fade to black or white and back. The only one that reliably survives a hard subject change.", TypicalMs: 400, Available: true},
			{Kind: "slide", Description: "The incoming scene pushes the outgoing one off. Carousel-feeling; good for lists and before/after.", TypicalMs: 350, Available: true},
			{Kind: "wipe", Description: "A hard edge sweeps across. Reads as graphic rather than cinematic.", TypicalMs: 300, Available: true},
			{Kind: "blur-dissolve", Description: "Dissolve with a blur ramp through the middle. The expensive-looking one; at most once per video.", TypicalMs: 450, Available: false},
			{Kind: "whip", Description: "Fast directional blur. Energetic, and the first thing to look cheap if overused.", TypicalMs: 160, Available: false},
		},
		TransitionPresets: []TransitionPreset{
			{Name: "snappy", Description: "All cuts, with a dip at a hard topic change. Default for reels and shorts."},
			{Name: "soft", Description: "Dissolves throughout, longer on the first and last boundary. Default for long video."},
			{Name: "graphic", Description: "Slides in a consistent direction, matching the motion scenes' own movement."},
			{Name: "energetic", Description: "Cuts with an occasional whip. Needs a music bed to land."},
		},
		AudioLanes: []string{"music", "voice", "sfx"},
		Rules: []string{
			"A transition consumes time from BOTH neighbouring scenes. The video's total length is the sum of scene durations MINUS every overlap, so do not add the durations up.",
			"A transition's length may never exceed half the shorter of the two scenes it joins.",
			"A motion scene that animates its own entrance should enter on a cut. Dissolving into something already flying in is the commonest way this kind of video looks amateur.",
			"Audio is anchored to a scene and an offset inside it, never to an absolute time on the whole video. Only the music bed is anchored to the document.",
			"Only the first media layer in a scene carries sound unless there is a reason to say otherwise.",
			"An HTML layer sits on top of media layers; it is the writing.",
			"A scene references media by id. It never holds a file. Two scenes that need the same clip share one asset request.",
			"Prefer, in order: media the brand already owns, then motion or composite, then a generated image, then a generated video. Every generated video must carry a justification naming the cheaper option it beat.",
			"A carousel has no clock: no durations, no transitions, no audio.",
			"CONSISTENCY. A person or place appearing in more than one generated clip must be declared in cast or locations, and anchored on ONE approved still that every clip of them references. Work down this list and use the first that fits: (1) do not show a face at all — over the shoulder, hands, the back of a head; (2) ONE generation referenced by several scenes at different trims, which is exactly consistent because it is the same footage and costs one generation; (3) a chain, ONLY where the scenes are declared continuous in the same unbroken moment; (4) an anchor still referenced by every clip, for genuinely different framings.",
			"A chain is NOT a way to hold a face across separate beats. It makes each clip continue the previous one in the same instant and framing, so a later beat looks like the same moment, drift compounds down the chain, and the clips can no longer run in parallel.",
			"Every generated clip featuring a cast member must repeat that cast member's look string verbatim in its prompt, so the lens, light and time of day cannot change between beats.",
			"Native audio must stay off for any clip in a scene that narrates: the model's own voice under ours is two voices at once, and a different one in every clip.",
			"Pick one voiceId for the whole document. A second speaker needs a stated reason.",
		},
	}
}

// ── The tool inventory ──────────────────────────────────────────────────────
//
// The planner is told what the system can DO, not only what it can describe,
// and it is told honestly: a tool marked Missing cannot be planned around yet.
// Without that distinction a planner invents a capability and the run fails on
// the first node.

// ToolStatus says whether a tool can be called today.
type ToolStatus string

const (
	// ToolExists is implemented and registered in the AI loop today.
	ToolExists ToolStatus = "exists"
	// ToolDriverOnly means the capability exists underneath (a mediagen
	// driver, an endpoint) but no AI tool exposes it yet. Cheap to add.
	ToolDriverOnly ToolStatus = "driver-only"
	// ToolMissing has to be built. Named here anyway, so the planner knows
	// what the finished system will be able to do and we know what to build.
	ToolMissing ToolStatus = "missing"
)

// Tool is one capability the director can rely on at run time.
type Tool struct {
	Name    string     `json:"name"`
	Purpose string     `json:"purpose"`
	Status  ToolStatus `json:"status"`
	// Phase is when it lands, for the ones that are missing.
	Phase string `json:"phase,omitempty"`
}

// ToolInventory is everything the run can call, grouped the way a planner
// thinks about it rather than the way the code is organised.
//
// The `exists` rows were read off the registry in internal/trendlyapis/ai and
// internal/trendlyapis/ai/tools, not invented: there are 35 read-only fetch
// tools today, and the write side is design, image, audio and timeline.
func ToolInventory() map[string][]Tool {
	return map[string][]Tool{
		"read the brand": {
			{Name: "get_brand_profile", Purpose: "Who the brand is, in its own words.", Status: ToolExists},
			{Name: "get_design_system", Purpose: "Brand kit, voice and rules. What makes a motion scene look like this brand and not a template.", Status: ToolExists},
			{Name: "get_brand_memory", Purpose: "Durable facts the brand has told us before.", Status: ToolExists},
			{Name: "get_active_strategy", Purpose: "The current content strategy and its pillars.", Status: ToolExists},
			{Name: "get_content_details", Purpose: "One post's full brief, script and destinations.", Status: ToolExists},
			{Name: "get_script", Purpose: "The content's script, which is what the narration is cut from.", Status: ToolExists},
		},
		"read what performed": {
			{Name: "get_top_performing_posts", Purpose: "What has worked for this brand. The best prior on an approach.", Status: ToolExists},
			{Name: "get_format_performance", Purpose: "Whether this brand's reels beat its carousels.", Status: ToolExists},
			{Name: "get_post_analytics", Purpose: "One post's numbers.", Status: ToolExists},
			{Name: "get_engagement_trends", Purpose: "Direction of travel.", Status: ToolExists},
		},
		"find media we already own": {
			{Name: "get_media_library", Purpose: "Everything on S3 for this brand. The cheapest scene is one made from this.", Status: ToolExists},
			{Name: "get_footage", Purpose: "Video assets with their ingest: duration, transcript, shot descriptions.", Status: ToolExists},
			{Name: "get_generated_assets", Purpose: "What the AI has made before, so it is reused rather than remade.", Status: ToolExists},
			{Name: "search_media", Purpose: "Semantic search over alt text, titles, transcripts and shot descriptions. 'a clip of someone laughing' has to resolve to an id.", Status: ToolMissing, Phase: "0"},
		},
		"make a still": {
			{Name: "generate_image", Purpose: "Text-to-image. Serves photoreal stills and illustration.", Status: ToolExists},
			{Name: "search_stock_images", Purpose: "Pexels and Unsplash search. Drivers exist in pkg/mediagen (KindStock); no AI tool is wired to them.", Status: ToolDriverOnly, Phase: "0"},
			{Name: "extract_frame", Purpose: "Take frame at t from a video and register it as media. The primitive behind both last-frame chaining and start-frame picking.", Status: ToolMissing, Phase: "0"},
		},
		"make moving picture": {
			{Name: "generate_design", Purpose: "Author an HTML/CSS scene. Today it writes one document holding every scene; in studio v2 it writes ONE scene.", Status: ToolExists},
			{Name: "apply_design_edits", Purpose: "Edit an existing HTML scene deterministically.", Status: ToolExists},
			{Name: "get_design_html", Purpose: "Read a scene's full HTML before editing it.", Status: ToolExists},
			{Name: "generate_video", Purpose: "Text-to-video, optionally from a start and end frame. mediagen and the mediaModels registry support it; NO AI tool exposes it.", Status: ToolDriverOnly, Phase: "4"},
			{Name: "cut_footage", Purpose: "Trim a piece of real footage onto the timeline.", Status: ToolExists},
		},
		"make sound": {
			{Name: "generate_voiceover", Purpose: "Narration from text, with word timings that captions are derived from.", Status: ToolExists},
			{Name: "generate_music", Purpose: "A bed. Can be asked for a length and a build.", Status: ToolExists},
		},
		"build the timeline": {
			{Name: "get_timeline", Purpose: "Read the current timeline.", Status: ToolExists},
			{Name: "add_scene", Purpose: "Insert a scene at an index, empty or with a source.", Status: ToolMissing, Phase: "1"},
			{Name: "set_scene_layers", Purpose: "Set a scene's layer stack: slots, media, volumes.", Status: ToolMissing, Phase: "4"},
			{Name: "reorder_scenes", Purpose: "Reorder by id.", Status: ToolMissing, Phase: "1"},
			{Name: "split_scene", Purpose: "Cut one scene into two at a time.", Status: ToolMissing, Phase: "2"},
			{Name: "delete_scene", Purpose: "Remove a scene, leaving its media alone.", Status: ToolMissing, Phase: "1"},
			{Name: "regenerate_scene", Purpose: "Re-run one scene's own generation with an edited prompt.", Status: ToolMissing, Phase: "3"},
			{Name: "set_transitions", Purpose: "Apply a document preset, or override one boundary.", Status: ToolMissing, Phase: "2"},
			{Name: "set_audio_lanes", Purpose: "Place the bed and the scene-anchored voice lines.", Status: ToolMissing, Phase: "2"},
			{Name: "render_studio", Purpose: "Queue the render. Charges the wallet, then dispatches to the image or video lane.", Status: ToolMissing, Phase: "5"},
		},
		"know what it may spend": {
			{Name: "get_entitlements_and_usage", Purpose: "Plan, wallet balance and what is gated. A plan that cannot be paid for is not a plan.", Status: ToolExists},
		},
	}
}
