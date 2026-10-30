package trendlymodels

import "testing"

// These tests deliberately reuse the VECTORS from
// shared-libs/design/__tests__/composition-runtime.test.ts. The arithmetic
// exists in both languages — Go owns the stored numbers because they are what a
// render is billed on, TS owns playback and layout — so the only thing keeping
// them in step is that a change to one fails the other's tests.

func designSource(revisionID string) CompositionClipSource {
	return CompositionClipSource{Kind: ClipSourceDesign, RevisionID: revisionID}
}

func comp(clips ...CompositionClip) *Composition {
	return &Composition{
		Width:  1080,
		Height: 1920,
		FPS:    30,
		Tracks: []CompositionTrack{{Kind: CompositionTrackVideo, Clips: clips}},
	}
}

func fade(ms int) *CompositionTransition {
	return &CompositionTransition{Kind: CompositionTransitionFade, Ms: ms}
}

func TestMeasurePlacesASingleClipAtZero(t *testing.T) {
	measured, total := MeasureComposition(comp(
		CompositionClip{ID: "a", DurationMs: 4200, Source: designSource("rev1")},
	))
	if len(measured) != 1 || measured[0].StartMs != 0 {
		t.Fatalf("got %+v", measured)
	}
	if total != 4200 {
		t.Errorf("total = %d, want 4200", total)
	}
}

func TestMeasureSubtractsTheOverlap(t *testing.T) {
	measured, total := MeasureComposition(comp(
		CompositionClip{ID: "a", DurationMs: 3000, Source: designSource("rev1")},
		CompositionClip{ID: "b", DurationMs: 3000, TransitionIn: fade(500), Source: designSource("rev2")},
	))
	if measured[1].StartMs != 2500 {
		t.Errorf("second clip starts at %d, want 2500", measured[1].StartMs)
	}
	// The total must be SHORTER than the sum of the durations. Anything that
	// adds the durations up has misunderstood the timeline.
	if total != 5500 {
		t.Errorf("total = %d, want 5500", total)
	}
}

func TestMeasureIgnoresAFadeOnTheFirstClip(t *testing.T) {
	measured, total := MeasureComposition(comp(
		CompositionClip{ID: "a", DurationMs: 2000, TransitionIn: fade(800), Source: designSource("rev1")},
	))
	if measured[0].TransitionMs != 0 {
		t.Errorf("first clip overlap = %d, want 0 — there is nothing behind it", measured[0].TransitionMs)
	}
	if total != 2000 {
		t.Errorf("total = %d, want 2000", total)
	}
}

func TestMeasureFallsBackForAClipWithNoLength(t *testing.T) {
	_, total := MeasureComposition(comp(
		CompositionClip{ID: "a", DurationMs: 0, Source: designSource("rev1")},
	))
	if total != DefaultClipMs {
		t.Errorf("total = %d, want %d", total, DefaultClipMs)
	}
}

func TestMeasureGivesADeclaredFadeTheDefaultLength(t *testing.T) {
	measured, _ := MeasureComposition(comp(
		CompositionClip{ID: "a", DurationMs: 3000, Source: designSource("rev1")},
		CompositionClip{ID: "b", DurationMs: 3000, TransitionIn: fade(0), Source: designSource("rev2")},
	))
	if measured[1].TransitionMs != DefaultTransitionMs {
		t.Errorf("overlap = %d, want %d", measured[1].TransitionMs, DefaultTransitionMs)
	}
}

func TestMeasureClampsAnOverlapToHalfTheShorterClip(t *testing.T) {
	measured, _ := MeasureComposition(comp(
		CompositionClip{ID: "a", DurationMs: 1000, Source: designSource("rev1")},
		CompositionClip{ID: "b", DurationMs: 8000, TransitionIn: fade(5000), Source: designSource("rev2")},
	))
	if measured[1].TransitionMs != 500 {
		t.Errorf("overlap = %d, want 500", measured[1].TransitionMs)
	}
	if measured[1].StartMs != 500 {
		t.Errorf("second clip starts at %d, want 500", measured[1].StartMs)
	}
}

