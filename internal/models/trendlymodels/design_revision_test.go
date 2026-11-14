package trendlymodels

import (
	"fmt"
	"testing"
)

// revs builds a list of n revisions numbered 1..n, each at its own prefix.
func revs(n int) []DesignRevision {
	out := make([]DesignRevision, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, DesignRevision{N: i, Prefix: fmt.Sprintf("design-projects/b/d/revisions/%d", i)})
	}
	return out
}

func TestPruneKeepsEverythingUnderTheCap(t *testing.T) {
	list, pruned := pruneDesignRevisions(revs(MaxDesignRevisions), "design-projects/b/d/revisions/10")
	if len(list) != MaxDesignRevisions {
		t.Fatalf("kept %d, want %d", len(list), MaxDesignRevisions)
	}
	if len(pruned) != 0 {
		t.Fatalf("pruned %v, want nothing", pruned)
	}
}

func TestPruneTakesTheOldestFirst(t *testing.T) {
	list, pruned := pruneDesignRevisions(revs(MaxDesignRevisions+3), "design-projects/b/d/revisions/13")
	if len(list) != MaxDesignRevisions {
		t.Fatalf("kept %d, want %d", len(list), MaxDesignRevisions)
	}
	if list[0].N != 4 {
		t.Fatalf("oldest kept is %d, want 4", list[0].N)
	}
	want := []string{
		"design-projects/b/d/revisions/1",
		"design-projects/b/d/revisions/2",
		"design-projects/b/d/revisions/3",
	}
	if len(pruned) != len(want) {
		t.Fatalf("pruned %v, want %v", pruned, want)
	}
	for i := range want {
		if pruned[i] != want[i] {
			t.Fatalf("pruned[%d] = %q, want %q", i, pruned[i], want[i])
		}
	}
}

// The case that costs a user their design if it is wrong: they restored
// revision 1, so the design is PLAYING revision 1's files while revision 1
// is the oldest entry and first in line to be pruned.
func TestPruneNeverDeletesWhatTheDesignIsPlaying(t *testing.T) {
	list := revs(MaxDesignRevisions + 1)
	current := "design-projects/b/d/revisions/1"
	kept, pruned := pruneDesignRevisions(list, current)
	if len(kept) != MaxDesignRevisions {
		t.Fatalf("kept %d, want %d", len(kept), MaxDesignRevisions)
	}
	for _, p := range pruned {
		if p == current {
			t.Fatalf("pruned the prefix the design points at: %q", p)
		}
	}
}

// A restore adds a SECOND entry on the same prefix. Dropping the older of
// the two must not delete the bytes they share.
func TestPruneKeepsFilesStillReferencedByAnotherEntry(t *testing.T) {
	list := revs(MaxDesignRevisions)
	shared := list[0].Prefix // revision 1's files…
	list = append(list, DesignRevision{N: 11, Prefix: shared, By: DesignRevisionRestored})

	kept, pruned := pruneDesignRevisions(list, shared)
	if len(kept) != MaxDesignRevisions {
		t.Fatalf("kept %d, want %d", len(kept), MaxDesignRevisions)
	}
	for _, p := range pruned {
		if p == shared {
			t.Fatalf("pruned %q, which revision 11 still points at", p)
		}
	}
	// And the restore entry survived.
	if kept[len(kept)-1].N != 11 {
		t.Fatalf("newest kept is %d, want 11", kept[len(kept)-1].N)
	}
}

func TestSortPutsRevisionsInOrder(t *testing.T) {
	list := []DesignRevision{{N: 3}, {N: 1}, {N: 12}, {N: 2}}
	sortDesignRevisions(list)
	for i, want := range []int{1, 2, 3, 12} {
		if list[i].N != want {
			t.Fatalf("list[%d] = %d, want %d", i, list[i].N, want)
		}
	}
}

func TestToIntReadsEveryShapeFirestoreReturns(t *testing.T) {
	// int64 off the wire, int from a local write, float64 from JSON.
	for _, v := range []any{int64(7), 7, float64(7)} {
		got, ok := toInt(v)
		if !ok || got != 7 {
			t.Fatalf("toInt(%T %v) = %d, %v", v, v, got, ok)
		}
	}
	if _, ok := toInt("7"); ok {
		t.Fatal("toInt accepted a string")
	}
	if _, ok := toInt(nil); ok {
		t.Fatal("toInt accepted nil")
	}
}

// A revision written before clips existed restores its own scenes, not
// whatever film was loaded a moment ago.
//
// This is the whole reason the restore writes `clips` unconditionally: the
// field used to be left alone when the revision had nothing to say, which
// left the timeline drawing the music and voiceovers of a different design.
func TestClipsOrFromScenesDerivesFromScenes(t *testing.T) {
	scenes := []DesignScene{
		{ID: "scene-hook", StartMs: 0, DurationMs: 3000},
		{ID: "scene-end", StartMs: 3000, DurationMs: 2000},
	}

	got := clipsOrFromScenes(nil, scenes)
	if len(got) != 2 {
		t.Fatalf("a two-scene revision should restore two clips, got %d", len(got))
	}
	for i, c := range got {
		if c.Kind != "scene" || c.TrackIndex != 0 {
			t.Fatalf("clip %d: a scene belongs on row 0, got kind %q row %d", i, c.Kind, c.TrackIndex)
		}
		if c.ElementID != scenes[i].ID || c.StartMs != scenes[i].StartMs || c.DurationMs != scenes[i].DurationMs {
			t.Fatalf("clip %d does not match its scene: %+v vs %+v", i, c, scenes[i])
		}
	}

	// A revision that HAS clips keeps them untouched — the derivation is a
	// fallback, not a rewrite.
	real := []DesignClip{{ElementID: "score", Kind: "music", TrackIndex: 3}}
	if out := clipsOrFromScenes(real, scenes); len(out) != 1 || out[0].ElementID != "score" {
		t.Fatalf("a revision with clips should keep them, got %+v", out)
	}

	// No scenes and no clips is an empty list rather than nil, so the
	// restore CLEARS the previous film's rows instead of leaving them.
	if out := clipsOrFromScenes(nil, nil); out == nil || len(out) != 0 {
		t.Fatalf("nothing to restore should clear the rows, got %+v", out)
	}
}
