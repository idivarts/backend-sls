package director

import "fmt"

// ── The capability manifest ─────────────────────────────────────────────────
//
// Everything the planner is given, in one value. It is assembled server-side
// rather than described in a prompt, because every field here is a fact that
// changes without us: an enabled model, a wallet balance, a media library.
//
// The six groups are deliberate. Take any one away and the planner starts
// guessing: without model capabilities it plans a 15-second Veo clip against
// an 8-second ceiling; without the media library it regenerates what the brand
// already owns; without the budget it plans something that cannot be paid for.

// ContentFacts is the post being made.
type ContentFacts struct {
	ContentID string   `json:"contentId"`
	Title     string   `json:"title"`
	Format    string   `json:"format"` // reel | post | carousel | story | video
	Platforms []string `json:"platforms"`
	Aspect    string   `json:"aspect"`
	// TargetSeconds is what the user or the format asks for. 0 means the
	// planner decides, which it should then say out loud in its summary.
	TargetSeconds int    `json:"targetSeconds"`
	Hook          string `json:"hook,omitempty"`
	Script        string `json:"script,omitempty"`
	Caption       string `json:"caption,omitempty"`
	Pillar        string `json:"pillar,omitempty"`
}

// BrandFacts is what makes the output look like this brand rather than a
// template. The design system is the single most valuable field here: it is
// what makes a motion scene worth choosing over a generated video.
type BrandFacts struct {
	BrandID     string   `json:"brandId"`
	Name        string   `json:"name"`
	Voice       string   `json:"voice,omitempty"`
	Palette     []string `json:"palette,omitempty"`
	Fonts       []string `json:"fonts,omitempty"`
	LogoMediaID string   `json:"logoMediaId,omitempty"`
	Rules       []string `json:"rules,omitempty"`
	// WhatWorked is a short list of past winners, phrased as approaches rather
	// than post ids — the planner needs the lesson, not the row.
	WhatWorked []string `json:"whatWorked,omitempty"`
}

// MediaModel mirrors trendlymodels.MediaModel, narrowed to what a planner
// needs. Only ENABLED rows are ever put in a manifest: `enabled` is a review
// gate, so an empty list is the normal state of a workspace where nothing has
// been turned on, and the planner must treat it as "not available to you"
// rather than as a fault.
type MediaModel struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Kind         string `json:"kind"` // image | video | music | voice | stock
	StartFrame   bool   `json:"startFrame,omitempty"`
	EndFrame     bool   `json:"endFrame,omitempty"`
	ImageRefs    bool   `json:"imageRefs,omitempty"`
	NativeAudio  bool   `json:"nativeAudio,omitempty"`
	MinDurationS int    `json:"minDurationS,omitempty"`
	MaxDurationS int    `json:"maxDurationS,omitempty"`
	// DurationsS is the EXACT set a model accepts when it accepts a set
	// rather than a range. Veo refuses 5 seconds with a 422, and bounds of
	// 4-8 cannot express that.
	DurationsS      []int    `json:"durationsS,omitempty"`
	Resolutions     []string `json:"resolutions,omitempty"`
	AspectRatios    []string `json:"aspectRatios,omitempty"`
	EstUSDPerSecond float64  `json:"estUsdPerSecond,omitempty"`
	EstUSDPerCall   float64  `json:"estUsdPerCall,omitempty"`
}

// MediaItem is one row of the brand's library, as a planner sees it: enough to
// decide whether it can be reused, and nothing else.
type MediaItem struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"` // image | video | audio
	Describes  string `json:"describes"`
	DurationMs int    `json:"durationMs,omitempty"`
	Aspect     string `json:"aspect,omitempty"`
	// Origin says where it came from, which is what stops the library being a
	// dumping ground a month later.
	Origin string `json:"origin,omitempty"`
	// NoAIInput marks an asset whose licence forbids feeding it to a model.
	// A stock photo is exactly the row that cannot be a reference image.
	NoAIInput bool `json:"noAiInput,omitempty"`
}

