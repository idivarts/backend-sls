// Package footage finishes what the render worker starts on an uploaded video.
//
// Ingest is split across two processes for one reason: ffprobe and ffmpeg live
// in the render worker's container and nowhere else, while the transcript and
// the shot list are provider calls that belong with every other provider call.
// So the worker measures the video and samples frames, and this package turns
// those into words and descriptions.
//
// The split is visible in the asset: the worker writes ingest.status
// "pending", and only this package writes "done". Nothing downstream should
// treat a half-ingested video as fully described, and that is why it is not
// marked done the moment the frames exist.
package footage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai"
	"github.com/idivarts/backend-sls/pkg/elevenlabs"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// FinishPending completes every ingest the worker has handed over.
//
// Driven by the same cron that polls media jobs rather than by a callback: the
// worker could call back, but that would be a second notification path to keep
// correct for work that is never urgent. Nobody is watching a progress bar
// here — the footage was usable the moment it uploaded.
func FinishPending(ctx context.Context, limit int) error {
	pending, err := trendlymodels.ListPendingIngests(ctx, limit)
	if err != nil {
		return err
	}
	for i := range pending {
		a := pending[i]
		if err := Finish(ctx, a.BrandID, &a.Asset); err != nil {
			log.Printf("footage: finishing ingest for %s: %v", a.Asset.ID, err)
		}
	}
	return nil
}

// Finish runs the transcript and the shot list for one asset.
//
// Each half is attempted independently. A video with no speech has no
// transcript and that is not a failure; a vision pass that refuses still
// leaves usable timings behind. Marking the whole ingest failed because one
// half did would throw away the other.
func Finish(ctx context.Context, brandID string, asset *trendlymodels.Asset) error {
	if asset.Ingest == nil {
		return fmt.Errorf("footage: asset %s has no ingest to finish", asset.ID)
	}
	out := *asset.Ingest

	if words, text, err := transcribe(asset.URL); err != nil {
		log.Printf("footage: transcript for %s: %v", asset.ID, err)
	} else {
		out.Words, out.Text = words, text
	}

	if shots, err := describeShots(ctx, brandID, &out); err != nil {
		log.Printf("footage: shot list for %s: %v", asset.ID, err)
	} else {
		out.Shots = shots
	}

	out.Status = trendlymodels.IngestDone
	out.Error = ""
	return trendlymodels.SetAssetIngest(ctx, brandID, asset.ID, &out)
}

// transcribe turns the footage into words with timings.
func transcribe(url string) ([]trendlymodels.TranscriptWord, string, error) {
	t, err := elevenlabs.TranscribeURL(url)
	if err != nil {
		return nil, "", err
	}
	words := make([]trendlymodels.TranscriptWord, 0, len(t.Words))
	for _, w := range t.Words {
		start, end := w.Ms()
		words = append(words, trendlymodels.TranscriptWord{
			Text:    w.Text,
			StartMs: start,
			EndMs:   end,
			// Spacing is KEPT, not filtered. A span deleted from the transcript
			// has to take its surrounding silence with it, or every cut leaves
			// a gap where the pause used to be.
			Kind: w.Type,
		})
	}
	return words, t.Text, nil
}

// shotListSchema is what the vision pass must return.
// shotListTokens caps the reply. A shot list for twelve frames is a handful of
// short sentences; a cap keeps a model that starts narrating from running on.
const shotListTokens = 1200

const shotListPrompt = `You are looking at evenly spaced frames from one video, in order.

Describe what is on screen as a short list of shots. For each shot give the
frame index it starts at and the frame index it ends at (inclusive), and one
plain sentence describing what is visible — the subject, the framing, what is
happening. Write it the way someone would describe the clip to a colleague who
has not seen it, because that is what it will be matched against when someone
asks for "the bit where...".

Do not guess at anything you cannot see. Do not describe the video's purpose or
its mood. If consecutive frames show the same thing, they are one shot.

Return JSON only: {"shots":[{"from":0,"to":2,"description":"..."}]}`

