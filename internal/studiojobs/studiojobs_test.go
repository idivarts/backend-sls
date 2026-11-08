package studiojobs

import "testing"

// Target decides whether a job is runnable or dropped. It is per type because
// the first version was not: it required RunID for every message, which would
// have silently discarded every scene design — a regenerate of a hand-built
// scene has no run at all.

func TestADirectorPlanNeedsARun(t *testing.T) {
	m := Message{Type: JobDirectorPlan, BrandID: "b", ContentID: "c", StudioID: "s"}
	if m.Target() {
		t.Fatal("a plan with no run has nothing to write to")
	}
	m.RunID = "r"
	if !m.Target() {
		t.Fatal("a plan naming its run is runnable")
	}
}

func TestASceneDesignNeedsASceneAndNotARun(t *testing.T) {
	m := Message{Type: JobSceneDesign, BrandID: "b", ContentID: "c", StudioID: "s"}
	if m.Target() {
		t.Fatal("a scene design with no scene has nothing to write to")
	}
	m.SceneID = "sc"
	if !m.Target() {
		t.Fatal("a scene design names a scene, not a run — this is the regression guard")
	}
}

func TestThePathIsAlwaysRequired(t *testing.T) {
	for _, m := range []Message{
		{Type: JobSceneDesign, ContentID: "c", StudioID: "s", SceneID: "sc"},
		{Type: JobSceneDesign, BrandID: "b", StudioID: "s", SceneID: "sc"},
		{Type: JobSceneDesign, BrandID: "b", ContentID: "c", SceneID: "sc"},
	} {
		if m.Target() {
			t.Errorf("a message missing part of its path is not runnable: %+v", m)
		}
	}
}

// Deploy skew: a newer producer sending a type this worker does not know.
// Guessing which job was meant would spend money on the wrong one.
func TestAnUnknownTypeIsNotRunnable(t *testing.T) {
	m := Message{Type: "something_new", BrandID: "b", ContentID: "c", StudioID: "s", SceneID: "sc", RunID: "r"}
	if m.Target() {
		t.Fatal("an unknown job type must be dropped, not guessed at")
	}
}
