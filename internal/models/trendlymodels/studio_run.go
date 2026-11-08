package trendlymodels

// A director run — brands/{brandId}/contents/{contentId}/studios/{studioId}/runs/{runId}.
//
// ── What a run is ───────────────────────────────────────────────────────────
// One plan, its approval, and the work that followed from it. It exists as a
// document rather than as a request/response for three reasons, each of which
// is a thing people actually do:
//
//   - APPROVE LATER. A plan is read, thought about, and approved — possibly on
//     another device. A plan that lived only in a response would have to be
//     re-planned, which costs money and produces a different plan.
//   - SEE WHAT IT COST. A run that spent real money is something the user is
//     entitled to look back at, including when it half failed.
//   - NOT RUN TWICE. The approval is recorded on the document, so two taps on
//     a slow button cannot both start the work.
//
// ⚠️ THE PLAN IS STORED AS OPAQUE JSON. It is the planner's output, versioned
// by whatever the director package understood on the day — not a model anything
// queries. Giving it Firestore fields would freeze a shape that is still
// changing, and an old run would then read as corrupt rather than as old.

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Run statuses.
const (
	// RunPlanning has been asked for but the planner has not answered yet.
	//
	// The row exists from the moment the request is accepted, so a plan is
	// never invisible while it is being made: the storyboard can show "working
	// on it" against a real id, and a reload finds it again. A director plan
	// does not fit in an HTTP request (see internal/studiojobs), so this is the
	// state a run is created in.
	RunPlanning = "planning"
	// RunPlanned has a plan and is waiting for a person.
	RunPlanned = "planned"
	// RunApproved has been accepted and the work has started.
	RunApproved = "approved"
	RunRunning  = "running"
	RunDone     = "done"
	RunFailed   = "failed"
	// RunRefused never got a usable plan: the planner could not produce one
	// without a fatal problem. Kept rather than deleted, because the problems
	// are the useful part.
	RunRefused = "refused"
	// RunCancelled was stopped by the user partway.
	RunCancelled = "cancelled"
)

// StudioRun is one plan and what came of it.
type StudioRun struct {
	ID string `json:"id" firestore:"-"`

	// Status: planned | approved | running | done | failed | refused | cancelled.
	Status string `json:"status" firestore:"status"`
	// Mode is the user's intent — economy | balanced | cinematic. They pick an
	// intent, never a model.
	Mode string `json:"mode" firestore:"mode"`
	// Brief is what the user asked for, in their words. Kept so a run is
	// legible months later without reading the plan.
	Brief string `json:"brief" firestore:"brief"`

	// PlanJSON is the planner's output, verbatim. See the note above.
	PlanJSON string `json:"planJson,omitempty" firestore:"planJson,omitempty"`
	// Storyboard is the human-readable rendering of the plan, as it was shown
	// for approval. Stored because it is the thing the user actually agreed
	// to — re-rendering it later from a newer printer could show something
	// subtly different from what they approved.
	Storyboard string `json:"storyboard,omitempty" firestore:"storyboard,omitempty"`

	// Problems are the validator's findings. A fatal one means the plan was
	// never runnable; warnings were shown and could be overridden.
	Problems []RunProblem `json:"problems,omitempty" firestore:"problems,omitempty"`

	// SceneIDs are the scenes this run created, in plan order, so a cancelled
	// or failed run can be undone without guessing which scenes were its.
	SceneIDs []string `json:"sceneIds,omitempty" firestore:"sceneIds,omitempty"`

	// EstCostUSD is what the registry priced the plan at — OUR number, not the
	// planner's claim about itself.
	EstCostUSD float64 `json:"estCostUsd,omitempty" firestore:"estCostUsd,omitempty"`
	// ChargedTokens is what the wallet actually paid across the run.
	ChargedTokens int64 `json:"chargedTokens,omitempty" firestore:"chargedTokens,omitempty"`

	Model     string `json:"model,omitempty" firestore:"model,omitempty"`
	Error     string `json:"error,omitempty" firestore:"error,omitempty"`
	CreatedBy string `json:"createdBy,omitempty" firestore:"createdBy,omitempty"`

	CreatedAt  int64 `json:"createdAt" firestore:"createdAt"`
	ApprovedAt int64 `json:"approvedAt,omitempty" firestore:"approvedAt,omitempty"`
	UpdatedAt  int64 `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`
}

