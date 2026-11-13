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

// A v3 design author job has no contentId and no studioId — a v3 design is a
// first-class object. The content/studio check is therefore per type, and if
// anyone hoists it back out of the switch every one of these is dropped
// silently: the job vanishes and the design sits at "writing" for ever.
func TestADesignAuthorJobNeedsOnlyABrandAndADesign(t *testing.T) {
	m := Message{Type: JobDesignAuthor, BrandID: "b", DesignID: "d"}
	if !m.Target() {
		t.Fatal("a design author job naming its brand and design is runnable")
	}
	if (Message{Type: JobDesignAuthor, BrandID: "b"}).Target() {
		t.Fatal("without a designId there is nothing to write to")
	}
	if (Message{Type: JobDesignAuthor, DesignID: "d"}).Target() {
		t.Fatal("without a brandId there is no scope")
	}
}

// Guards the hoist directly: the other two types must still demand the pair.
func TestTheContentStudioPairIsStillRequiredWhereItApplies(t *testing.T) {
	if (Message{Type: JobDirectorPlan, BrandID: "b", RunID: "r"}).Target() {
		t.Fatal("a plan still needs its content and studio")
	}
	if (Message{Type: JobSceneDesign, BrandID: "b", SceneID: "s"}).Target() {
		t.Fatal("a scene design still needs its content and studio")
	}
}