// The clamp's whole purpose. Without it a long transition pushes a clip's start
// back past its predecessor's, the offsets stop increasing, and "which clip is
// at time T" stops having an answer.
func TestOffsetsStayStrictlyIncreasingForAnyTransitionLength(t *testing.T) {
	lengths := []int{120, 500, 1000, 3000, 9000}
	fades := []int{1, 100, 400, 2500, 50000}
	for _, a := range lengths {
		for _, b := range lengths {
			for _, f := range fades {
				measured, total := MeasureComposition(comp(
					CompositionClip{ID: "a", DurationMs: a, Source: designSource("rev1")},
					CompositionClip{ID: "b", DurationMs: b, TransitionIn: fade(f), Source: designSource("rev2")},
				))
				if measured[1].StartMs <= measured[0].StartMs {
					t.Fatalf("a=%d b=%d fade=%d: offsets %d then %d", a, b, f, measured[0].StartMs, measured[1].StartMs)
				}
				if total <= measured[1].StartMs {
					t.Fatalf("a=%d b=%d fade=%d: total %d not past last start %d", a, b, f, total, measured[1].StartMs)
				}
			}
		}
	}
}

func TestMeasureAnEmptyCompositionIsZeroRatherThanAPanic(t *testing.T) {
	measured, total := MeasureComposition(comp())
	if len(measured) != 0 || total != 0 {
		t.Errorf("got %d clips, total %d", len(measured), total)
	}
	if m, total := MeasureComposition(nil); m != nil || total != 0 {
		t.Errorf("nil composition: got %v, %d", m, total)
	}
}

func TestStampDerivedOverwritesWhateverWasStored(t *testing.T) {
	// A client-supplied duration is exactly what must not be trusted: it is
	// what the render is billed on.
	c := comp(
		CompositionClip{ID: "a", DurationMs: 2000, StartMs: 999, Source: designSource("rev1")},
		CompositionClip{ID: "b", DurationMs: 2000, StartMs: 12345, TransitionIn: fade(400), Source: designSource("rev2")},
	)
	c.DurationMs = 1
	c.StampDerived()

	if c.DurationMs != 3600 {
		t.Errorf("durationMs = %d, want 3600", c.DurationMs)
	}
	clips := c.Tracks[0].Clips
	if clips[0].StartMs != 0 || clips[1].StartMs != 1600 {
		t.Errorf("startMs = %d, %d; want 0, 1600", clips[0].StartMs, clips[1].StartMs)
	}
}

func TestSingleDesignPassthrough(t *testing.T) {
	if got := SingleDesignPassthrough(comp(
		CompositionClip{ID: "a", DurationMs: 5000, Source: designSource("revX")},
	)); got != "revX" {
		t.Errorf("got %q, want revX", got)
	}

	// A fade on the only clip normalizes to a cut, so this is still a
	// passthrough — the predicate reads the measured transition, not the
	// authored one, or an existing design with a stray transition would pick up
	// a second lossy generation for a transition that never plays.
	if got := SingleDesignPassthrough(comp(
		CompositionClip{ID: "a", DurationMs: 5000, TransitionIn: fade(400), Source: designSource("revP")},
	)); got != "revP" {
		t.Errorf("pointless fade: got %q, want revP", got)
	}

	scene := 1
	cases := map[string]*Composition{
		"a second clip": comp(
			CompositionClip{ID: "a", DurationMs: 1000, Source: designSource("r1")},
			CompositionClip{ID: "b", DurationMs: 1000, Source: designSource("r2")},
		),
		"a single scene of the design": comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: ClipSourceDesign, RevisionID: "r", SceneIndex: &scene}}),
		"a transparent capture": comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: ClipSourceDesign, RevisionID: "r", Transparent: true}}),
		"a non-design source": comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: ClipSourceImage, URL: "u"}}),
		"no clips": comp(),
		"a clip on an overlay track": {Width: 1080, Height: 1920, FPS: 30, Tracks: []CompositionTrack{
			{Kind: CompositionTrackVideo, Clips: []CompositionClip{{ID: "a", DurationMs: 1000, Source: designSource("r")}}},
			{Kind: CompositionTrackOverlay, Clips: []CompositionClip{{ID: "o", DurationMs: 500, Source: designSource("r")}}},
		}},
		"two video tracks": {Width: 1080, Height: 1920, FPS: 30, Tracks: []CompositionTrack{
			{Kind: CompositionTrackVideo, Clips: []CompositionClip{{ID: "a", DurationMs: 1000, Source: designSource("r1")}}},
			{Kind: CompositionTrackVideo, Clips: []CompositionClip{{ID: "b", DurationMs: 1000, Source: designSource("r2")}}},
		}},
	}
	for label, c := range cases {
		if got := SingleDesignPassthrough(c); got != "" {
			t.Errorf("%s: should not be a passthrough, got %q", label, got)
		}
	}
}