// RunProblem is one validator finding, flattened for storage.
type RunProblem struct {
	Field   string `json:"field" firestore:"field"`
	Message string `json:"message" firestore:"message"`
	Fatal   bool   `json:"fatal" firestore:"fatal"`
}

func runsCollection(brandID, contentID, studioID string) *firestore.CollectionRef {
	return studioDoc(brandID, contentID, studioID).Collection("runs")
}

// CreateStudioRun stores a plan awaiting approval.
func CreateStudioRun(ctx context.Context, brandID, contentID, studioID string, r *StudioRun) (string, error) {
	now := time.Now().UnixMilli()
	if r.CreatedAt == 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	if r.Status == "" {
		r.Status = RunPlanned
	}
	ref, _, err := runsCollection(brandID, contentID, studioID).Add(ctx, r)
	if err != nil {
		return "", fmt.Errorf("CreateStudioRun: %w", err)
	}
	return ref.ID, nil
}

// GetStudioRun reads one run, or (nil, nil) when absent.
func GetStudioRun(ctx context.Context, brandID, contentID, studioID, runID string) (*StudioRun, error) {
	doc, err := runsCollection(brandID, contentID, studioID).Doc(runID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var r StudioRun
	if err := doc.DataTo(&r); err != nil {
		return nil, err
	}
	r.ID = doc.Ref.ID
	return &r, nil
}

// ApproveStudioRun moves a planned run to approved, ONCE.
//
// Guarded in a transaction, which is the whole reason this is not an Update: a
// run that starts twice generates everything twice and charges for both. Two
// taps on a slow button is the ordinary way that happens.
//
// Returns false when the run was already approved — which is a no-op, not an
// error, because the second tap should simply do nothing.
func ApproveStudioRun(ctx context.Context, brandID, contentID, studioID, runID string) (bool, error) {
	ref := runsCollection(brandID, contentID, studioID).Doc(runID)
	approved := false

	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var r StudioRun
		if err := snap.DataTo(&r); err != nil {
			return err
		}
		if r.Status != RunPlanned {
			approved = false
			return nil
		}
		approved = true
		return tx.Update(ref, []firestore.Update{
			{Path: "status", Value: RunApproved},
			{Path: "approvedAt", Value: time.Now().UnixMilli()},
			{Path: "updatedAt", Value: time.Now().UnixMilli()},
		})
	})
	if err != nil {
		return false, err
	}
	return approved, nil
}

// UpdateStudioRun applies partial updates and stamps updatedAt.
func UpdateStudioRun(ctx context.Context, brandID, contentID, studioID, runID string,
	updates []firestore.Update) error {
	updates = append(updates, firestore.Update{Path: "updatedAt", Value: time.Now().UnixMilli()})
	_, err := runsCollection(brandID, contentID, studioID).Doc(runID).Update(ctx, updates)
	return err
}

// ── Closing a run ───────────────────────────────────────────────────────────
//
// A run goes `running` the moment its scenes are materialised, and the work
// that follows happens one scene at a time, in separate requests, driven by
// whoever is holding the studio open. So nothing on the server knows the run is
// over except the scenes themselves — which is why the close is computed from
// them rather than announced by a caller.
//
// ⚠️ THIS IS WHY A RUN USED TO STAY `running` FOREVER. The materialise wrote
// seven pending scenes, marked the run running, and handed off; the client
// never took the hand-off, and nothing ever looked at the run again. A run that
// is permanently `running` is indistinguishable from one that is working, which
// is the state that hid the bug for three hours. Settling here means the run
// closes even when the client dies after the last scene lands.

