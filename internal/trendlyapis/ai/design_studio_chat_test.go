package ai

import (
	"os"
	"strings"
	"testing"

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
