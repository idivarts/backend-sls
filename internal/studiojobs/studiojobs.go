// Package studiojobs is the async pipe for Design Studio work that cannot fit
// in an HTTP request.
//
// ── Why this exists ─────────────────────────────────────────────────────────
//
// API Gateway's integration timeout is 29 seconds and is NOT configurable. A
// director plan is one or two calls to a frontier reasoning model, each
// producing a whole storyboard as JSON, and it reliably takes longer: measured
// against the deployed dev stack it returned 504 at 30s on both a cold and a
// warm call, with no run stored, because the write happens after the planner
// answers.
//
// Raising the Lambda timeout cannot fix that — the gateway gives up first. So
// the request has to stop waiting. The pieces for that were already in place:
// a run is STORED rather than returned and forgotten, precisely so a person can
// read a storyboard and approve it later, and GET …/runs/:runId already exists
// to read one back.
//
// One queue with a discriminated `type`, not a queue per operation — the same
// shape as internal/socialsync, for the same reason: a new kind of studio job
// should be a new JobType and a worker case, not new infrastructure.
package studiojobs

import (
	"encoding/json"
	"os"

	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

// QueueEnv holds the studio job queue URL. Set on trendly_studio_apis in
// serverless.trendly.yml.
//
// ⚠️ Env vars there are PER FUNCTION. A missing URL here does not fail a
// deploy and does not raise an error at runtime — Enqueue returns
// queued=false — so the symptom would be a plan that sits in "planning"
// forever. JobQueued() exists so the caller can refuse up front instead.
const QueueEnv = "STUDIO_JOB_QUEUE_URL"

// JobType discriminates what the worker runs.
type JobType = string

const (
	// JobDirectorPlan asks the director for a plan and stores it on the run.
	JobDirectorPlan JobType = "director_plan"
)

// Message is the payload shape. BrandID, ContentID and StudioID are always
// required; RunID identifies the row the worker writes its answer to.
type Message struct {
	Type      JobType `json:"type"`
	BrandID   string  `json:"brandId"`
	ContentID string  `json:"contentId"`
	StudioID  string  `json:"studioId"`
	RunID     string  `json:"runId"`
}

// Configured reports whether a queue is available.
//
// Checked BEFORE the run row is written. Without it a missing env var would
// leave a row stuck in "planning" and a user watching a spinner that never
// resolves — the worst failure shape available, because it looks like slowness
// rather than breakage.
func Configured() bool { return os.Getenv(QueueEnv) != "" }

// Enqueue sends a job to the studio queue.
//
// Unlike socialsync.Enqueue there is no inline fallback: the whole point is
// that this work does not fit in the caller's request.
func Enqueue(msg Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return sqshandler.SendToQueue(os.Getenv(QueueEnv), string(body), 0)
}
