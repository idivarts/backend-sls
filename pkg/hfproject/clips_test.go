package hfproject

import "testing"

// A reel exactly like the ones dev is making: two scenes, a photograph,
// music, two voiceovers and two caption tracks. Before this, its timeline
// drew two rows and hid the other six things.
const reel = `<!doctype html>
<html><body>
  <div id="root" data-composition-id="main" data-duration="9">
    <audio id="bgm" data-media="bgm" data-media-intent="warm lo-fi beats"
           data-media-resolved=".media/audio/bgm/bgm_001.wav" src=".media/audio/bgm/bgm_001.wav"
           data-start="0" data-duration="9"></audio>
    <audio id="vo-1" data-media="voice" data-media-intent="Hit the 2 PM wall? Not any more."
           src="assets/voice/a.wav" data-start="0.2" data-duration="4"></audio>
    <section id="scene-hook" data-scene="hook" data-start="0" data-duration="5">
      <h1 id="hook-title" class="clip" data-start="0.2" data-duration="4">Hit the 2 PM wall?</h1>
      <p id="hook-sub" class="clip" data-start="1" data-duration="3">Cold brew, no sugar</p>
      <img id="shot" class="clip" src=".media/images/image_001.jpg" data-start="0" data-duration="5" />
    </section>
    <div id="captions-1" class="caption-track" data-captions="vo-1" data-start="0.2" data-duration="4"><span
      class="clip" data-start="0.2" data-duration="0.3">Hit</span></div>
    <section id="scene-outro" data-scene="outro" data-start="5" data-duration="4">
      <h1 id="outro-title" class="clip" data-start="5" data-duration="4">Fuel your focus</h1>
    </section>
  </div>
  <script>gsap.timeline();</script>
</body></html>`

func byID(cs []Clip, id string) (Clip, bool) {
	for _, c := range cs {
		if c.ElementID == id {
			return c, true
		}
	}
	return Clip{}, false
}

func TestClipsFindsEverythingOnTheTimeline(t *testing.T) {
	cs := Clips(reel)
	want := map[string]string{
		"scene-hook":  ClipScene,
		"scene-outro": ClipScene,
		"shot":        ClipImage,
		"bgm":         ClipMusic,
		"vo-1":        ClipVoice,
		"captions-1":  ClipCaptions,
	}
	for id, kind := range want {
		c, ok := byID(cs, id)
		if !ok {
			t.Fatalf("%s is not on the timeline", id)
		}
		if c.Kind != kind {
			t.Fatalf("%s reads as %q, want %q", id, c.Kind, kind)
		}
	}
	if len(cs) != len(want) {
		t.Fatalf("got %d clips, want %d: %+v", len(cs), len(want), cs)
	}
}

// Headlines are timed and addressable and still not clips: a three-scene
// reel would otherwise have thirty rows. The layer tree shows them.
func TestTextInsideASceneIsNotAClip(t *testing.T) {
	for _, id := range []string{"hook-title", "hook-sub", "outro-title", "root"} {
		if _, ok := byID(Clips(reel), id); ok {
			t.Fatalf("%s is on the timeline", id)
		}
	}
}

// The read-time fallback: a film that never declared a track still draws
// sensibly, because a kind implies a row.
func TestTracksAreInferredWhenTheFilmDoesNotSay(t *testing.T) {
	cs := Clips(reel)
	for id, track := range map[string]int{
		"scene-hook": 0, "scene-outro": 0,
		"shot": 1, "vo-1": 2, "bgm": 3, "captions-1": 4,
	} {
		c, _ := byID(cs, id)
		if c.TrackIndex != track {
			t.Fatalf("%s is on row %d, want %d", id, c.TrackIndex, track)
		}
		if c.Authored {
			t.Fatalf("%s claims the film declared its row", id)
		}
	}
}

func TestAnAuthoredTrackWins(t *testing.T) {
	film := `<div id="root" data-composition-id="main" data-duration="3">
	  <audio id="bgm" data-media="bgm" data-track-index="7" src="a.wav" data-start="0" data-duration="3"></audio>
	  <audio id="old" data-media="sfx" data-layer="5" src="b.wav" data-start="0" data-duration="1"></audio>
	</div>`
	cs := Clips(film)
	c, _ := byID(cs, "bgm")
	if c.TrackIndex != 7 || !c.Authored {
		t.Fatalf("authored track ignored: %+v", c)
	}
	// data-layer is the older spelling of the same thing.
	o, _ := byID(cs, "old")
	if o.TrackIndex != 5 || !o.Authored {
		t.Fatalf("legacy data-layer ignored: %+v", o)
	}
}

