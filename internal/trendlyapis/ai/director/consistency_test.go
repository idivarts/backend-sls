package director

import "testing"

// The consistency rules exist because the first real run of this planner
// reached for chaining to hold a founder's face across three separate beats —
// the one mechanism that cannot do that job. Each test below is one way that
// goes wrong, and each one costs real money to discover any other way.

// twoClipPlan is two generated clips of the same person in different beats,
// which is exactly the case that needs an anchor.
func twoClipPlan(cast []CastMember, reqs []AssetRequest, continues bool) Plan {
	return Plan{
		Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30, Aspect: "9:16"},
		Cast:   cast,
		Scenes: []ScenePlan{
			{
				ID: "s1", Beat: "the confession", Approach: "video", DurationMs: 15000,
				TransitionIn: Transition2{Kind: "cut"}, Why: "the library has no shot of this person",
				Layers: []LayerPlan{{Kind: "media", Slot: "full", AssetRequestID: "a1"}},
			},
			{
				ID: "s2", Beat: "the relief", Approach: "video", DurationMs: 15000,
				TransitionIn: Transition2{Kind: "cut"}, Why: "mirrors the opening, same person",
				ContinuesPrevious: continues,
				Layers:            []LayerPlan{{Kind: "media", Slot: "full", AssetRequestID: "a2"}},
			},
		},
		AssetRequests: reqs,
		EstCostUSD:    total(reqs),
	}
}

func total(reqs []AssetRequest) float64 {
	var c float64
	for _, r := range reqs {
		c += r.EstCostUSD
	}
	return c
}

func videoReq(id, startFrom string, cast []string, prompt string) AssetRequest {
	return AssetRequest{
		ID: id, Kind: "video", Intent: "founder at a laptop", Model: "veo-3.1-fast-generate-001",
		Prompt: prompt, DurationS: 4, Aspect: "9:16", CastIDs: cast,
		StartFrameFromAssetID: startFrom, EstCostUSD: 0.60,
	}
}

const look = "35mm, shallow depth, cool desk lamp from the left, 2am"

func TestChainIsNotAnAnchor(t *testing.T) {
	// Two separate beats held together by a chain: what the real run did.
	p := twoClipPlan(
		[]CastMember{{ID: "c_founder", Description: "early thirties, grey sweatshirt", Look: look}},
		[]AssetRequest{
			videoReq("a1", "", []string{"c_founder"}, "seen from behind at a desk. "+look),
			videoReq("a2", "a1", []string{"c_founder"}, "same person, calmer. "+look),
		},
		false,
	)
	got := p.Validate(fixture(t, "cinematic", 12))
	if !hasFatalContaining(got, "Chaining is not an anchor") {
		t.Error("two unanchored clips of one person were accepted")
	}
	if !hasFatalContaining(got, "does not declare continuesPrevious") {
		t.Error("a chain across separate beats was accepted")
	}
}

func TestAnchoredClipsValidate(t *testing.T) {
	anchor := AssetRequest{
		ID: "a_anchor", Kind: "image", Intent: "canonical still of the founder",
		Model: "google/gemini-3.1-flash-image-preview", Prompt: "founder from behind. " + look,
		Aspect: "9:16", CastIDs: []string{"c_founder"}, EstCostUSD: 0.04,
	}
	a1 := videoReq("a1", "", []string{"c_founder"}, "seen from behind at a desk. "+look)
	a1.RefMediaIDs = []string{"a_anchor"}
	a2 := videoReq("a2", "", []string{"c_founder"}, "same person, calmer. "+look)
	a2.RefMediaIDs = []string{"a_anchor"}

	p := twoClipPlan(
		[]CastMember{{ID: "c_founder", Description: "early thirties, grey sweatshirt", Look: look, AnchorAssetID: "a_anchor"}},
		[]AssetRequest{anchor, a1, a2},
		false,
	)
	for _, pr := range p.Validate(fixture(t, "cinematic", 12)) {
		t.Errorf("unexpected: %s", pr)
	}
}

