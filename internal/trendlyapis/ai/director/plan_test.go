package director

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The validator is the only thing standing between a plausible-looking plan
// and a run that spends money on something unrenderable. These tests lock in
// the failures that actually cost us: a duration the provider refuses, a model
// nobody enabled, a chain that points forward, and a plan over the cap.

func fixture(t *testing.T, mode string, cap_ float64) Manifest {
	t.Helper()
	return FixtureManifest("reel", 30, mode, cap_)
}

// loadInvalid is the plan in testdata, written to break one rule per line.
func loadInvalid(t *testing.T) Plan {
	t.Helper()
	raw, err := os.ReadFile("testdata/invalid-plan.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return p
}

// hasProblemContaining ignores severity — for the rules that are reported but
// deliberately do not block.
func hasProblemContaining(ps []Problem, sub string) bool {
	for _, p := range ps {
		if strings.Contains(p.Message, sub) {
			return true
		}
	}
	return false
}

func hasFatalContaining(ps []Problem, sub string) bool {
	for _, p := range ps {
		if p.Fatal && strings.Contains(p.Message, sub) {
			return true
		}
	}
	return false
}

func TestValidateCatchesEveryFatal(t *testing.T) {
	p := loadInvalid(t)
	// The cap is $1 because costs are now priced from the registry, not from
	// the plan: this fixture CLAIMS $7.53 while its requests really come to
	// about $1.53, so a $5 cap no longer catches it. That is the point.
	problems := p.Validate(fixture(t, "balanced", 1))

	if !Fatal(problems) {
		t.Fatal("the invalid fixture validated clean")
	}
	for _, want := range []string{
		// The expensive one: Veo answers 422 on 5 seconds, minutes after the
		// wallet was charged.
		"takes exactly [4 6 8] seconds",
		"is not enabled for this workspace",
		"may not be fed to a model",
		"a chained beat can only follow one already made",
		"more than half of the shorter neighbour",
		"layers, limit is",
		"unknown slot",
		"is not in the library",
		"does not exist",
		"must reuse media, request media, or carry an HTML brief",
		"is not a voice model",
		"over the $1.00 run cap",
	} {
		if !hasFatalContaining(problems, want) {
			t.Errorf("no fatal problem mentioning %q", want)
		}
	}
}

func TestAcceptsDurationExactSet(t *testing.T) {
	veo, ok := fixture(t, "balanced", 8).ModelByID("veo-3.1-fast-generate-001")
	if !ok {
		t.Fatal("veo missing from the fixture")
	}
	for _, s := range []int{4, 6, 8} {
		if ok, why := veo.AcceptsDuration(s); !ok {
			t.Errorf("%ds should be accepted: %s", s, why)
		}
	}
	// 5 and 7 sit inside min/max but outside the set, which is exactly the
	// case bounds alone cannot express.
	for _, s := range []int{3, 5, 7, 9, 15} {
		if ok, _ := veo.AcceptsDuration(s); ok {
			t.Errorf("%ds should be refused", s)
		}
	}
}

func TestEconomyModeRefusesGeneratedVideo(t *testing.T) {
	p := Plan{
		Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30},
		Scenes: []ScenePlan{{
			ID: "s1", Beat: "hook", Approach: "video", DurationMs: 4000,
			TransitionIn: Transition2{Kind: "cut"},
			Layers:       []LayerPlan{{Kind: "media", Slot: "full", AssetRequestID: "a1"}},
			Why:          "needs real motion",
		}},
		AssetRequests: []AssetRequest{{
			ID: "a1", Kind: "video", Intent: "empty calendar", Model: "veo-3.1-fast-generate-001",
			DurationS: 4, Aspect: "9:16", EstCostUSD: 0.60,
		}},
		EstCostUSD: 0.60,
	}
	if !hasFatalContaining(p.Validate(fixture(t, "economy", 8)), "economy mode does not generate video") {
		t.Error("economy mode allowed a generated video")
	}
	if Fatal(p.Validate(fixture(t, "balanced", 8))) {
		t.Error("the same plan should be fine in balanced mode")
	}
}

