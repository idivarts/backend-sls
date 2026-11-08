package trendlymodels

import "testing"

// runOutcome is the whole of "is this run over, and how did it go". It is
// tested on its own because the Firestore around it is bookkeeping, and
// because the bug it exists to fix — a run stuck `running` forever — was
// invisible until somebody read the document three hours later.

func sceneOf(runID string, pending bool, genStatus string) StudioScene {
	sc := StudioScene{RunID: runID, Pending: pending}
	if genStatus != "" {
		sc.Gen = &SceneGeneration{Status: genStatus}
	}
	return sc
}

func TestARunWithAPendingSceneDoesNotSettle(t *testing.T) {
	scenes := []StudioScene{
		sceneOf("r1", false, "done"),
		sceneOf("r1", true, "queued"),
	}
	if settle, _ := runOutcome(scenes, "r1"); settle {
		t.Fatal("a run still waiting on a scene is not over")
	}
}

func TestARunSettlesDoneWhenEverySceneHasLanded(t *testing.T) {
	scenes := []StudioScene{
		sceneOf("r1", false, "done"),
		sceneOf("r1", false, "done"),
	}
	settle, status := runOutcome(scenes, "r1")
	if !settle || status != RunDone {
		t.Fatalf("want settle/done, got %v/%q", settle, status)
	}
}

// The case that decides whether a user is told their video is gone. Six scenes
// on the rail and one that did not come back is a run that produced something.
func TestOneFailedSceneDoesNotFailTheWholeRun(t *testing.T) {
	scenes := []StudioScene{
		sceneOf("r1", false, "done"),
		sceneOf("r1", false, "failed"),
		sceneOf("r1", false, "done"),
	}
	settle, status := runOutcome(scenes, "r1")
	if !settle || status != RunDone {
		t.Fatalf("a run that made two of three scenes is done, got %v/%q", settle, status)
	}
}

func TestARunWhoseEverySceneFailedIsFailed(t *testing.T) {
	scenes := []StudioScene{
		sceneOf("r1", false, "failed"),
		sceneOf("r1", false, "failed"),
	}
	settle, status := runOutcome(scenes, "r1")
	if !settle || status != RunFailed {
		t.Fatalf("want settle/failed, got %v/%q", settle, status)
	}
}

// Scenes the user built by hand, and scenes from an earlier run, must not
// settle this one — nor keep it open.
func TestOtherRunsScenesAreIgnored(t *testing.T) {
	scenes := []StudioScene{
		sceneOf("r1", false, "done"),
		sceneOf("r2", true, "queued"), // another run, still working
		sceneOf("", true, ""),         // hand-built, pending for its own reasons
	}
	settle, status := runOutcome(scenes, "r1")
	if !settle || status != RunDone {
		t.Fatalf("another run's pending scene must not hold r1 open, got %v/%q", settle, status)
	}
}

// A run whose scenes were all deleted has no outcome to report. Closing it as
// `done` would claim work that no longer exists.
func TestARunWithNoScenesLeftDoesNotSettle(t *testing.T) {
	if settle, _ := runOutcome([]StudioScene{sceneOf("r2", false, "done")}, "r1"); settle {
		t.Fatal("a run that owns no scenes should not settle")
	}
}
