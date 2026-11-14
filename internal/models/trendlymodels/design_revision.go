package trendlymodels

// Revisions of a design's project — the way back.
//
// ── Why a design needs this before it can be edited ───────────────────────
// Everything that writes a design's project used to OVERWRITE it: one
// prefix, one copy, and the previous film gone the moment the next one
// landed. That was survivable while the only writer was "author the whole
// film from a brief", because the input was the brief and you could run it
// again. It stops being survivable the moment an AI edit can rewrite one
// element of a film you liked: an edit with no way back is worse than no
// edit at all.
//
// So every write becomes a revision. The files go to
// design-projects/{brand}/{design}/revisions/{n}/, and the design document
// holds a short list of what was written when, by what, and where it is.
//
// ── The pointer is projectPrefix, and it already existed ──────────────────
// The renderer, the preview and the player all read the project from the
// design's projectPrefix. Nothing computes that path for itself. So making
// them read a revision needs no change in any of them: the pointer moves,
// and they follow it. Restore is the same write in the other direction.
//
// ── Why the list lives on the design document ─────────────────────────────
// Not a subcollection. The app already subscribes to the design document to
// draw the studio, so a list of revisions on it arrives with everything
// else and needs no second query, no index, and no extra read per open. It
// is bounded (see MaxDesignRevisions) precisely so that stays true.

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// MaxDesignRevisions is how many revisions of one design are kept.
//
// A project is a few hundred kilobytes, so fifty of them is a design that
// costs more to keep than it did to make — and nobody restores the
// forty-first version of anything. Ten covers a working session with room
// to change your mind, and keeps the design document small enough that
// carrying the list on it stays the cheap option.
const MaxDesignRevisions = 10

// How a revision came to exist. Shown to the user, so these are the words
// the app prints, not internal names.
const (
	DesignRevisionAuthored  = "authoring"
	DesignRevisionAIEdit    = "ai-edit"
	DesignRevisionInspector = "inspector"
	DesignRevisionRestored  = "restore"
)

// DesignRevision is one saved state of a design's project.
type DesignRevision struct {
	// N is the revision number, counting from 1 and never reused.
	N int `json:"n" firestore:"n"`

	// Prefix is where this revision's files are. The design's
	// projectPrefix is a copy of exactly one of these.
	Prefix string `json:"prefix" firestore:"prefix"`

	// PreviewURL is the player page written beside those files. Kept per
	// revision because the player plays the project next to it: restoring
	// has to move this too, or the app plays the newest film while the
	// renderer renders the restored one.
	PreviewURL string `json:"previewUrl,omitempty" firestore:"previewUrl,omitempty"`

	// At is when it was written, epoch milliseconds.
	At int64 `json:"at" firestore:"at"`

	// By is what wrote it: one of the DesignRevision* constants.
	By string `json:"by" firestore:"by"`

	// Note is one line about what changed, for the list in the app.
	Note string `json:"note,omitempty" firestore:"note,omitempty"`

	// DurationMs and Scenes are the shape of the film at this revision, so
	// a restore can put the timeline back without re-parsing the project.
	DurationMs int           `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`
	Scenes     []DesignScene `json:"scenes,omitempty" firestore:"scenes,omitempty"`
}

// AllocateDesignRevision reserves the next revision number for a design.
//
// In a transaction, and against a counter rather than the length of the
// list, for two reasons. The list is pruned, so its length is not the
// number of revisions there have ever been — reusing a number would point
// two entries at one directory and make a restore ambiguous. And two
// writers racing (an AI edit while an authoring run finishes) must not get
// the same number: whoever commits second would otherwise overwrite the
// other's files at the very moment this whole mechanism exists to prevent.
//
// A number allocated and then not used is simply skipped. Gaps are cheaper
// than collisions.
func AllocateDesignRevision(ctx context.Context, brandID, designID string) (int, error) {
	if brandID == "" || designID == "" {
		return 0, fmt.Errorf("AllocateDesignRevision: empty brandID or designID")
	}
	ref := designDoc(brandID, designID)
	var next int
	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		counter := 0
		if v, err := snap.DataAt("revisionCounter"); err == nil {
			if n, ok := toInt(v); ok {
				counter = n
			}
		}
		// A design written before revisions existed has no counter and no
		// list: its current files sit at the legacy root prefix, and the
		// first revision it gets is 1.
		next = counter + 1
		return tx.Update(ref, []firestore.Update{{Path: "revisionCounter", Value: next}})
	})
	if err != nil {
		return 0, err
	}
	return next, nil
}

