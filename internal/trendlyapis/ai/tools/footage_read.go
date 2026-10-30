package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// footageLibrary lets the model see what is IN the brand's uploaded video, not
// just that some video exists.
//
// This is the payoff of ingest. "Use the bit where she picks up the cup" is
// only answerable because something recorded that there is a cup and when —
// and the model can only act on that if it can read the shot list and the
// transcript without being handed the whole video.
func footageLibrary() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_footage",
			"List the brand's uploaded video with what is in it: how long each clip is, what is said in it and when, and a shot list describing what is on screen over time. Use this to find the right piece of footage and the right moment inside it before placing anything.",
			openrouter.ObjectSchema(map[string]any{
				"assetId": openrouter.StringProp("Optional: one asset to return in full, including its whole transcript."),
				"limit":   openrouter.NumberProp("Max clips to list (default 10)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			if id := argStr(args, "assetId"); id != "" {
				asset, err := trendlymodels.GetAsset(ctx, brandID, id)
				if err != nil {
					return nil, err
				}
				if asset == nil {
					return map[string]any{"error": "no such clip"}, nil
				}
				return footageDetail(asset, true), nil
			}

			list, err := trendlymodels.ListFootageAssets(ctx, brandID, argInt(args, "limit", 10))
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(list))
			for i := range list {
				// Summaries only in the list: a full transcript per clip would
				// fill the context with words nobody has asked for yet, and
				// the point of the shot list is to narrow down which clip to
				// look at properly.
				out = append(out, footageDetail(&list[i], false))
			}
			return map[string]any{"count": len(out), "footage": out}, nil
		},
	}
}

// footageDetail renders one clip for the model. `full` includes the transcript.
func footageDetail(a *trendlymodels.Asset, full bool) map[string]any {
	row := map[string]any{"assetId": a.ID, "url": a.URL}
	if a.Title != "" {
		row["title"] = a.Title
	}

	if a.Ingest == nil {
		// Said plainly, so the model does not read silence as "this clip is
		// empty" and skip a perfectly good take.
		row["ingest"] = "not analysed yet — it can still be placed and trimmed by hand"
		return row
	}
	switch a.Ingest.Status {
	case trendlymodels.IngestPending:
		row["ingest"] = "still being analysed"
	case trendlymodels.IngestFailed:
		row["ingest"] = "could not be analysed: " + a.Ingest.Error
	}

	if a.Ingest.DurationMs > 0 {
		row["durationMs"] = a.Ingest.DurationMs
	}
	if a.Ingest.Width > 0 {
		row["size"] = fmt.Sprintf("%dx%d", a.Ingest.Width, a.Ingest.Height)
	}
	row["hasAudio"] = a.Ingest.HasAudio

	if len(a.Ingest.Shots) > 0 {
		shots := make([]map[string]any, 0, len(a.Ingest.Shots))
		for _, s := range a.Ingest.Shots {
			shots = append(shots, map[string]any{
				"startMs": s.StartMs, "endMs": s.EndMs, "description": s.Description,
			})
		}
		row["shots"] = shots
	}

	if full && len(a.Ingest.Words) > 0 {
		// Word timings, not just the prose: the whole point is that the model
		// can answer "start on the line about pricing" with a millisecond.
		words := make([]map[string]any, 0, len(a.Ingest.Words))
		for _, w := range a.Ingest.Words {
			if w.Kind == "spacing" {
				continue
			}
			words = append(words, map[string]any{
				"text": w.Text, "startMs": w.StartMs, "endMs": w.EndMs,
			})
		}
		row["words"] = words
		row["transcript"] = a.Ingest.Text
	} else if a.Ingest.Text != "" {
		row["transcript"] = truncateWords(a.Ingest.Text, 60)
	}
	return row
}

// truncateWords keeps a list readable without spending the context on one clip.
func truncateWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) <= n {
		return s
	}
	return strings.Join(f[:n], " ") + "…"
}
