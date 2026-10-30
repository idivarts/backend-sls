package mediagen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/pkg/elevenlabs"
)

// elevenlabsDriver generates video.
//
// The first ASYNCHRONOUS driver, and the reason Submit and Get were two methods
// from the start rather than one. A clip takes minutes, so Submit returns a job
// id and StatusPending, and the result is collected later — by webhook if the
// workspace has one configured, by the polling cron otherwise.
//
// ⚠️ A completed generation's URL is SIGNED and expires about an hour out, so
// the caller must copy the bytes to our own storage. That is stated on Asset
// and enforced by Hotlink being false here: a composition that resolves to an
// expired URL is one nobody can re-render, and designs are re-rendered weeks
// later as a matter of course.
type elevenlabsDriver struct{}

func init() { Register(elevenlabsDriver{}) }

// audioFieldFor decides whether this request says anything about sound.
//
// Stated explicitly rather than left to the API's default, which is ON — but
// only to a model that takes the field. One that does not rejects it outright
// ("Extra inputs are not permitted"), so an explicit "no sound" would fail
// every generation it serves rather than producing a silent clip.
func audioFieldFor(s Spec) *bool {
	if !s.NativeAudio {
		return nil
	}
	v := s.GenerateAudio
	return &v
}

func (elevenlabsDriver) Name() string { return "elevenlabs" }

func (elevenlabsDriver) Supports(k Kind) bool { return k == KindVideo }

func (d elevenlabsDriver) Submit(ctx context.Context, s Spec) (Result, error) {
	if s.Kind != KindVideo {
		return Result{}, fmt.Errorf("%w: elevenlabs serves video only", ErrUnsupported)
	}
	if strings.TrimSpace(s.ModelID) == "" {
		return Result{}, fmt.Errorf("%w: a video needs a model id from the registry", ErrUnsupported)
	}
	// Rejected rather than ignored, like every other capability mismatch in
	// this package: the API forbids the combination, and a caller who pinned a
	// last frame and got a clip ignoring it has paid for a misunderstanding.
	if (s.StartFrame != nil || s.EndFrame != nil) && len(s.ImageRefs) > 0 {
		return Result{}, fmt.Errorf(
			"%w: a start or end frame cannot be combined with reference images", ErrUnsupported)
	}
	if s.SourceVideo != nil {
		return Result{}, fmt.Errorf(
			"%w: editing an existing video is not wired up yet", ErrUnsupported)
	}

	req := elevenlabs.VideoRequest{
		ModelID:      s.ModelID,
		Prompt:       s.Prompt,
		AspectRatio:  s.AspectRatio,
		DurationSecs: s.DurationSecs,
		Resolution:   s.Resolution,
		Webhook:      elevenlabs.WebhookAll(),
	}
	req.GenerateAudio = audioFieldFor(s)

	var err error
	if req.StartFrame, err = inlineFrame(ctx, s.StartFrame); err != nil {
		return Result{Status: StatusFailed, Error: "could not read the start frame"}, err
	}
	if req.EndFrame, err = inlineFrame(ctx, s.EndFrame); err != nil {
		return Result{Status: StatusFailed, Error: "could not read the end frame"}, err
	}
	for _, r := range s.ImageRefs {
		ref, ierr := inlineFrame(ctx, &r)
		if ierr != nil {
			return Result{Status: StatusFailed, Error: "could not read a reference image"}, ierr
		}
		if ref != nil {
			req.Images = append(req.Images, *ref)
		}
	}

	gen, err := elevenlabs.CreateVideo(req)
	if err != nil {
		return Result{Status: StatusFailed, Error: err.Error()}, err
	}
	return Result{JobID: gen.ID, Status: StatusPending}, nil
}

// Get polls one generation. Also how an expired content URL is refreshed: a
// completed generation is re-signed every time it is fetched.
func (elevenlabsDriver) Get(ctx context.Context, jobID string) (Result, error) {
	gen, err := elevenlabs.GetVideo(jobID)
	if err != nil {
		return Result{}, err
	}
	return resultFromGeneration(gen), nil
}