// CommitDesignRevision records a revision and makes it the current one.
//
// One transaction, because the entry and the pointer have to move together:
// a pointer at a revision the list does not mention cannot be restored
// from, and a list entry nothing points at is a revision the user is told
// they have and cannot get back to.
//
// Returns the prefixes of any revisions pruned by the retention cap, for
// the caller to delete. They are returned rather than deleted here so that
// a model stays a model — and so that a failure to delete bytes can never
// fail the write that matters.
func CommitDesignRevision(
	ctx context.Context,
	brandID, designID string,
	rev DesignRevision,
) ([]string, error) {
	if brandID == "" || designID == "" {
		return nil, fmt.Errorf("CommitDesignRevision: empty brandID or designID")
	}
	if rev.N <= 0 || rev.Prefix == "" {
		return nil, fmt.Errorf("CommitDesignRevision: a revision needs a number and a prefix")
	}
	if rev.At == 0 {
		rev.At = nowMs()
	}
	ref := designDoc(brandID, designID)
	var pruned []string
	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		pruned = nil
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		list := readDesignRevisions(snap)

		// Replace rather than append if this number is already listed. It
		// should not happen — numbers come from the counter — but a retry
		// that got as far as committing must not leave two entries that
		// disagree about where revision 7 is.
		replaced := false
		for i := range list {
			if list[i].N == rev.N {
				list[i] = rev
				replaced = true
				break
			}
		}
		if !replaced {
			list = append(list, rev)
		}

		list, pruned = pruneDesignRevisions(list, rev.Prefix)

		updates := []firestore.Update{
			{Path: "revisions", Value: list},
			{Path: "projectPrefix", Value: rev.Prefix},
		}
		if rev.DurationMs > 0 {
			updates = append(updates, firestore.Update{Path: "durationMs", Value: rev.DurationMs})
		}
		if rev.Scenes != nil {
			updates = append(updates, firestore.Update{Path: "scenes", Value: rev.Scenes})
		}
		// An empty previewUrl CLEARS the old one, on the same reasoning as
		// SetBrandDesignProject: a revision whose player could not be
		// written must not leave the app playing the previous film.
		if rev.PreviewURL != "" {
			updates = append(updates, firestore.Update{Path: "previewUrl", Value: rev.PreviewURL})
		} else {
			updates = append(updates, firestore.Update{Path: "previewUrl", Value: firestore.Delete})
		}
		return tx.Update(ref, updates)
	})
	if err != nil {
		return nil, err
	}
	return pruned, nil
}

