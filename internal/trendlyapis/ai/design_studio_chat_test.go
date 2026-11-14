package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// ⚠️ The studio chat must not run on the chat lane.
//
// It IS a chat, so TaskChat is the obvious answer and the wrong one:
// Resolve takes the first allowed model the plan unlocks, and the two task
// lists start differently. Routing the studio to TaskChat silently drops
// it from Opus to Flash, with the dropdown still naming Opus.
func TestDesignStudioRunsOnTheDesignLane(t *testing.T) {
	if got := TaskForModule(moduleDesign); got != openrouter.TaskDesign {
		t.Fatalf("the studio chat runs on %q, want %q", got, openrouter.TaskDesign)
	}
	if got := TaskForModule(moduleContent); got != openrouter.TaskDesign {
		t.Fatalf("the content surface authors designs too: got %q", got)
	}
	for _, m := range []string{moduleStrategy, moduleCalendar, moduleOnboarding, "general", ""} {
		if got := TaskForModule(m); got != openrouter.TaskChat {
			t.Fatalf("module %q should be an ordinary chat, got %q", m, got)
		}
	}
}

// The default model for a film is Claude Opus 4.8, and it is the task list
// that decides it — Resolve takes the first allowed entry.
func TestTheStudioLeadsWithOpus(t *testing.T) {
	// Read from the built-in registry's own source rather than a live
	// lookup: Firestore's ai_config REPLACES this at runtime, so a test
	// against the live registry would pass or fail on a document rather
	// than on the decision recorded here.
	raw, err := os.ReadFile("../../../pkg/openrouter/models.go")
	if err != nil {
		t.Fatalf("could not read the registry: %v", err)
	}
	line := ""
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.Contains(l, "TaskDesign:") && strings.Contains(l, "Allowed") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatal("the design task is not in the built-in registry")
	}
	first := strings.Index(line, "Allowed: []string{\"")
	if first < 0 {
		t.Fatalf("could not read the design task's model list: %s", line)
	}
	rest := line[first+len("Allowed: []string{\""):]
	lead := rest[:strings.Index(rest, "\"")]
	if lead != "anthropic/claude-opus-4.8" {
		t.Fatalf("the studio leads with %q; the decision on record is anthropic/claude-opus-4.8", lead)
	}
}

// The studio gets the tools that change ITS film, and not the ones that
// change a content's design.
func TestDesignStudioToolsAreTheRightOnes(t *testing.T) {
	have := map[string]bool{}
	for _, tool := range toolsForModule(moduleDesign) {
		have[tool.Function.Name] = true
	}
	for _, want := range []string{toolReadHyperframesProject, toolWriteHyperframesProject, toolStartDesignAuthoring} {
		if !have[want] {
			t.Fatalf("the studio chat cannot %s", want)
		}
	}
	// ⚠️ generate_design and apply_design_edits work on a CONTENT's own HTML
	// design revision, not on a v3 film. Offered here, a model would call
	// one with a designId it has no business with and replace a film with a
	// slide.
	for _, unwanted := range []string{toolGenerateDesign, toolApplyDesignEdits} {
		if have[unwanted] {
			t.Fatalf("the studio chat is offered %s, which works on a different document", unwanted)
		}
	}
	// Read before write: the write tool replaces the project whole.
	var desc string
	for _, tool := range designStudioTools() {
		if tool.Function.Name == toolReadHyperframesProject {
			desc = tool.Function.Description
		}
	}
	if !strings.Contains(strings.ToLower(desc), "before write_hyperframes_project") {
		t.Fatal("the read tool does not tell the model to read before it writes")
	}

	// ⚠️ A first authoring is a JOB, and the model must be told which tool
	// is which. Writing a film inside the turn means it dies with the
	// socket — every disconnect a lost film the org already paid for.
	var authorDesc string
	for _, tool := range designStudioTools() {
		if tool.Function.Name == toolStartDesignAuthoring {
			authorDesc = strings.ToLower(tool.Function.Description)
		}
	}
	for _, phrase := range []string{"not write_hyperframes_project", "background job"} {
		if !strings.Contains(authorDesc, phrase) {
			t.Fatalf("the authoring tool does not say %q, so the model will write inline instead", phrase)
		}
	}
}

