// Package designpreview renders a few small frames of an HTML design so the AI
// that authored it can look at what it made.
//
// This is the inspection lane, and it is deliberately the opposite of the
// publish lane in internal/trendlyapis/rendering: that one is fire-and-forget
// (SQS for images, Batch for video) because nobody is waiting on the answer.
// Here somebody is — a chat turn is blocked on it — so the worker Lambda is
// invoked SYNCHRONOUSLY and hands the frames straight back.
//
// It is a separate Lambda function from the queue consumer, running the same
// container: an inspection should never sit behind a queue of publish renders,
// but it should inherit their warm Chromium.
package designpreview

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
)

// ErrNotConfigured means no preview function is wired up for this stage. The
// caller should degrade (skip the look) rather than fail the whole turn.
var ErrNotConfigured = errors.New("design preview is not configured")

// Frame is one still to capture: a scene index, and how far into that scene.
// A negative Ms is a fraction of that scene's own duration (-0.15 = 15% in),
// which is how the caller asks for "just after the entrance" without knowing
// the scene's timing. Keep in sync with the worker's preview.ts.
type Frame struct {
	Slide int     `json:"slide"`
	Ms    float64 `json:"ms"`
}

type previewEvent struct {
	Kind       string  `json:"kind"`
	BrandID    string  `json:"brandId"`
	ContentID  string  `json:"contentId"`
	RevisionID string  `json:"revisionId"`
	Frames     []Frame `json:"frames,omitempty"`
	MaxEdge    int     `json:"maxEdge,omitempty"`
}

// Result mirrors the worker's PreviewResponse.
type Result struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	DocType    string `json:"docType,omitempty"`
	SlideCount int    `json:"slideCount,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	// Images are data:image/jpeg;base64,… URIs, one per captured frame.
	Images   []string `json:"images,omitempty"`
	Captured []Frame  `json:"captured,omitempty"`
}

var svc *lambda.Lambda

func init() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	// A preview renders a browser page, so it is slow by SDK standards and the
	// default 1-minute client timeout would cut it off mid-screenshot. The
	// worker function has its own timeout; this one only has to outlast it.
	svc = lambda.New(sess, &aws.Config{
		HTTPClient: &http.Client{Timeout: 150 * time.Second},
	})
}

// FunctionName is the preview Lambda, from the environment. Empty means the
// stage has no preview function deployed.
func FunctionName() string {
	return os.Getenv("DESIGN_PREVIEW_FUNCTION")
}

// Render captures frames of a design revision and returns them as data URIs.
//
// frames may be nil, in which case the worker picks a sensible sample: every
// slide for a still design, and three points inside each scene for a video.
func Render(brandID, contentID, revisionID string, frames []Frame, maxEdge int) (*Result, error) {
	fn := FunctionName()
	if fn == "" {
		return nil, ErrNotConfigured
	}

	payload, err := json.Marshal(previewEvent{
		Kind:       "preview",
		BrandID:    brandID,
		ContentID:  contentID,
		RevisionID: revisionID,
		Frames:     frames,
		MaxEdge:    maxEdge,
	})
	if err != nil {
		return nil, err
	}

	out, err := svc.Invoke(&lambda.InvokeInput{
		FunctionName:   aws.String(fn),
		InvocationType: aws.String(lambda.InvocationTypeRequestResponse),
		Payload:        payload,
	})
	if err != nil {
		return nil, err
	}
	// A handler that threw comes back 200 with FunctionError set, so the status
	// code alone is not enough to tell success from failure.
	if out.FunctionError != nil && *out.FunctionError != "" {
		return nil, fmt.Errorf("preview worker: %s", string(out.Payload))
	}

	var res Result
	if err := json.Unmarshal(out.Payload, &res); err != nil {
		return nil, fmt.Errorf("preview worker returned unreadable payload: %w", err)
	}
	if !res.OK {
		return nil, fmt.Errorf("preview failed: %s", res.Error)
	}
	return &res, nil
}