func TestAnchorMustBeReferencedAndMustBeAStill(t *testing.T) {
	// Declared but never passed to the clips: an anchor nobody uses.
	p := twoClipPlan(
		[]CastMember{{ID: "c_founder", Description: "founder", Look: look, AnchorMediaID: "md_founder_desk"}},
		[]AssetRequest{
			videoReq("a1", "", []string{"c_founder"}, "behind a desk. "+look),
			videoReq("a2", "", []string{"c_founder"}, "calmer. "+look),
		},
		false,
	)
	if !hasFatalContaining(p.Validate(fixture(t, "cinematic", 12)), "does not reference the anchor still") {
		t.Error("clips ignoring their anchor were accepted")
	}

	// An anchor that is itself a video: expensive to make and hard for a
	// person to judge, which is the whole reason it has to be a still.
	bad := twoClipPlan(
		[]CastMember{{ID: "c_founder", Description: "founder", Look: look, AnchorAssetID: "a_vid_anchor"}},
		[]AssetRequest{
			videoReq("a_vid_anchor", "", nil, "the founder. "+look),
			videoReq("a1", "", []string{"c_founder"}, "behind a desk. "+look),
			videoReq("a2", "", []string{"c_founder"}, "calmer. "+look),
		},
		false,
	)
	if !hasFatalContaining(bad.Validate(fixture(t, "cinematic", 12)), "an anchor must be an image") {
		t.Error("a video anchor was accepted")
	}
}

func TestLookMustTravelVerbatim(t *testing.T) {
	a1 := videoReq("a1", "", []string{"c_founder"}, "behind a desk. "+look)
	a1.RefMediaIDs = []string{"md_founder_desk"}
	// Paraphrased look: the light changes between beats and nothing catches it.
	a2 := videoReq("a2", "", []string{"c_founder"}, "calmer, warm morning light, 50mm")
	a2.RefMediaIDs = []string{"md_founder_desk"}

	p := twoClipPlan(
		[]CastMember{{ID: "c_founder", Description: "founder", Look: look, AnchorMediaID: "md_founder_desk"}},
		[]AssetRequest{a1, a2},
		false,
	)
	if !hasFatalContaining(p.Validate(fixture(t, "cinematic", 12)), "does not carry c_founder's look verbatim") {
		t.Error("a paraphrased look was accepted")
	}
}

// Veo can emit its own audio. Under our narration that is two voices at once,
// and a different second voice in every clip.
func TestNativeAudioRefusedInANarratedScene(t *testing.T) {
	a1 := videoReq("a1", "", nil, "a desk at night")
	a1.NativeAudio = true
	p := Plan{
		Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30},
		Audio:  AudioPlan{VoiceModel: "eleven_multilingual_v2", VoiceID: "v_rachel"},
		Scenes: []ScenePlan{{
			ID: "s1", Beat: "hook", Approach: "video", DurationMs: 30000,
			TransitionIn: Transition2{Kind: "cut"}, Narration: "Sunday nights used to be the worst.", NarrationSource: "paraphrase",
			Why: "no library clip of this", Layers: []LayerPlan{{Kind: "media", Slot: "full", AssetRequestID: "a1"}},
		}},
		AssetRequests: []AssetRequest{
			a1,
			{ID: "v1", Kind: "voice", Intent: "hook line", Model: "eleven_multilingual_v2", SceneID: "s1", EstCostUSD: 0.005},
		},
	}
	if !hasFatalContaining(p.Validate(fixture(t, "cinematic", 12)), "native audio is on in scene s1") {
		t.Error("a second voice from the video model was accepted")
	}
}