// ParseWebhook turns a flows callback body into a Result.
//
// The payload is documented as matching the terminal GET response, so the two
// decode through one function — which is what stops a webhook and a poll from
// ever disagreeing about what "done" looks like.
func (elevenlabsDriver) ParseWebhook(body []byte) (Result, error) {
	gen, err := elevenlabs.ParseVideoWebhook(body)
	if err != nil {
		return Result{}, err
	}
	return resultFromGeneration(gen), nil
}

func resultFromGeneration(gen *elevenlabs.VideoGeneration) Result {
	out := Result{JobID: gen.ID}
	switch {
	case gen.Running():
		out.Status = StatusPending
	case gen.Status == elevenlabs.VideoCompleted:
		out.Status = StatusSucceeded
		out.Assets = []Asset{{
			URL:      gen.ContentURL,
			MimeType: orDefault(gen.ContentMimeType, "video/mp4"),
			// NOT hotlinkable: this URL is signed and dies within the hour.
			Hotlink: false,
		}}
	default:
		out.Status = StatusFailed
		out.Error = userFacingFailure(gen)
	}
	return out
}

// userFacingFailure turns a failure_reason into a sentence worth showing.
//
// The enum is the useful part — "moderated" and "invalid_parameters" are things
// a user can act on, while the rest are ours to deal with — so each one that
// has a real next step gets its own line rather than a generic apology.
func userFacingFailure(gen *elevenlabs.VideoGeneration) string {
	switch gen.FailureReason {
	case "moderated":
		return "The model declined this prompt. Try describing the shot differently."
	case "invalid_parameters":
		return "That combination of settings isn't supported by this model. Try a different length or size."
	case "timeout":
		return "The clip took too long to generate. Try a shorter one."
	case "charging_failed":
		return "Video generation is temporarily unavailable for this workspace. We've been notified."
	}
	if gen.ErrorMessage != "" {
		return "The clip couldn't be generated: " + gen.ErrorMessage
	}
	return "The clip couldn't be generated. Please try again."
}

// inlineFrame fetches a frame we host and hands it over as bytes.
//
// The API takes its own asset ids, its own generation ids, or raw bytes — an
// arbitrary URL is none of those, and every frame this product has is a URL in
// our own bucket. So the bytes make the trip.
func inlineFrame(ctx context.Context, ref *AssetRef) (*elevenlabs.ImageRef, error) {
	if ref == nil || strings.TrimSpace(ref.URL) == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mediagen: frame fetch returned %d", resp.StatusCode)
	}
	// Capped well under the API's 25MB decoded limit. A frame is a still at
	// post dimensions; anything near this is not a frame.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}

	mime, ok := frameMimeType(data, resp.Header.Get("content-type"))
	if !ok {
		// Refused here rather than sent and rejected: the API answers an
		// unknown mime type with a 422 naming a field path, which reaches the
		// user as "couldn't start this clip" and reaches nobody as a reason.
		return nil, fmt.Errorf(
			"mediagen: that frame isn't an image the model accepts (JPEG, PNG, WebP, HEIC or HEIF)")
	}
	out := elevenlabs.InlineImage(data, mime)
	return &out, nil
}

// frameMimeType decides what a frame's bytes ARE, not what the server said.
//
// The header is checked second, and only as a tie-break, because it is the part
// that was wrong: a file uploaded to S3 without an explicit content type comes
// back as `binary/octet-stream`, and forwarding that got every pinned start
// frame refused with a 422 on `start_frame.inline_base64.mime_type`. The bytes
// cannot be wrong in the same way — a PNG begins with a PNG header whatever the
// bucket claims.
func frameMimeType(data []byte, header string) (string, bool) {
	allowed := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
		"image/heic": true,
		"image/heif": true,
	}

	sniffed := http.DetectContentType(data)
	if i := strings.Index(sniffed, ";"); i > 0 {
		sniffed = strings.TrimSpace(sniffed[:i])
	}
	if allowed[sniffed] {
		return sniffed, true
	}

	// DetectContentType knows nothing about HEIC/HEIF and reports WebP only on
	// newer Go, so a header the API would have accepted is still worth taking.
	h := strings.ToLower(strings.TrimSpace(header))
	if i := strings.Index(h, ";"); i > 0 {
		h = strings.TrimSpace(h[:i])
	}
	if allowed[h] {
		return h, true
	}
	return "", false
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