// runOutcome decides what a run's status should become, given every scene in
// the studio.
//
// Pure, and separately tested, because it is the whole of the decision: the
// Firestore around it is bookkeeping.
//
//   - settle is false while ANY scene of the run is still pending, and for a
//     run that owns no scenes at all (a run whose scenes were deleted has
//     nothing to report, and closing it would be inventing an outcome).
//   - failed only when EVERY scene failed. A run of seven where one scene did
//     not come back is a run that produced six scenes, and calling it `failed`
//     would tell the user their video is gone when it is on the rail in front
//     of them. The individual failure is already visible on the scene.
func runOutcome(scenes []StudioScene, runID string) (settle bool, status string) {
	mine, failed := 0, 0
	for _, sc := range scenes {
		if sc.RunID != runID {
			continue
		}
		if sc.Pending {
			return false, ""
		}
		mine++
		if sc.Gen != nil && sc.Gen.Status == "failed" {
			failed++
		}
	}
	if mine == 0 {
		return false, ""
	}
	if failed == mine {
		return true, RunFailed
	}
	return true, RunDone
}

// SettleStudioRunIfComplete closes a run once none of its scenes are pending.
//
// Safe to call after every scene that stops being pending, including the ones
// that stopped by failing: it is a no-op unless this was the last one. Errors
// are the caller's to ignore — a run left open is a cosmetic problem, and
// failing a scene's own request over it would be worse.
func SettleStudioRunIfComplete(ctx context.Context, brandID, contentID, studioID, runID string) error {
	if runID == "" {
		return nil
	}
	run, err := GetStudioRun(ctx, brandID, contentID, studioID, runID)
	if err != nil || run == nil {
		return err
	}
	// Only a live run closes. A cancelled or already-settled run must not be
	// reopened by a straggler scene finishing after the fact.
	if run.Status != RunApproved && run.Status != RunRunning {
		return nil
	}
	scenes, err := ListStudioScenes(ctx, brandID, contentID, studioID)
	if err != nil {
		return err
	}
	settle, status := runOutcome(scenes, runID)
	if !settle {
		return nil
	}
	return UpdateStudioRun(ctx, brandID, contentID, studioID, runID, []firestore.Update{
		{Path: "status", Value: status},
	})
}

// ListStudioRuns returns a studio's runs, newest first.
func ListStudioRuns(ctx context.Context, brandID, contentID, studioID string, limit int) ([]StudioRun, error) {
	if limit <= 0 {
		limit = 20
	}
	iter := runsCollection(brandID, contentID, studioID).
		OrderBy("createdAt", firestore.Desc).Limit(limit).Documents(ctx)
	defer iter.Stop()

	out := []StudioRun{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, err
		}
		var r StudioRun
		if err := doc.DataTo(&r); err != nil {
			continue
		}
		r.ID = doc.Ref.ID
		out = append(out, r)
	}
	return out, nil
}

// DeleteStudioRuns removes every run under a studio.
//
// ⚠️ Firestore does NOT delete a document's subcollections when the document
// goes. A deleted studio's runs would survive it: invisible to anything that
// walks from the studio, still billable, and still holding the full planJson of
// every plan ever made for content the user believes is gone. That last part
// matters — a plan carries the brief and the script.
//
// Batched because a studio that has been planned a dozen times should not need
// a dozen round trips, and because this runs inside a delete the user is
// waiting on.
func DeleteStudioRuns(ctx context.Context, brandID, contentID, studioID string) error {
	for {
		iter := runsCollection(brandID, contentID, studioID).Limit(200).Documents(ctx)
		batch := firestoredb.Client.BulkWriter(ctx)
		n := 0
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				iter.Stop()
				return err
			}
			if _, err := batch.Delete(doc.Ref); err != nil {
				iter.Stop()
				return err
			}
			n++
		}
		iter.Stop()
		batch.End()
		// Fewer than the page size means that was the last page. Looping on a
		// full page rather than once, so a studio with hundreds of runs is
		// fully cleared instead of mostly cleared.
		if n < 200 {
			return nil
		}
	}
}
