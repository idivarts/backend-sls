package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/studiojobs"
	"github.com/idivarts/backend-sls/internal/trendlyapis/studio"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// handler is the worker for Design Studio work that cannot fit in an HTTP
// request.
//
// Two jobs: the director's plan, and a scene's design. API Gateway's integration
// timeout is 29 seconds and is not configurable, while a plan is one or two
// calls to a frontier reasoning model emitting a whole storyboard as JSON — it
// returned 504 at 30s against the deployed dev stack, cold and warm, with
// nothing stored. The HTTP endpoint now creates the run in `planning` state and
// hands it here; the client polls GET …/runs/:runId.
//
// A scene design is here for the same reason as the plan, measured the same
// way: 25–40s of frontier model against a 30s HTTP ceiling, with three of five
// scenes in a real run dying at exactly 29.999s and unable to record that they
// had — the request context carries the Lambda deadline, so the write that
// reports a failure expires with it.
//
// ⚠️ Errors are swallowed per record rather than returned. Returning an error
// makes SQS replay the WHOLE batch, and a replayed plan is a second call to a
// frontier model — real money for a job whose own failure has already been
// written onto the run row where the user can see it and retry deliberately.
// ExecutePlanJob always records an outcome, so a swallowed error is never a
// silent one.
func handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, record := range sqsEvent.Records {
		var msg studiojobs.Message
		if err := json.Unmarshal([]byte(record.Body), &msg); err != nil {
			log.Println("studio_sqs: bad message:", err, record.Body)
			mysentry.Capture(err, map[string]string{"lambda": "studio_sqs", "op": "unmarshal"})
			continue
		}
		// Per type — a scene design names a scene, not a run, and the old
		// blanket "RunID must be set" would have dropped every one of them.
		if !msg.Target() {
			log.Println("studio_sqs: incomplete message, skipping:", record.Body)
			continue
		}

		switch msg.Type {
		case studiojobs.JobDirectorPlan:
			if err := studio.ExecutePlanJob(ctx, msg); err != nil {
				log.Printf("studio_sqs: director plan failed for %s/%s/%s run %s: %v",
					msg.BrandID, msg.ContentID, msg.StudioID, msg.RunID, err)
				mysentry.Capture(err, map[string]string{
					"lambda":   "studio_sqs",
					"op":       "director_plan",
					"brandId":  msg.BrandID,
					"studioId": msg.StudioID,
				})
			}
		case studiojobs.JobSceneDesign:
			if err := studio.ExecuteSceneDesignJob(ctx, msg); err != nil {
				log.Printf("studio_sqs: scene design failed for %s/%s/%s scene %s: %v",
					msg.BrandID, msg.ContentID, msg.StudioID, msg.SceneID, err)
				mysentry.Capture(err, map[string]string{
					"lambda":   "studio_sqs",
					"op":       "scene_design",
					"brandId":  msg.BrandID,
					"studioId": msg.StudioID,
					"sceneId":  msg.SceneID,
				})
			}
		default:
			// Not a default-to-something: an unknown type is a deploy skew
			// (a newer producer, an older worker), and guessing which job was
			// meant would spend money on the wrong one.
			log.Println("studio_sqs: unknown job type, skipping:", msg.Type)
		}
	}
	return nil
}

func main() {
	// Non-Gin entry point, so it never passes through the Sentry middleware on
	// the shared Gin engine. Wrap captures errors and panics and — the part
	// that matters — flushes before Lambda freezes the environment.
	mysentry.Init()
	lambda.Start(mysentry.Wrap(handler))
}