// RestoreDesignRevision points a design back at a revision it already has.
//
// It writes no files: the revision's bytes are still where they were, which
// is the whole point of keeping them. It moves the pointer, the player URL
// and the timeline together, and it appends a new entry recording that a
// restore happened — so restoring is itself undoable, and the list reads as
// a history rather than quietly rewriting itself.
//
// Returns the restored revision so the caller can answer with it.
func RestoreDesignRevision(ctx context.Context, brandID, designID string, n int) (*DesignRevision, []string, error) {
	if brandID == "" || designID == "" {
		return nil, nil, fmt.Errorf("RestoreDesignRevision: empty brandID or designID")
	}
	if n <= 0 {
		return nil, nil, fmt.Errorf("RestoreDesignRevision: %d is not a revision number", n)
	}
	ref := designDoc(brandID, designID)
	var restored DesignRevision
	var pruned []string
	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		pruned = nil
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		list := readDesignRevisions(snap)
		var found *DesignRevision
		for i := range list {
			if list[i].N == n {
				found = &list[i]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("this design has no revision %d", n)
		}
		restored = *found

		counter := len(list)
		if v, err := snap.DataAt("revisionCounter"); err == nil {
			if c, ok := toInt(v); ok && c > counter {
				counter = c
			}
		}
		entry := DesignRevision{
			N:          counter + 1,
			Prefix:     found.Prefix,
			PreviewURL: found.PreviewURL,
			At:         nowMs(),
			By:         DesignRevisionRestored,
			Note:       fmt.Sprintf("Restored revision %d", n),
			DurationMs: found.DurationMs,
			Scenes:     found.Scenes,
		}
		list = append(list, entry)
		list, pruned = pruneDesignRevisions(list, entry.Prefix)

		updates := []firestore.Update{
			{Path: "revisions", Value: list},
			{Path: "revisionCounter", Value: entry.N},
			{Path: "projectPrefix", Value: entry.Prefix},
		}
		if entry.DurationMs > 0 {
			updates = append(updates, firestore.Update{Path: "durationMs", Value: entry.DurationMs})
		}
		if entry.Scenes != nil {
			updates = append(updates, firestore.Update{Path: "scenes", Value: entry.Scenes})
		}
		if entry.PreviewURL != "" {
			updates = append(updates, firestore.Update{Path: "previewUrl", Value: entry.PreviewURL})
		}
		// A restore invalidates the rendered film: it was made from the
		// revision that was current a moment ago. Clearing the status is
		// what puts the Render button back rather than offering a download
		// of the wrong film.
		updates = append(updates,
			firestore.Update{Path: "renderStatus", Value: firestore.Delete},
			firestore.Update{Path: "renderUrl", Value: firestore.Delete},
		)
		return tx.Update(ref, updates)
	})
	if err != nil {
		return nil, nil, err
	}
	return &restored, pruned, nil
}

// readDesignRevisions pulls the list off a snapshot, tolerating a design
// that has none.
func readDesignRevisions(snap *firestore.DocumentSnapshot) []DesignRevision {
	var holder struct {
		Revisions []DesignRevision `firestore:"revisions"`
	}
	if err := snap.DataTo(&holder); err != nil || holder.Revisions == nil {
		return nil
	}
	return holder.Revisions
}

// pruneDesignRevisions trims the list to the retention cap and says which
// revisions' files can go.
//
// `keepPrefix` is the prefix the design now points at, and it is never
// reported as prunable. Two cases need that, and both are real: a design
// restored to an old revision would otherwise have the files it is CURRENTLY
// playing deleted by the next edit, and a restore puts a second entry on the
// same prefix, so dropping the older of the two must not take the bytes they
// share.
//
// Returns the list to store and the prefixes to delete, in that order.
func pruneDesignRevisions(list []DesignRevision, keepPrefix string) ([]DesignRevision, []string) {
	sortDesignRevisions(list)
	var pruned []string
	for len(list) > MaxDesignRevisions {
		gone := list[0]
		list = list[1:]
		if gone.Prefix == "" || gone.Prefix == keepPrefix {
			continue
		}
		// Still referenced by an entry we are keeping — a restore makes
		// exactly this shape — so the files stay.
		if prefixInRevisions(list, gone.Prefix) {
			continue
		}
		pruned = append(pruned, gone.Prefix)
	}
	return list, pruned
}

func prefixInRevisions(list []DesignRevision, prefix string) bool {
	for _, r := range list {
		if r.Prefix == prefix {
			return true
		}
	}
	return false
}

// sortDesignRevisions puts the list in revision order, oldest first.
//
// An insertion sort because the list is at most MaxDesignRevisions long and
// is already sorted every time but the one where an entry was replaced.
func sortDesignRevisions(list []DesignRevision) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j-1].N > list[j].N; j-- {
			list[j-1], list[j] = list[j], list[j-1]
		}
	}
}

// toInt reads a Firestore number, which arrives as int64 from the wire and
// as int from a local write in a test.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int64:
		return int(n), true
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}