// Budget is what the run may spend. Mode is the user's intent, which is what
// they actually choose — they do not pick models.
type Budget struct {
	WalletUSD float64 `json:"walletUsd"`
	// RunCapUSD is the ceiling for this one run, which is usually well below
	// the wallet. Nobody wants one reel to cost a month's allowance.
	RunCapUSD float64 `json:"runCapUsd"`
	Plan      string  `json:"plan"`
	Mode      string  `json:"mode"` // economy | balanced | cinematic
}

// Manifest is the whole of what the planner sees.
type Manifest struct {
	Content      ContentFacts       `json:"content"`
	Brand        BrandFacts         `json:"brand"`
	Models       []MediaModel       `json:"enabledModels"`
	Library      []MediaItem        `json:"mediaLibrary"`
	Budget       Budget             `json:"budget"`
	Capabilities StudioCapabilities `json:"studioCapabilities"`
	Tools        map[string][]Tool  `json:"tools"`

	// Voices the narration can speak in, with enough label to choose between
	// them. Handed over for the same reason the model list is: a plan naming
	// something that does not exist is invalid, and a planner asked to produce
	// an id it has never seen will either invent one or leave it blank.
	//
	// ⚠️ IT USED TO BE NEITHER. The planner was told a voiceId was required
	// and never shown one, so it left the field empty and the plan was refused
	// as FATAL — a dead end a user could do nothing about, since the id lives
	// in a provider's account and not anywhere they can read.
	Voices []VoiceOption `json:"voiceCatalogue,omitempty"`

	// PreferredVoiceID is the voice this studio already speaks in, if any.
	//
	// Ranked above the catalogue when filling a gap: a studio that has been
	// narrated before should go on sounding like itself, and silently
	// switching voices between runs is worse than any catalogue pick.
	PreferredVoiceID string `json:"preferredVoiceId,omitempty"`
}

// VoiceOption is one voice the narration could use.
type VoiceOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Labels is the provider's own description, flattened — "american · calm ·
	// narration". Enough for a planner to match a voice to the content's tone,
	// which is the judgement we actually want the model making.
	Labels string `json:"labels,omitempty"`
}

// ModelByID finds an enabled model. A plan naming anything else is invalid,
// which is the whole point of handing the list over.
func (m Manifest) ModelByID(id string) (MediaModel, bool) {
	for _, mm := range m.Models {
		if mm.ID == id {
			return mm, true
		}
	}
	return MediaModel{}, false
}

// MediaByID finds a library row the plan proposes to reuse.
func (m Manifest) MediaByID(id string) (MediaItem, bool) {
	for _, it := range m.Library {
		if it.ID == id {
			return it, true
		}
	}
	return MediaItem{}, false
}

// AcceptsDuration reports whether a model will take this many seconds, and
// says what it would take instead. The exact-set case is why this is a method
// and not a comparison against Min/Max at each call site.
func (mm MediaModel) AcceptsDuration(s int) (bool, string) {
	if len(mm.DurationsS) > 0 {
		for _, d := range mm.DurationsS {
			if d == s {
				return true, ""
			}
		}
		return false, fmt.Sprintf("%s takes exactly %v seconds", mm.ID, mm.DurationsS)
	}
	if mm.MinDurationS > 0 && s < mm.MinDurationS {
		return false, fmt.Sprintf("%s takes at least %ds", mm.ID, mm.MinDurationS)
	}
	if mm.MaxDurationS > 0 && s > mm.MaxDurationS {
		return false, fmt.Sprintf("%s takes at most %ds", mm.ID, mm.MaxDurationS)
	}
	return true, ""
}

// EstCost is what one generation of this length costs, as far as we know.
// Per-model prices are PROVISIONAL for every row — the providers do not
// publish them — so anything user-facing built on this has to say so.
func (mm MediaModel) EstCost(seconds int, chars int) float64 {
	switch {
	case mm.EstUSDPerSecond > 0 && seconds > 0:
		return mm.EstUSDPerSecond * float64(seconds)
	case mm.Kind == "voice" && mm.EstUSDPerCall > 0 && chars > 0:
		// Speech is billed per character, so EstUSDPerCall is really per char
		// for this kind. Named badly in the registry; corrected here rather
		// than at every call site.
		return mm.EstUSDPerCall * float64(chars)
	case mm.EstUSDPerCall > 0:
		return mm.EstUSDPerCall
	}
	return 0
}