func TestCompositionFromDesignIsAPassthrough(t *testing.T) {
	c := CompositionFromDesign("rev9", 1080, 1920, 30, 7400)
	if got := SingleDesignPassthrough(c); got != "rev9" {
		t.Fatalf("got %q, want rev9 — an existing design must render exactly as before", got)
	}
	if c.DurationMs != 7400 || c.Width != 1080 || c.Height != 1920 || c.FPS != 30 {
		t.Errorf("got %+v", c)
	}
	if ok, why := CompositionRenderable(c); !ok {
		t.Errorf("not renderable: %s", why)
	}

	zero := CompositionFromDesign("r", 1080, 1920, 30, 0)
	if zero.DurationMs != DefaultClipMs {
		t.Errorf("unmeasured design: durationMs = %d, want %d", zero.DurationMs, DefaultClipMs)
	}
}

func TestValidateComposition(t *testing.T) {
	fatalPaths := func(c *Composition) map[string]bool {
		out := map[string]bool{}
		for _, p := range ValidateComposition(c) {
			if p.Fatal {
				out[p.Path] = true
			}
		}
		return out
	}

	if got := ValidateComposition(comp(
		CompositionClip{ID: "a", DurationMs: 1000, Source: designSource("r")},
	)); len(got) != 0 {
		t.Errorf("a plain single-design composition should pass, got %+v", got)
	}

	speed := 0.0
	in, out := 2000, 1000
	cases := []struct {
		label string
		c     *Composition
		want  string
	}{
		{"footage with no url", comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: ClipSourceFootage}}), "tracks.video.clips[0].source.url"},
		{"image with no url", comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: ClipSourceImage}}), "tracks.video.clips[0].source.url"},
		{"a generation still in flight", comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: ClipSourceGenVideo, Model: "m", Prompt: "p", JobID: "j"}}),
			"tracks.video.clips[0].source.url"},
		{"an inverted trim", comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: ClipSourceFootage, URL: "u", InMs: &in, OutMs: &out}}),
			"tracks.video.clips[0].source.outMs"},
		{"a zero speed", comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: ClipSourceFootage, URL: "u", Speed: &speed}}),
			"tracks.video.clips[0].source.speed"},
		{"a clip with no duration", comp(CompositionClip{ID: "a", DurationMs: 0,
			Source: designSource("r")}), "tracks.video.clips[0].durationMs"},
		{"no source", comp(CompositionClip{ID: "a", DurationMs: 1000}), "tracks.video.clips[0].source"},
		{"an unknown source", comp(CompositionClip{ID: "a", DurationMs: 1000,
			Source: CompositionClipSource{Kind: "hologram"}}), "tracks.video.clips[0].source.kind"},
		{"no clips", comp(), "tracks.video.clips"},
		{"no video track", &Composition{Width: 1080, Height: 1920, FPS: 30}, "tracks"},
	}
	for _, tc := range cases {
		if !fatalPaths(tc.c)[tc.want] {
			t.Errorf("%s: expected a fatal problem at %s, got %+v", tc.label, tc.want, ValidateComposition(tc.c))
		}
	}

	dupes := fatalPaths(comp(
		CompositionClip{ID: "same", DurationMs: 1000, Source: designSource("r1")},
		CompositionClip{ID: "same", DurationMs: 1000, Source: designSource("r2")},
	))
	if !dupes["tracks.video.clips[1].id"] {
		t.Error("duplicate clip ids must be rejected — a UI keys on them")
	}

	// Placed audio clips are still ahead of us; overlays are not.
	notYet := fatalPaths(&Composition{Width: 1080, Height: 1920, FPS: 30, Tracks: []CompositionTrack{
		{Kind: CompositionTrackVideo, Clips: []CompositionClip{{ID: "a", DurationMs: 1000, Source: designSource("r")}}},
		{Kind: CompositionTrackAudio, Clips: []CompositionClip{{ID: "s", DurationMs: 100, Source: designSource("r")}}},
	}})
	if !notYet["tracks.audio"] {
		t.Errorf("placed audio must still be refused plainly, got %+v", notYet)
	}

	// An overlay IS a transparent design — anything else is a different feature.
	overlay := func(clip CompositionClip) map[string]bool {
		return fatalPaths(&Composition{Width: 1080, Height: 1920, FPS: 30, Tracks: []CompositionTrack{
			{Kind: CompositionTrackVideo, Clips: []CompositionClip{{ID: "a", DurationMs: 4000, Source: designSource("r")}}},
			{Kind: CompositionTrackOverlay, Clips: []CompositionClip{clip}},
		}})
	}
	good := overlay(CompositionClip{ID: "cap", StartMs: 500, DurationMs: 2000,
		Source: CompositionClipSource{Kind: ClipSourceDesign, RevisionID: "cap", Transparent: true}})
	if len(good) != 0 {
		t.Errorf("a transparent design overlay should pass, got %+v", good)
	}
	if p := overlay(CompositionClip{ID: "c", DurationMs: 1000,
		Source: CompositionClipSource{Kind: ClipSourceImage, URL: "u"}}); !p["tracks.overlay.clips[0].source"] {
		t.Errorf("a non-design overlay must be refused, got %+v", p)
	}
	if p := overlay(CompositionClip{ID: "c", DurationMs: 1000,
		Source: designSource("r")}); !p["tracks.overlay.clips[0].source.transparent"] {
		t.Errorf("an opaque overlay must be refused, got %+v", p)
	}

	// And a transparent design on the VIDEO track points at the right track.
	if p := fatalPaths(comp(CompositionClip{ID: "a", DurationMs: 1000,
		Source: CompositionClipSource{Kind: ClipSourceDesign, RevisionID: "r", Transparent: true}})); !p["tracks.video.clips[0].source.transparent"] {
		t.Errorf("a transparent design on the video track must be refused, got %+v", p)
	}

	bad := comp(CompositionClip{ID: "a", DurationMs: 1000, Source: designSource("r")})
	bad.Width = 0
	if !fatalPaths(bad)["size"] {
		t.Error("a zero width must be fatal")
	}
	bad.Width, bad.FPS = 1080, 240
	if !fatalPaths(bad)["fps"] {
		t.Error("240fps must be fatal")
	}

	// A fade on the first clip is pointless, not broken.
	warn := ValidateComposition(comp(
		CompositionClip{ID: "a", DurationMs: 1000, TransitionIn: fade(400), Source: designSource("r")},
	))
	if len(warn) != 1 || warn[0].Fatal {
		t.Errorf("want one non-fatal warning, got %+v", warn)
	}
}