// A valid plan must validate clean, or the warnings become noise and people
// stop reading them — which is how a fatal problem gets missed.
// minimalValidPlan is the cheapest plan that validates clean: two composite
// scenes built from media the brand already owns, and nothing generated. It is
// the baseline the rule-specific tests add one thing to.
func minimalValidPlan() Plan {
	return Plan{
		Summary: "Reuse what we own.",
		Canvas:  Canvas{Width: 1080, Height: 1920, Fps: 30, Aspect: "9:16"},
		Scenes: []ScenePlan{
			{
				ID: "s1", Beat: "hook", Approach: "composite", DurationMs: 15000,
				TransitionIn: Transition2{Kind: "cut"},
				Layers: []LayerPlan{
					{Kind: "media", Slot: "full", ReuseMediaID: "md_chat_models"},
					{Kind: "html", Slot: "full", HTMLBrief: "the claim, large"},
				},
			},
			{
				ID: "s2", Beat: "payoff", Approach: "composite", DurationMs: 15000,
				TransitionIn: Transition2{Kind: "dissolve", Ms: 300},
				Layers: []LayerPlan{
					{Kind: "media", Slot: "full", ReuseMediaID: "md_strategy_doc"},
					{Kind: "html", Slot: "lower-third", HTMLBrief: "the result"},
					{Kind: "media", Slot: "pip-br", ReuseMediaID: "md_logo_mark"},
				},
			},
		},
	}
}

func TestMinimalValidPlanIsClean(t *testing.T) {
	p := minimalValidPlan()
	if problems := p.Validate(fixture(t, "balanced", 8)); len(problems) != 0 {
		for _, pr := range problems {
			t.Errorf("unexpected: %s", pr)
		}
	}
}

// The two defects a real run exposed: narration planned as one file for the
// whole video, and a justification demanded for a scene that spends nothing.

func TestNarrationMustBePerScene(t *testing.T) {
	base := func(voice []AssetRequest) Plan {
		return Plan{
			Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30},
			Audio:  AudioPlan{VoiceModel: "eleven_multilingual_v2", VoiceID: "v_rachel"},
			Scenes: []ScenePlan{
				{
					ID: "s1", Beat: "hook", Approach: "composite", DurationMs: 15000,
					TransitionIn: Transition2{Kind: "cut"}, Narration: "line one", NarrationSource: "paraphrase",
					Layers: []LayerPlan{{Kind: "html", Slot: "full", HTMLBrief: "the claim"}},
				},
				{
					ID: "s2", Beat: "payoff", Approach: "composite", DurationMs: 15000,
					TransitionIn: Transition2{Kind: "cut"}, Narration: "line two", NarrationSource: "paraphrase",
					Layers: []LayerPlan{{Kind: "html", Slot: "full", HTMLBrief: "the result"}},
				},
			},
			AssetRequests: voice,
		}
	}

	// One request for the whole video — what both models produced unprompted.
	blob := base([]AssetRequest{{
		ID: "a_vo", Kind: "voice", Intent: "full narration",
		Model: "eleven_multilingual_v2", EstCostUSD: 0.01,
	}})
	if !hasFatalContaining(blob.Validate(fixture(t, "balanced", 8)), "must name the scene it narrates") {
		t.Error("a single narration blob was accepted")
	}

	// A line per scene.
	split := base([]AssetRequest{
		{ID: "a_vo1", Kind: "voice", Intent: "line one", Model: "eleven_multilingual_v2", SceneID: "s1", EstCostUSD: 0.005},
		{ID: "a_vo2", Kind: "voice", Intent: "line two", Model: "eleven_multilingual_v2", SceneID: "s2", EstCostUSD: 0.005},
	})
	if problems := split.Validate(fixture(t, "balanced", 8)); len(problems) != 0 {
		for _, pr := range problems {
			t.Errorf("unexpected: %s", pr)
		}
	}

	// A line for a scene that does not exist, and a scene left unvoiced.
	wrong := base([]AssetRequest{
		{ID: "a_vo1", Kind: "voice", Intent: "line one", Model: "eleven_multilingual_v2", SceneID: "s9", EstCostUSD: 0.005},
	})
	got := wrong.Validate(fixture(t, "balanced", 8))
	if !hasFatalContaining(got, `narrates scene "s9", which does not exist`) {
		t.Error("a voice request for a missing scene was accepted")
	}
	if !hasFatalContaining(got, "narrates but has no voice request of its own") {
		t.Error("an unvoiced narrated scene was accepted")
	}
}

func TestWhyOwedOnlyWhereMoneyIsSpent(t *testing.T) {
	scene := func(layer LayerPlan) Plan {
		return Plan{
			Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30},
			Scenes: []ScenePlan{{
				ID: "s1", Beat: "someone at a laptop at night", Approach: "video", DurationMs: 30000,
				TransitionIn: Transition2{Kind: "cut"}, Layers: []LayerPlan{layer},
			}},
		}
	}

	// Reusing owned footage spends nothing, so no justification is owed.
	reuse := scene(LayerPlan{Kind: "media", Slot: "full", ReuseMediaID: "md_founder_desk"})
	for _, pr := range reuse.Validate(fixture(t, "balanced", 8)) {
		if strings.Contains(pr.Message, "justification") {
			t.Errorf("justification demanded for reused footage: %s", pr)
		}
	}

	// A generation does owe one.
	gen := scene(LayerPlan{Kind: "media", Slot: "full", AssetRequestID: "a1"})
	gen.AssetRequests = []AssetRequest{{
		ID: "a1", Kind: "video", Intent: "founder at a laptop, night", Model: "veo-3.1-fast-generate-001",
		DurationS: 8, Aspect: "9:16", EstCostUSD: 1.20,
	}}
	found := false
	for _, pr := range gen.Validate(fixture(t, "balanced", 8)) {
		if strings.Contains(pr.Message, "justification") {
			found = true
		}
	}
	if !found {
		t.Error("no justification demanded for a generated video scene")
	}
}

