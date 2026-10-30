package trendlymodels

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

// ── One in-flight generative media job ──────────────────────────────────────
//
// A generated clip takes minutes, which is long enough that nothing can hold
// the request open and wait. So the job gets a document: the timeline can show
// the clip as queued, a webhook or the polling cron can find which clip to
// resolve when it lands, and a refresh of the page does not lose the work.
//
// Stored top-level at mediaJobs/{jobId} rather than under the content, because
// the two things that look a job up — a provider callback and a sweep for
// stale ones — both arrive knowing only the provider's own id. A subcollection
// would make the callback hunt for the parent it belongs to.
//
// The provider's id IS the document id. A callback that arrives twice, or a
// poll that races a webhook, then writes the same document twice rather than
// creating a second job for one generation.
type MediaJob struct {
	ID string `json:"id" firestore:"-"`

	// Provider is the mediagen driver that owns the job ("elevenlabs").
	Provider string `json:"provider" firestore:"provider"`
	// Kind mirrors mediagen.Kind — video for now.
	Kind    string `json:"kind" firestore:"kind"`
	ModelID string `json:"modelId" firestore:"modelId"`
	Prompt  string `json:"prompt,omitempty" firestore:"prompt,omitempty"`

	// Where the result belongs. A job exists to fill a specific clip on a
	// specific composition; without all four it has nowhere to land.
	BrandID       string `json:"brandId" firestore:"brandId"`
	ContentID     string `json:"contentId" firestore:"contentId"`
	CompositionID string `json:"compositionId" firestore:"compositionId"`
	ClipID        string `json:"clipId" firestore:"clipId"`

	// Status mirrors mediagen.Status: pending | succeeded | failed.
	Status string `json:"status" firestore:"status"`
	// URL is the FINAL, re-hosted url — never the provider's signed one, which
	// expires within the hour.
	URL   string `json:"url,omitempty" firestore:"url,omitempty"`
	Error string `json:"error,omitempty" firestore:"error,omitempty"`

	// ChargedTokens is what the wallet paid when the job was submitted. Kept so
	// a failure can be refunded exactly once, and so a retry knows.
	ChargedTokens int64 `json:"chargedTokens,omitempty" firestore:"chargedTokens,omitempty"`
	// Refunded guards that refund against a webhook and a poll both firing.
	Refunded bool `json:"refunded,omitempty" firestore:"refunded,omitempty"`

	OrganizationID string `json:"organizationId,omitempty" firestore:"organizationId,omitempty"`
	CreatedBy      string `json:"createdBy,omitempty" firestore:"createdBy,omitempty"`
	SubmittedAt    int64  `json:"submittedAt" firestore:"submittedAt"`
	ResolvedAt     int64  `json:"resolvedAt,omitempty" firestore:"resolvedAt,omitempty"`
}

// Media job statuses, mirroring mediagen's.
const (
	MediaJobPending   = "pending"
	MediaJobSucceeded = "succeeded"
	MediaJobFailed    = "failed"
)

const mediaJobsCollection = "mediaJobs"

func mediaJobs() *firestore.CollectionRef {
	return firestoredb.Client.Collection(mediaJobsCollection)
}

// CreateMediaJob records a submitted generation, keyed by the PROVIDER's id.
//
// Set rather than Add: the provider's id is the document id, so a duplicate
// submission of the same generation overwrites one document instead of
// creating a second job that nothing will ever resolve.
func CreateMediaJob(ctx context.Context, job *MediaJob) error {
	if job.ID == "" {
		return fmt.Errorf("CreateMediaJob: a job needs the provider's id")
	}
	if job.SubmittedAt == 0 {
		job.SubmittedAt = time.Now().UnixMilli()
	}
	if job.Status == "" {
		job.Status = MediaJobPending
	}
	_, err := mediaJobs().Doc(job.ID).Set(ctx, job)
	return err
}

// GetMediaJob reads one job, or (nil, nil) when absent.
//
// A callback for a job we have no record of is the normal case for a stage
// that shares a provider workspace with another: dev and prod both receive
// every webhook the workspace sends. Returning nil rather than an error is
// what lets the handler ignore those quietly.
func GetMediaJob(ctx context.Context, jobID string) (*MediaJob, error) {
	doc, err := mediaJobs().Doc(jobID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var j MediaJob
	if err := doc.DataTo(&j); err != nil {
		return nil, err
	}
	j.ID = doc.Ref.ID
	return &j, nil
}

// ResolveMediaJob records a terminal outcome.
//
// Returns false when the job had already reached one, which is how a webhook
// and a poll arriving for the same generation produce one resolution rather
// than two — and, more importantly, one refund rather than two.
func ResolveMediaJob(ctx context.Context, jobID, jobStatus, url, errMsg string) (bool, error) {
	ref := mediaJobs().Doc(jobID)
	changed := false
	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var j MediaJob
		if err := doc.DataTo(&j); err != nil {
			return err
		}
		if j.Status != MediaJobPending {
			changed = false
			return nil
		}
		changed = true
		return tx.Set(ref, map[string]interface{}{
			"status":     jobStatus,
			"url":        url,
			"error":      errMsg,
			"resolvedAt": time.Now().UnixMilli(),
		}, firestore.MergeAll)
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

// MarkMediaJobRefunded records that a failed job's tokens went back, so a
// second callback cannot refund them again.
func MarkMediaJobRefunded(ctx context.Context, jobID string) (bool, error) {
	ref := mediaJobs().Doc(jobID)
	refunded := false
	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var j MediaJob
		if err := doc.DataTo(&j); err != nil {
			return err
		}
		if j.Refunded {
			refunded = false
			return nil
		}
		refunded = true
		return tx.Set(ref, map[string]interface{}{"refunded": true}, firestore.MergeAll)
	})
	if err != nil {
		return false, err
	}
	return refunded, nil
}

// ListPendingMediaJobs finds jobs still in flight, oldest first.
//
// This is the polling path, and it is not a fallback nicety: a webhook that is
// misconfigured, blocked by a network, or simply never sent would otherwise
// leave every clip queued forever, with the first symptom being a timeline
// that never finishes. Needs the composite index on (status, submittedAt) that
// ships in both index files.
func ListPendingMediaJobs(ctx context.Context, limit int) ([]MediaJob, error) {
	q := mediaJobs().
		Where("status", "==", MediaJobPending).
		OrderBy("submittedAt", firestore.Asc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(ctx)
	defer iter.Stop()

	out := []MediaJob{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, err
		}
		var j MediaJob
		if err := doc.DataTo(&j); err != nil {
			continue
		}
		j.ID = doc.Ref.ID
		out = append(out, j)
	}
	return out, nil
}