func TestCompositionPricingMatchesTheDesignItReplaces(t *testing.T) {
	// The reason this matters: every pre-existing video design now goes through
	// the composition path. If a single-clip composition priced differently, a
	// user's reel would change price for no visible reason.
	for _, ms := range []int{500, 3000, 15000, 30000, 60000} {
		design := RenderTokenCost("video", 1, ms)
		composition := CompositionRenderTokenCost(ms, 1, 0)
		if design != composition {
			t.Errorf("%dms: design costs %d, one-clip composition costs %d", ms, design, composition)
		}
	}
}

func TestCompositionPricingChargesForClipsAndDecode(t *testing.T) {
	base := CompositionRenderTokenCost(15000, 1, 0)
	clips := CompositionRenderTokenCost(15000, 4, 0)
	if clips <= base {
		t.Errorf("four clips (%d) should cost more than one (%d)", clips, base)
	}
	withDecode := CompositionRenderTokenCost(15000, 4, 60000)
	if withDecode <= clips {
		t.Errorf("decoding a minute of footage (%d) should cost more than not (%d)", withDecode, clips)
	}
}

func TestCompositionDecodedMsCountsSourceTimeNotTimelineTime(t *testing.T) {
	in, out := 10000, 13000
	double, half := 2.0, 0.5

	cases := []struct {
		label string
		clip  CompositionClip
		want  int
	}{
		{"a design clip decodes nothing",
			CompositionClip{ID: "a", DurationMs: 5000, Source: designSource("r")}, 0},
		{"a still decodes nothing",
			CompositionClip{ID: "a", DurationMs: 5000, Source: CompositionClipSource{Kind: ClipSourceImage, URL: "u"}}, 0},
		{"untrimmed footage decodes its slot",
			CompositionClip{ID: "a", DurationMs: 4000, Source: CompositionClipSource{Kind: ClipSourceFootage, URL: "u"}}, 4000},
		{"a trim is already source time",
			CompositionClip{ID: "a", DurationMs: 3000, Source: CompositionClipSource{
				Kind: ClipSourceFootage, URL: "u", InMs: &in, OutMs: &out}}, 3000},
		{"double speed pulls twice the footage through",
			CompositionClip{ID: "a", DurationMs: 3000, Source: CompositionClipSource{
				Kind: ClipSourceFootage, URL: "u", Speed: &double}}, 6000},
		{"half speed needs half of it",
			CompositionClip{ID: "a", DurationMs: 3000, Source: CompositionClipSource{
				Kind: ClipSourceFootage, URL: "u", Speed: &half}}, 1500},
		{"a generated clip decodes like footage",
			CompositionClip{ID: "a", DurationMs: 8000, Source: CompositionClipSource{
				Kind: ClipSourceGenVideo, Model: "m", Prompt: "p", URL: "u"}}, 8000},
	}
	for _, tc := range cases {
		if got := CompositionDecodedMs(comp(tc.clip)); got != tc.want {
			t.Errorf("%s: got %dms, want %dms", tc.label, got, tc.want)
		}
	}
}

