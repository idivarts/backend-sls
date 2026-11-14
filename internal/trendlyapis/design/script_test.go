package design

import (
	"strings"
	"testing"
)

const scriptFilm = `<!doctype html>
<html><head><style>.caption-track span{color:#fff}</style></head>
<body>
  <div id="root" data-composition-id="main" data-duration="9">
    <section id="s1" data-scene="hook" data-start="0" data-duration="3">
      <h1 id="hook-title" class="clip" data-start="0.2" data-duration="2.6">Hit the <em>2 PM</em> wall?</h1>
      <p id="hook-sub" class="clip" data-start="1" data-duration="2">Cold brew, no sugar</p>
      <audio id="vo-hook" data-media="voice" data-media-intent="Hit the two PM wall?"
             data-media-resolved="assets/voice/a.wav" src="assets/voice/a.wav"
             data-start="0" data-duration="3"></audio>
      <div id="cap-hook" class="caption-track" data-captions="vo-hook" data-start="0" data-duration="3">
        <span class="clip" data-start="0.3" data-duration="0.4">Hit</span>
      </div>
    </section>
    <section id="s2" data-scene="end" data-start="3" data-duration="6">
      <h1 id="end-title" class="clip" data-start="3.2" data-duration="2">Fuel your focus</h1>
      <img id="end-shot" class="clip" src="assets/a.jpg" data-start="3" data-duration="3" />
    </section>
  </div>
  <script>gsap.timeline();</script>
</body></html>`

func TestTheScriptIsTheFilmsCopyInOrder(t *testing.T) {
	lines := readScript(scriptFilm)
	var got []string
	for _, l := range lines {
		got = append(got, l.Kind+":"+l.Text)
	}
	want := []string{
		"voice:Hit the two PM wall?",
		"text:Hit the 2 PM wall?",
		"text:Cold brew, no sugar",
		"text:Fuel your focus",
	}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("script is:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestTheScriptKnowsWhichSceneEachLineIsIn(t *testing.T) {
	for _, l := range readScript(scriptFilm) {
		switch l.ElementID {
		case "hook-title", "hook-sub", "vo-hook":
			if l.SceneID != "hook" {
				t.Fatalf("%s is in scene %q", l.ElementID, l.SceneID)
			}
		case "end-title":
			if l.SceneID != "end" {
				t.Fatalf("%s is in scene %q", l.ElementID, l.SceneID)
			}
		}
	}
}

// What is NOT a line: the root, a scene, a caption track (generated from
// the voiceover and about to be regenerated), and an image.
func TestTheScriptLeavesOutWhatIsNotCopy(t *testing.T) {
	for _, l := range readScript(scriptFilm) {
		for _, never := range []string{"root", "s1", "s2", "cap-hook", "end-shot"} {
			if l.ElementID == never {
				t.Fatalf("%q is in the script", never)
			}
		}
	}
}

// A line with markup inside it is still one line.
func TestALineWithMarkupInsideItReadsAsItSpeaks(t *testing.T) {
	for _, l := range readScript(scriptFilm) {
		if l.ElementID == "hook-title" && l.Text != "Hit the 2 PM wall?" {
			t.Fatalf("the headline reads %q", l.Text)
		}
	}
}

func TestRewritingAVoiceoverAsksForItToBeSpokenAgain(t *testing.T) {
	words := "Tired of sugar? Try it black."
	out, err := applyElementEdit(scriptFilm, editElementRequest{ElementID: "vo-hook", Voice: &words})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `data-media-intent="Tired of sugar? Try it black."`) {
		t.Fatalf("the words did not change:\n%s", out)
	}
	// Both of these are what tell the resolver it is already done, so both
	// have to go or the film keeps saying the old line.
	if !strings.Contains(out, `src=""`) || !strings.Contains(out, `data-media-resolved=""`) {
		t.Fatalf("the old audio is still attached:\n%s", out)
	}
	// And it is still a voice declaration, or nothing would resolve it.
	if !strings.Contains(out, `data-media="voice"`) {
		t.Fatal("the declaration was lost")
	}
}

func TestOnlyAVoiceoverCanBeGivenWords(t *testing.T) {
	words := "x"
	if _, err := applyElementEdit(scriptFilm, editElementRequest{ElementID: "hook-title", Voice: &words}); err == nil {
		t.Fatal("accepted a voiceover edit on a headline")
	}
	empty := "   "
	if _, err := applyElementEdit(scriptFilm, editElementRequest{ElementID: "vo-hook", Voice: &empty}); err == nil {
		t.Fatal("accepted a voiceover that says nothing")
	}
}

// Editing a line of on-screen text goes through the same path as the
// inspector — one write path, not two.
func TestEditingATextLineIsAnOrdinaryTextEdit(t *testing.T) {
	words := "Tired of sugar?"
	out, err := applyElementEdit(scriptFilm, editElementRequest{ElementID: "hook-sub", Text: &words})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, ">Tired of sugar?</p>") {
		t.Fatalf("the line did not change:\n%s", out)
	}
}