// The project read hands back the film's source and not its media.
func TestProjectReadSkipsMedia(t *testing.T) {
	for _, src := range []string{"index.html", "compositions/hook.html", "style.css", "data.json"} {
		if !isProjectSource(src) {
			t.Fatalf("%q is part of the project the model writes", src)
		}
	}
	for _, asset := range []string{
		".media/audio/bgm/bgm_001.wav", "assets/pour.jpg", "player.html", "", "frames/0001.png",
	} {
		if isProjectSource(asset) {
			t.Fatalf("%q is not source; handing a model its bytes spends the context on nothing", asset)
		}
	}
}

// A file's key is the prefix joined to its relative path.
//
// ── Why this test exists ─────────────────────────────────────────────────
// mediastore.ListPrefix returns paths RELATIVE to the prefix and
// mediastore.Get takes a FULL key, and the read tool passed the first
// straight into the second. The bucket was asked for "index.html" at its
// root, every source came back unreadable, and the file LIST still looked
// right — so the model saw a film it could not open and said it must still
// be being written. The studio chat could not read a single film from the
// day it shipped.
func TestProjectKeyIsAbsolute(t *testing.T) {
	const prefix = "design-projects/brand1/design1/revisions/3"

	for _, c := range []struct{ rel, want string }{
		{"index.html", prefix + "/index.html"},
		{"compositions/hook.html", prefix + "/compositions/hook.html"},
		// A leading slash on either side must not double up.
		{"/index.html", prefix + "/index.html"},
	} {
		if got := projectKey(prefix, c.rel); got != c.want {
			t.Fatalf("projectKey(%q, %q) = %q, want %q", prefix, c.rel, got, c.want)
		}
	}
	if projectKey(prefix+"/", "index.html") != prefix+"/index.html" {
		t.Fatal("a trailing slash on the prefix must not double up")
	}
	// The bug, named: the key must never be the bare relative path.
	if projectKey(prefix, "index.html") == "index.html" {
		t.Fatal("the key is the relative path — Get would look at the bucket root")
	}
}

// Media a project ASKS for is not media it HAS.
//
// ── Why this matters ─────────────────────────────────────────────────────
// A project declares media as an element with data-media-intent and no src,
// and a later pass fetches the file and fills the src in. Authoring runs
// that pass; the chat's write did not. A user asked for music, the model
// correctly declared `<audio data-media="bgm" data-media-intent="warm,
// minimal, no vocals">`, said it had added the track — and the film played
// in silence, because no file was ever fetched. Counting the declarations
// is what lets the write queue the fetch and stop the model claiming the
// music is already there.
func TestUnresolvedMediaCountsDeclarationsOnly(t *testing.T) {
	p := &hfproject.Project{Clips: []hfproject.Clip{
		// Declared, no file: these are the ones that need fetching.
		{Kind: hfproject.ClipMusic, ElementID: "score"},
		{Kind: hfproject.ClipVoice, ElementID: "vo-1"},
		{Kind: hfproject.ClipImage, ElementID: "hero", Src: "   "},
		// Resolved: a file is already behind them.
		{Kind: hfproject.ClipVideo, ElementID: "b-roll", Src: ".media/video/1.mp4"},
		{Kind: hfproject.ClipSound, ElementID: "whoosh", Src: ".media/audio/sfx/1.wav"},
		// Not media at all — a scene has no file and never will.
		{Kind: hfproject.ClipScene, ElementID: "scene-1"},
		{Kind: hfproject.ClipCaptions, ElementID: "cc"},
	}}
	if got := unresolvedMedia(p); got != 3 {
		t.Fatalf("unresolvedMedia = %d, want 3 (the music, the voiceover and the image)", got)
	}
	if got := unresolvedMedia(&hfproject.Project{}); got != 0 {
		t.Fatalf("a project with no clips has nothing pending, got %d", got)
	}
}