func TestVoiceIDRequiredAndOverrideNeedsAReason(t *testing.T) {
	base := func(docVoice, sceneVoice, why string) Plan {
		return Plan{
			Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30},
			Audio:  AudioPlan{VoiceModel: "eleven_multilingual_v2", VoiceID: docVoice},
			Scenes: []ScenePlan{{
				ID: "s1", Beat: "hook", Approach: "motion", DurationMs: 30000,
				TransitionIn: Transition2{Kind: "cut"}, Narration: "a line", NarrationSource: "paraphrase",
				VoiceID: sceneVoice, VoiceWhy: why,
				Layers: []LayerPlan{{Kind: "html", Slot: "full", HTMLBrief: "the line, large"}},
			}},
			AssetRequests: []AssetRequest{{
				ID: "v1", Kind: "voice", Intent: "the line", Model: "eleven_multilingual_v2",
				SceneID: "s1", EstCostUSD: 0.005,
			}},
		}
	}
	// A missing voice used to be FATAL. It is not any more, and must not be:
	// voice ids live in a provider account, so refusing the plan asked the
	// user to supply something nobody holds and killed an otherwise fine plan
	// on a blank field. EnsureVoice fills it; this is only the last-resort
	// note for when even the fallback is empty.
	if Fatal(base("", "", "").Validate(fixture(t, "balanced", 8))) {
		t.Error("a missing voice must never block a plan")
	}
	if !hasProblemContaining(base("", "", "").Validate(fixture(t, "balanced", 8)), "different voice") {
		t.Error("a missing voice should still be reported, just not as fatal")
	}
	if !hasFatalContaining(base("v_rachel", "v_adam", "").Validate(fixture(t, "balanced", 8)), "overrides the document voice with no reason") {
		t.Error("an unexplained second voice was accepted")
	}
	if Fatal(base("v_rachel", "v_adam", "a second speaker answers here").Validate(fixture(t, "balanced", 8))) {
		t.Error("a declared second speaker should be allowed")
	}
}

// One generation placed twice is the mechanism we most want chosen, so the
// validator points at it rather than only forbidding the alternatives.
func TestDuplicateShotSuggestsOneGenerationAndTrims(t *testing.T) {
	a1 := videoReq("a1", "", []string{"c_founder"}, "behind a desk. "+look)
	a1.RefMediaIDs = []string{"md_founder_desk"}
	a1.Framing = "over-shoulder mid"
	a1.LocationID = "l_desk"
	a2 := videoReq("a2", "", []string{"c_founder"}, "behind a desk, calmer. "+look)
	a2.RefMediaIDs = []string{"md_founder_desk"}
	a2.Framing = "over-shoulder mid"
	a2.LocationID = "l_desk"

	p := twoClipPlan(
		[]CastMember{{ID: "c_founder", Description: "founder", Look: look, AnchorMediaID: "md_founder_desk"}},
		[]AssetRequest{a1, a2},
		false,
	)
	p.Locations = []Location{{ID: "l_desk", Description: "home desk at night", Look: look}}

	found := false
	for _, pr := range p.Validate(fixture(t, "cinematic", 12)) {
		if !pr.Fatal && contains([]string{pr.Message}, pr.Message) &&
			len(pr.Message) > 0 && pr.Where == "assetRequests a2" {
			found = true
		}
	}
	if !found {
		t.Error("two identical shots did not suggest one generation with trims")
	}
}

// A trim window that runs past the end of its clip plays black at the tail.
func TestTrimMustFitTheClip(t *testing.T) {
	a1 := videoReq("a1", "", nil, "a desk at night")
	p := Plan{
		Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30},
		Scenes: []ScenePlan{{
			ID: "s1", Beat: "hook", Approach: "video", DurationMs: 30000,
			TransitionIn: Transition2{Kind: "cut"}, Why: "nothing in the library",
			Layers: []LayerPlan{{Kind: "media", Slot: "full", AssetRequestID: "a1", TrimInMs: 0, TrimOutMs: 9000}},
		}},
		AssetRequests: []AssetRequest{a1},
	}
	if !hasFatalContaining(p.Validate(fixture(t, "cinematic", 12)), "trims to 9000ms of a 4s clip") {
		t.Error("a trim past the end of the clip was accepted")
	}
}