// TotalMs must subtract overlaps. A transition consumes time from both
// neighbours, so adding the durations up overstates the video — and length is
// what platform caps and render cost are measured on.
func TestTotalMsSubtractsOverlaps(t *testing.T) {
	p := Plan{Scenes: []ScenePlan{
		{DurationMs: 3000, TransitionIn: Transition2{Kind: "cut"}},
		{DurationMs: 3000, TransitionIn: Transition2{Kind: "dissolve", Ms: 400}},
	}}
	if got := p.TotalMs(); got != 5600 {
		t.Errorf("TotalMs = %d, want 5600", got)
	}
}

// The hole this closed: the cap used to be checked against the plan's own
// arithmetic, so a plan that under-reported what it was spending passed a cap
// it actually blew. Every run so far got its own total wrong by a few cents;
// nothing stopped one being wrong by five dollars.
func TestCapIsJudgedOnOurPricesNotThePlans(t *testing.T) {
	p := Plan{
		Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30},
		Scenes: []ScenePlan{{
			ID: "s1", Beat: "an expensive shot", Approach: "video", DurationMs: 30000,
			TransitionIn: Transition2{Kind: "cut"}, Why: "nothing in the library",
			Layers: []LayerPlan{{Kind: "media", Slot: "full", AssetRequestID: "a1"}},
		}},
		AssetRequests: []AssetRequest{{
			ID: "a1", Kind: "video", Intent: "eight seconds on the dear model",
			Model: "veo-3.1-generate-001", DurationS: 8, Aspect: "9:16",
			// Eight seconds of Veo 3.1 is $3.20. The plan says four cents.
			EstCostUSD: 0.04,
		}},
		EstCostUSD: 0.04,
	}
	got := p.Validate(fixture(t, "cinematic", 2))
	if !hasFatalContaining(got, "$3.20 is over the $2.00 run cap") {
		t.Error("an under-reported plan slipped past the run cap")
	}
	// And the discrepancy itself is surfaced, because a planner that does not
	// know what it is spending is worth seeing.
	found := false
	for _, pr := range got {
		if strings.Contains(pr.Message, "the registry prices it at") {
			found = true
		}
	}
	if !found {
		t.Error("the gap between the plan's price and ours was not reported")
	}
}

// A line that cannot be said in the time its scene is on screen.
func TestNarrationMustFitItsScene(t *testing.T) {
	long := "Scheduling was never the hard part, and that is the thing nobody tells you when you start doing this on your own every single week."
	scene := func(fit bool, ms int) Plan {
		return Plan{
			Canvas: Canvas{Width: 1080, Height: 1920, Fps: 30},
			Audio:  AudioPlan{VoiceModel: "eleven_multilingual_v2", VoiceID: "v_rachel"},
			Scenes: []ScenePlan{{
				ID: "s1", Beat: "the distinction", Approach: "motion", DurationMs: ms,
				TransitionIn: Transition2{Kind: "cut"}, Narration: long, NarrationSource: "paraphrase", FitToNarration: fit,
				Layers: []LayerPlan{{Kind: "html", Slot: "full", HTMLBrief: "two lines of type"}},
			}},
			AssetRequests: []AssetRequest{{
				ID: "v1", Kind: "voice", Intent: "the line", Model: "eleven_multilingual_v2", SceneID: "s1",
			}},
		}
	}
	// ~133 characters is about 9.5s of speech. In a 3s scene it is refused.
	if !hasFatalContaining(scene(false, 3000).Validate(fixture(t, "balanced", 8)), "to say and the scene is 3.0s") {
		t.Error("a line that cannot fit its scene was accepted")
	}
	// The same line is fine when the scene is told to follow it.
	if Fatal(scene(true, 3000).Validate(fixture(t, "balanced", 8))) {
		t.Error("fitToNarration should absorb a long line")
	}
	// And the effective length reflects the stretch, not the planned 3s.
	if got := scene(true, 3000).EffectiveTotalMs(); got < 9000 {
		t.Errorf("EffectiveTotalMs = %dms, want the narration length (~9.8s)", got)
	}
}

// ── Script provenance ──────────────────────────────────────────────────────
//
// The brand's first rule is that a claim has to be one we can stand behind. A
// planner asked for a hooky opener will write a confident sentence nobody
// approved, and the only way to catch that without forbidding good writing is
// to make it declare a source and then check the declaration.

