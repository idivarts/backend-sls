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

	// JobSceneDesign writes ONE scene's HTML.
	//
	// It moved here for exactly the reason the package exists. A design is a
	// frontier model emitting a whole HTML document, and on dev it reliably
	// took 25–40s against a Lambda capped at 30 — three of five scenes in a
	// real run died at precisely 29.999s. Worse, the request context carries
	// that deadline, so the write that records the failure used an expired
	// context and was dropped: the scenes could not even say they had failed,
	// and sat at "Making…" indefinitely.
	//
	// No synchronous HTTP path can hold this work. The queue's worker has 900
	// seconds, which is an order of magnitude more than a design needs.
	JobSceneDesign JobType = "scene_design"

	// JobDesignAuthor writes a whole Design Studio v3 project: index.html and
	// one file per scene, as a Hyperframes project.
	//
	// Here for the same measured reason as the two above, only more so. A
	// scene design is one HTML document and took 25-40s; a v3 project is
	// several, and the first attempt against the 30s HTTP route returned 504
	// with nothing written. The queue's worker has 900 seconds.
	JobDesignAuthor JobType = "design_author"
)

// Message is the payload shape. BrandID, ContentID and StudioID are always
// required; the rest identifies the row the worker writes its answer to, and
// which one that is depends on the type — RunID for a plan, SceneID for a
// scene design. `Target` reports whether a message has the one it needs, so a
// malformed job is dropped rather than retried against nothing.
type Message struct {
	Type      JobType `json:"type"`
	BrandID   string  `json:"brandId"`
	ContentID string  `json:"contentId"`
	StudioID  string  `json:"studioId"`
	RunID     string  `json:"runId,omitempty"`
	SceneID   string  `json:"sceneId,omitempty"`

	// DesignID targets a Design Studio v3 design at
	// brands/{brandId}/designs/{designId}.
	//
	// ⚠️ A v3 design has NO contentId and NO studioId — that is the whole
	// point of v3 — so Target() cannot require them for this type. See the
	// note there.
	DesignID string `json:"designId,omitempty"`
	// Brief is what the user asked for, in their words.
	Brief string `json:"brief,omitempty"`
}

// Target reports whether the message names everything its type needs.
//
// ⚠️ Per type, deliberately. The first version required RunID for every
// message, which would have silently dropped every scene design: a regenerate
// of a hand-built scene has no run at all.
func (m Message) Target() bool {
	if m.BrandID == "" {
		return false
	}
	// ⚠️ The content/studio pair is checked PER TYPE, not up front.
	//
	// It used to be required for every message, which was already once the
	// cause of this comment: the first version required RunID for all of
	// them and would have dropped every scene design. A v3 design author job
	// has neither a contentId nor a studioId — a v3 design is a first-class
	// object — so hoisting that check back out would silently drop every one
	// of them, with the job vanishing into the queue and the design sitting
	// at "writing" for ever.
	switch m.Type {
	case JobDirectorPlan:
		return m.ContentID != "" && m.StudioID != "" && m.RunID != ""
	case JobSceneDesign:
		return m.ContentID != "" && m.StudioID != "" && m.SceneID != ""
	case JobDesignAuthor:
		return m.DesignID != ""
	default:
		return false
	}
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