func TestClipsCarryTimingAndAReadableLabel(t *testing.T) {
	cs := Clips(reel)
	vo, _ := byID(cs, "vo-1")
	if vo.StartMs != 200 || vo.DurationMs != 4000 {
		t.Fatalf("voice timing is %d/%d", vo.StartMs, vo.DurationMs)
	}
	// A voiceover is known by what it says — the only way to tell two of
	// them apart at a glance.
	if vo.Label != "Hit the 2 PM wall? Not any more." {
		t.Fatalf("voice label is %q", vo.Label)
	}
	scene, _ := byID(cs, "scene-hook")
	if scene.Label != "hook" {
		t.Fatalf("scene label is %q", scene.Label)
	}
	// Music has no words, so its file is the next most honest thing.
	music, _ := byID(cs, "bgm")
	if music.Label != "warm lo-fi beats" {
		t.Fatalf("music label is %q", music.Label)
	}
	shot, _ := byID(cs, "shot")
	if shot.Label != "image_001.jpg" {
		t.Fatalf("image label is %q", shot.Label)
	}
}

func TestClipsComeBackByTrackThenTime(t *testing.T) {
	cs := Clips(reel)
	for i := 1; i < len(cs); i++ {
		a, b := cs[i-1], cs[i]
		if a.TrackIndex > b.TrackIndex {
			t.Fatalf("row %d came after row %d", b.TrackIndex, a.TrackIndex)
		}
		if a.TrackIndex == b.TrackIndex && a.StartMs > b.StartMs {
			t.Fatalf("on row %d, %dms came after %dms", a.TrackIndex, b.StartMs, a.StartMs)
		}
	}
}

// An element with no id cannot be addressed by an edit, so offering it on
// a timeline someone drags things around on would be a lie.
func TestAClipWithoutAnIdIsNotReported(t *testing.T) {
	film := `<div id="root" data-composition-id="main" data-duration="3">
	  <audio data-media="bgm" src="a.wav" data-start="0" data-duration="3"></audio>
	</div>`
	if cs := Clips(film); len(cs) != 0 {
		t.Fatalf("reported %+v", cs)
	}
}

func TestAProjectCarriesItsClips(t *testing.T) {
	p, err := Build([]File{{Path: "index.html", Content: reel}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Clips) != 6 {
		t.Fatalf("project carries %d clips", len(p.Clips))
	}
	if len(p.Scenes) != 2 {
		t.Fatalf("scenes changed: %d", len(p.Scenes))
	}
}

// A clip timed with data-end rather than data-duration still has a length.
//
// Both spellings are in the composition contract. A clip reported as
// zero-length draws as a bar with no width, which reads as a broken
// timeline rather than as a film that chose the other attribute.
func TestClipsReadDataEnd(t *testing.T) {
	const doc = `<div id="root" data-composition-id="reel">
       <section id="s1" class="clip" data-scene="hook" data-start="0" data-end="3"></section>
       <audio id="score" class="clip" src="a.wav" data-start="1" data-end="0.5"></audio>
     </div>`

	byID := map[string]Clip{}
	for _, c := range Clips(doc) {
		byID[c.ElementID] = c
	}
	if got := byID["s1"].DurationMs; got != 3000 {
		t.Fatalf("data-end should give the scene a length: got %d, want 3000", got)
	}
	// An end BEFORE the start is not a length. Reported as zero rather than
	// as a negative, which would draw a bar running backwards.
	if got := byID["score"].DurationMs; got != 0 {
		t.Fatalf("an end before the start is no length: got %d, want 0", got)
	}
}

// A placed voiceover file is a voiceover, not music.
//
// The one pair the tag cannot tell apart, which is why media.go writes
// data-media alongside the file it resolved.
func TestClipsPlacedVoiceIsNotMusic(t *testing.T) {
	const doc = `<div id="root" data-composition-id="reel">
       <section id="s1" class="clip" data-scene="hook" data-start="0" data-duration="3">
         <audio id="vo" class="clip" data-media="voice" src="v.wav"
                data-media-resolved="v.wav" data-track-index="2"
                data-start="0" data-duration="3"></audio>
       </section>
     </div>`

	for _, c := range Clips(doc) {
		if c.ElementID != "vo" {
			continue
		}
		if c.Kind != ClipVoice {
			t.Fatalf("a placed voiceover is %q, want %q", c.Kind, ClipVoice)
		}
		if c.TrackIndex != 2 || !c.Authored {
			t.Fatalf("it keeps its authored row: track %d authored %v", c.TrackIndex, c.Authored)
		}
		return
	}
	t.Fatal("the voiceover was not reported as a clip at all")
}
