package design

import (
	"strings"
	"testing"
)

const captionedFilm = `<div id="root" data-composition-id="main" data-duration="9">
  <section id="s1" data-scene="hook" data-start="0" data-duration="5">
    <h1 id="s1-title" class="clip" data-start="0" data-duration="5">Hit the 2 PM wall?</h1>
    <audio id="s1-vo" data-media="voice" data-media-intent="Hit the 2 PM wall? Not any more."
           data-media-resolved="assets/voice/a.wav" src="assets/voice/a.wav"
           data-start="0.2" data-duration="4"></audio>
    <div data-captions-for="assets/voice/a.wav" id="s1-captions" class="caption-track"
         data-captions="s1-vo" data-start="0.2" data-duration="4"><span
      class="clip" data-start="0.20" data-duration="0.32">Hit</span> <span
      class="clip" data-start="0.55" data-duration="0.21">the</span> <span
      class="clip" data-start="0.80" data-duration="0.45">2PM</span></div>
  </section>
  <section id="s2" data-scene="outro" data-start="5" data-duration="4">
    <div id="s2-captions" class="caption-track" data-captions="s2-vo"
         data-start="5.2" data-duration="3"></div>
    <audio id="s2-vo" data-media="voice" data-media-intent="Real coffee, real cold."
           data-start="5.2" data-duration="3"></audio>
  </section>
</div>`

func TestCaptionsReadBackWithTheirWordsAndTiming(t *testing.T) {
	tracks := readCaptions(captionedFilm)
	if len(tracks) != 2 {
		t.Fatalf("found %d tracks, want 2", len(tracks))
	}

	first := tracks[0]
	if first.ElementID != "s1-captions" || first.SceneID != "hook" || first.Follows != "s1-vo" {
		t.Fatalf("first track is %+v", first)
	}
	if first.StartMs != 200 {
		t.Fatalf("first track starts at %d", first.StartMs)
	}
	// What the voiceover was asked to say, beside what the transcriber
	// heard — the two are not always the same, and the panel shows both.
	if first.Spoken != "Hit the 2 PM wall? Not any more." {
		t.Fatalf("spoken line is %q", first.Spoken)
	}
	if len(first.Words) != 3 {
		t.Fatalf("got %d words", len(first.Words))
	}
	if first.Words[0] != (captionWord{Text: "Hit", StartMs: 200, DurationMs: 320}) {
		t.Fatalf("first word is %+v", first.Words[0])
	}
	if first.Words[2].Text != "2PM" || first.Words[2].StartMs != 800 {
		t.Fatalf("third word is %+v", first.Words[2])
	}
}

// A track that has not been filled yet is still a track, and the panel
// says it is empty rather than pretending it is not there.
func TestAnUnfilledCaptionTrackIsStillReported(t *testing.T) {
	tracks := readCaptions(captionedFilm)
	second := tracks[1]
	if second.ElementID != "s2-captions" {
		t.Fatalf("second track is %q", second.ElementID)
	}
	if len(second.Words) != 0 {
		t.Fatalf("an empty track reported %d words", len(second.Words))
	}
	if second.Spoken != "Real coffee, real cold." {
		t.Fatalf("spoken line is %q", second.Spoken)
	}
}

// A filled track on an older film has lost its data-captions. It is still
// a caption track, by the class the contract gives it.
func TestATrackThatLostItsAttributeIsStillFound(t *testing.T) {
	film := strings.Replace(captionedFilm, ` data-captions="s1-vo"`, "", 1)
	tracks := readCaptions(film)
	if len(tracks) != 2 {
		t.Fatalf("found %d tracks, want 2", len(tracks))
	}
	if tracks[0].Follows != "" {
		t.Fatalf("it claims to follow %q", tracks[0].Follows)
	}
	// The words are still readable, which is what the panel shows.
	if len(tracks[0].Words) != 3 {
		t.Fatalf("got %d words", len(tracks[0].Words))
	}
}

// Caption tracks are not lines of the script: they are a transcript of
// something that is regenerated, and editing them would be editing the
// wrong thing.
func TestCaptionsAreNotInTheScript(t *testing.T) {
	for _, l := range readScript(captionedFilm) {
		if strings.Contains(l.ElementID, "captions") {
			t.Fatalf("%q is in the script", l.ElementID)
		}
	}
}