func narratedPlan(line, source, factID string) Plan {
	p := minimalValidPlan()
	// One short scene, so the plan's length matches the fixture's 10s target
	// and the only thing under test is the narration.
	p.Scenes = p.Scenes[:1]
	p.Scenes[0].DurationMs = 5000
	p.Scenes[0].Narration = line
	p.Scenes[0].NarrationSource = source
	p.Scenes[0].NarrationBrandFactID = factID
	p.Scenes[0].FitToNarration = true
	p.Audio.VoiceModel = "eleven_multilingual_v2"
	p.Audio.VoiceID = "v_1"
	p.AssetRequests = append(p.AssetRequests, AssetRequest{
		ID: "a_voice", Kind: "voice", Intent: "the line",
		Model: "eleven_multilingual_v2", SceneID: p.Scenes[0].ID,
	})
	return p
}

func TestANarratedLineNeedsASource(t *testing.T) {
	m := FixtureManifest("reel", 10, "balanced", 5.0)
	p := narratedPlan("Sunday night is when the panic sets in.", "", "")

	if !Fatal(p.Validate(m)) {
		t.Fatal("a narrated line with no source should be fatal")
	}
}

func TestAnUnknownSourceIsRefused(t *testing.T) {
	m := FixtureManifest("reel", 10, "balanced", 5.0)
	p := narratedPlan("Sunday night is when the panic sets in.", "vibes", "")

	if !Fatal(p.Validate(m)) {
		t.Fatal("an unrecognised narrationSource should be fatal")
	}
}

func TestABrandClaimMustNameItsFact(t *testing.T) {
	m := FixtureManifest("reel", 10, "balanced", 5.0)

	if !Fatal(p(narratedPlan("We plan a week in ten minutes.", "brand", "")).Validate(m)) {
		t.Fatal("a brand-attributed line with no fact id should be fatal")
	}
	if Fatal(p(narratedPlan("We plan a week in ten minutes.", "brand", "f_wedge")).Validate(m)) {
		t.Error("naming the fact should satisfy it")
	}
}

// An invented scenario is allowed — it is the honest label — and gets marked on
// screen. What is refused is a plan made entirely of them.
func TestOneIllustrationIsFineAndAWholePlanOfThemIsNot(t *testing.T) {
	m := FixtureManifest("reel", 10, "balanced", 5.0)

	one := narratedPlan("Say you're a founder with four posts to write.", "illustrative", "")
	if Fatal(one.Validate(m)) {
		t.Errorf("a single illustration should be allowed: %v", one.Validate(m))
	}

	// Every narrated line invented.
	all := one
	all.Scenes = append(all.Scenes, ScenePlan{
		ID: "s_two", Beat: "another", Approach: "motion", DurationMs: 4000,
		TransitionIn:    Transition2{Kind: "cut"},
		Narration:       "And say that founder has no idea where to start.",
		NarrationSource: "illustrative",
		FitToNarration:  true,
		Layers:          []LayerPlan{{Kind: "html", Slot: "full", HTMLBrief: "type on brand colour"}},
	})
	all.AssetRequests = append(all.AssetRequests, AssetRequest{
		ID: "a_voice2", Kind: "voice", Intent: "the second line", SceneID: "s_two",
	})
	if !Fatal(all.Validate(m)) {
		t.Error("a plan where every line is invented should be refused")
	}
}

// Invented specifics are the ones that do damage.
func TestAnInventedFigureIsRefused(t *testing.T) {
	m := FixtureManifest("reel", 10, "balanced", 5.0)

	for _, line := range []string{
		"It saves you £40 a month.",
		"Teams see a 62% lift in output.",
		"Over 12,000 founders already plan this way.",
	} {
		p := narratedPlan(line, "paraphrase", "")
		if !Fatal(p.Validate(m)) {
			t.Errorf("a paraphrase containing a specific should be refused: %q", line)
		}
		// The same figure IS allowed when it comes from the user's own script.
		q := narratedPlan(line, "script", "")
		if Fatal(q.Validate(m)) {
			t.Errorf("a figure from the script should be allowed: %q", line)
		}
	}
}

// Ordinary speech is not a claim. "Three things" must not trip the detector, or
// the planner spends repair rounds on nothing.
func TestSmallNumbersInProseArePermitted(t *testing.T) {
	m := FixtureManifest("reel", 10, "balanced", 5.0)
	p := narratedPlan("Three things fix a Sunday night, and this is the first.", "paraphrase", "")

	if Fatal(p.Validate(m)) {
		t.Errorf("plain prose numbers should not be treated as claims: %v", p.Validate(m))
	}
}

// p is a tiny helper so the table-ish tests above read as one line each.
func p(pl Plan) Plan { return pl }