// A reference the film cannot satisfy is caught while the model can still
// change it.
//
// The alternative is the gate finding it minutes later — "404 loading
// assets/fonts/space-grotesk-700.woff2" in a red banner over a film that
// will not finish. The model cannot upload a woff2, so the only useful
// moment to say so is while it is being told what it just saved.
func TestDanglingAssetsNamesWhatIsMissing(t *testing.T) {
	project := &hfproject.Project{Files: []hfproject.File{{
		Path: "index.html",
		Content: `<style>@font-face{src:url('assets/fonts/space-grotesk-700.woff2')}` +
			`:root{--brand-logo:url('assets/brand-logo.png')}</style>` +
			`<img src="assets/fonts/inter-400.woff2">`,
	}}}
	kept := map[string]bool{"assets/fonts/inter-400.woff2": true}

	got := danglingAssets(project, kept)
	if len(got) != 2 {
		t.Fatalf("danglingAssets = %v, want the font and the logo", got)
	}
	for _, want := range []string{"assets/fonts/space-grotesk-700.woff2", "assets/brand-logo.png"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q is referenced and not in the film, but was not reported: %v", want, got)
		}
	}
	// A file the film DOES have is not a problem, and must not be reported
	// — a warning that cries wolf is one the model learns to write past.
	for _, g := range got {
		if g == "assets/fonts/inter-400.woff2" {
			t.Fatal("a file carried forward into the revision was reported as missing")
		}
	}
	if n := len(danglingAssets(project, map[string]bool{
		"assets/fonts/space-grotesk-700.woff2": true,
		"assets/brand-logo.png":                true,
		"assets/fonts/inter-400.woff2":         true,
	})); n != 0 {
		t.Fatalf("nothing is missing, got %d", n)
	}
}

// The chat is told what is wrong with the film, because it is the only
// party that can fix it.
func TestDesignFaultCarriesTheGateFindings(t *testing.T) {
	d := &trendlymodels.Design{
		PreviewStatus:   trendlymodels.DesignPreviewFailed,
		PreviewError:    "404 loading assets/fonts/space-grotesk-700.woff2",
		PreviewFindings: []string{"404 loading assets/fonts/space-grotesk-700.woff2"},
	}
	got := designFault(d)
	for _, want := range []string{"FAILED", "space-grotesk-700.woff2", "assets/"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the fault does not mention %q:\n%s", want, got)
		}
	}
	if designFault(&trendlymodels.Design{}) != "" {
		t.Fatal("a film with nothing wrong with it says nothing")
	}
}

// Declared media and real media look identical on a timeline; only one of
// them plays.
func TestPendingMediaCountsWhatThePlayerCannotPlay(t *testing.T) {
	got := pendingMedia([]trendlymodels.DesignClip{
		{Kind: "music"},                          // declared
		{Kind: "voice", Src: " "},                // declared, blank src
		{Kind: "image", Src: ".media/img/1.png"}, // real
		{Kind: "scene"},                          // never has a file
		{Kind: "captions"},                       // nor this
	})
	if got != 2 {
		t.Fatalf("pendingMedia = %d, want 2 (the music and the voiceover)", got)
	}
}

// The studio chat can ask for a film to be finished.
func TestPrepareToolIsOffered(t *testing.T) {
	var desc string
	for _, tool := range designStudioTools() {
		if tool.Function.Name == toolPrepareDesign {
			desc = strings.ToLower(tool.Function.Description)
		}
	}
	if desc == "" {
		t.Fatal("prepare_design is not offered, so the model cannot fetch declared media")
	}
	// The description has to say WHY, or the model will not reach for it:
	// a declaration looks like a finished clip from where it is standing.
	for _, phrase := range []string{"declare", "silent", "queued"} {
		if !strings.Contains(desc, phrase) {
			t.Fatalf("the prepare tool does not mention %q", phrase)
		}
	}
}