// describeShots asks a multimodal model what is in the sampled frames.
//
// Frame INDICES in, milliseconds out. The model is good at "these three frames
// are the same shot" and bad at arithmetic on timestamps, so it is asked for
// the thing it can do and the conversion is done here, where the sampling
// interval is actually known.
func describeShots(ctx context.Context, brandID string, ingest *trendlymodels.AssetIngest) ([]trendlymodels.Shot, error) {
	frames := ingest.SampleFrameURLs
	if len(frames) == 0 {
		return nil, fmt.Errorf("no sample frames")
	}

	model, locked := ai.PickModelFor(ctx, brandID, openrouter.TaskMultimodal)
	if locked {
		return nil, fmt.Errorf("the plan does not include the vision pass")
	}

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:       model,
		Temperature: 0,
		MaxTokens:   shotListTokens,
		Messages: []openrouter.Message{
			{Role: "system", Content: shotListPrompt},
			openrouter.UserMessageWithImages(
				fmt.Sprintf("%d frames, evenly spaced across %dms of video.",
					len(frames), ingest.DurationMs),
				frames),
		},
	})
	if err != nil {
		return nil, err
	}
	// Metered like any other AI action. Describing twelve frames is a real
	// multimodal call, not a free side effect of uploading.
	ai.MeterBrandUsage(brandID, resp.Usage)
	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason == "length" {
		return nil, fmt.Errorf("the shot list hit the token cap before closing its JSON")
	}

	var parsed struct {
		Shots []indexedShot `json:"shots"`
	}
	reply := ""
	if len(resp.Choices) > 0 {
		reply = resp.Choices[0].Message.Content
	}
	if err := json.Unmarshal([]byte(stripFence(reply)), &parsed); err != nil {
		return nil, fmt.Errorf("the shot list came back unreadable: %w", err)
	}

	return shotsFromIndices(ingest, parsed.Shots), nil
}

// indexedShot is what the model returns: frame indices, not times.
type indexedShot struct {
	From        int    `json:"from"`
	To          int    `json:"to"`
	Description string `json:"description"`
}

// shotsFromIndices converts frame indices into milliseconds.
//
// Each frame stands for one slice of the video, sampled at its midpoint — so a
// shot running from frame a to frame b covers the slices those frames were
// taken from: the start of a's slice to the end of b's. That is also why the
// end uses to+1, and why the last shot reaches the end of the footage instead
// of stopping a slice short.
//
// Indices are clamped and nonsense is dropped rather than stored. A model that
// invents a thirteenth frame out of twelve must not produce a shot that runs
// past the end of the video, because something will later trim to it.
func shotsFromIndices(ingest *trendlymodels.AssetIngest, shots []indexedShot) []trendlymodels.Shot {
	frames := len(ingest.SampleFrameURLs)
	if frames == 0 || ingest.DurationMs <= 0 {
		return nil
	}
	sliceMs := float64(ingest.DurationMs) / float64(frames)

	out := make([]trendlymodels.Shot, 0, len(shots))
	for _, s := range shots {
		from, to := s.From, s.To
		if from < 0 {
			from = 0
		}
		if to >= frames {
			to = frames - 1
		}
		if to < from || strings.TrimSpace(s.Description) == "" {
			continue
		}
		out = append(out, trendlymodels.Shot{
			StartMs:     int(float64(from) * sliceMs),
			EndMs:       int(float64(to+1) * sliceMs),
			Description: strings.TrimSpace(s.Description),
		})
	}
	return out
}

// stripFence removes a ```json wrapper a model adds despite being asked for
// JSON only. Cheaper than a retry, and it happens often enough to be worth it.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.Index(s, "\n"); i > 0 {
		s = s[i+1:]
	}
	return strings.TrimSuffix(strings.TrimSpace(s), "```")
}