func TestOverlayClipsKeepTheirAuthoredStart(t *testing.T) {
	transparent := func(id string) CompositionClipSource {
		return CompositionClipSource{Kind: ClipSourceDesign, RevisionID: id, Transparent: true}
	}
	c := &Composition{Width: 1080, Height: 1920, FPS: 30, Tracks: []CompositionTrack{
		{Kind: CompositionTrackVideo, Clips: []CompositionClip{{ID: "v", DurationMs: 10000, Source: designSource("v")}}},
		{Kind: CompositionTrackOverlay, Clips: []CompositionClip{
			{ID: "b", StartMs: 6000, DurationMs: 1500, Source: transparent("b")},
			{ID: "a", StartMs: 1000, DurationMs: 2000, Source: transparent("a")},
		}},
	}}

	// Sorted by start, not document order, so the graph composites them in the
	// order they appear.
	got := OverlayClips(c)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("got %+v", got)
	}

	// StampDerived restamps the VIDEO track only. Restamping overlays would
	// move every caption to the start of the timeline.
	c.StampDerived()
	after := OverlayClips(c)
	if after[0].StartMs != 1000 || after[1].StartMs != 6000 {
		t.Errorf("overlay starts were restamped: %d, %d", after[0].StartMs, after[1].StartMs)
	}

	if OverlayClips(nil) != nil {
		t.Error("a nil composition has no overlays")
	}
	if got := OverlayClips(comp(CompositionClip{ID: "a", DurationMs: 1, Source: designSource("r")})); got != nil {
		t.Errorf("a composition with no overlay track has no overlays, got %+v", got)
	}
}
