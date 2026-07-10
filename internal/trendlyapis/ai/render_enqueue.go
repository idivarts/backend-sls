package ai

import (
	"encoding/json"
	"log"
	"os"

	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

// RenderJob is the message contract for the async scene render worker (see
// backend-sls/docs/render-worker.md). The worker loads the scene revision,
// rasterizes it (image → PNG, video → MP4 via CanvasKit + FFmpeg, muxing any
// generated audio), stores the output, and writes renderUrl back onto the
// content's sceneRef.
type RenderJob struct {
	BrandID    string `json:"brandId"`
	ContentID  string `json:"contentId"`
	RevisionID string `json:"revisionId"`
	DocType    string `json:"docType"` // image|video
}

// enqueueRender best-effort queues a render. If RENDER_QUEUE_URL is unset (local
// dev, or before the worker is provisioned) it is a no-op — the frontend still
// renders the scene for preview, and the final raster is produced later. Never
// fails the AI turn.
func enqueueRender(brandID, contentID, revisionID, docType string) {
	queueURL := os.Getenv("RENDER_QUEUE_URL")
	if queueURL == "" {
		return
	}
	body, err := json.Marshal(RenderJob{
		BrandID: brandID, ContentID: contentID, RevisionID: revisionID, DocType: docType,
	})
	if err != nil {
		return
	}
	if err := sqshandler.SendToQueue(queueURL, string(body), 0); err != nil {
		log.Printf("enqueueRender: %v", err)
	}
}
