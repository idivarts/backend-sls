package trendlymodels

import (
	"context"
	"strings"
	"testing"

	"cloud.google.com/go/firestore"
)

// Audio lanes hold media references, and nothing counted them until now — a
// soundtrack could sit on a studio with refCount 0 while an archive sweep was
// free to take the bytes. These lock the two pieces of the fix in place.
func TestLaneMediaIDsAreDistinctAndOrdered(t *testing.T) {
	audio := &StudioAudio{Lanes: []AudioLane{
		{ID: "l1", Kind: "music", MediaID: "bed"},
		{ID: "l2", Kind: "voiceover", MediaID: "vo1"},
		// The same row twice is one reference to count, not two: the counter
		// tracks whether anything uses the bytes, not how many times.
		{ID: "l3", Kind: "voiceover", MediaID: "vo1"},
		{ID: "l4", Kind: "sfx", MediaID: ""},
	}}
	got := LaneMediaIDs(audio)
	want := []string{"bed", "vo1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if ids := LaneMediaIDs(nil); len(ids) != 0 {
		t.Errorf("nil audio should have no media, got %v", ids)
	}
}

// The guard is the part that keeps the fix true later. Without it the next
// handler to write `audio` through the generic updater reopens the hole, and
// the symptom — a render that comes back silent weeks afterwards — points
// nowhere near the cause.
//
// ⚠️ Every assertion here checks the error's TEXT, not just that there was one.
// With no credentials the Firestore call fails anyway, so `err != nil` is true
// whether the guard fired or not: an earlier version of this test passed with
// the guard deliberately broken.
func TestUpdateStudioRefusesTheAudioObject(t *testing.T) {
	refused := func(t *testing.T, u firestore.Update, label string) {
		t.Helper()
		err := UpdateStudio(context.Background(), "b", "c", "s", []firestore.Update{u})
		if err == nil || !strings.Contains(err.Error(), "SetStudioAudio") {
			t.Errorf("UpdateStudio should refuse %s by the guard, got %v", label, err)
		}
	}
	for _, path := range []string{"audio", "audio.lanes"} {
		refused(t, firestore.Update{Path: path, Value: nil}, "Path "+path)
	}
	// The other spelling. Firestore treats a FieldPath slice identically, so a
	// guard that only knows dotted strings is one the next handler walks past
	// without noticing it has done anything wrong.
	for _, fp := range []firestore.FieldPath{{"audio"}, {"audio", "lanes"}} {
		refused(t, firestore.Update{FieldPath: fp, Value: nil}, "FieldPath "+strings.Join(fp, "."))
	}

	// The two fields that name no media stay allowed, or parking the
	// director's request would need the heavyweight path for nothing. This
	// reaches Firestore and fails there, which is not what is under test — the
	// assertion is only that the guard did not stop it.
	err := UpdateStudio(context.Background(), "b", "c", "s",
		[]firestore.Update{{Path: "audio.voiceId", Value: "v"}})
	if err != nil && strings.Contains(err.Error(), "SetStudioAudio") {
		t.Errorf("audio.voiceId should not be refused by the guard: %v", err)
	}
}
