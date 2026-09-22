package tools

import (
	"context"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func mediaLibrary() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_media_library",
			"Aggregate reusable media attachments (images/videos) across the brand's content from roughly the last year, so you can reuse existing assets instead of generating new ones.",
			openrouter.ObjectSchema(map[string]any{
				"limit": openrouter.NumberProp("Max assets to return (default 40)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			now := time.Now().UTC()
			start := now.AddDate(-1, 0, 0).UnixMilli()
			end := now.AddDate(0, 3, 0).UnixMilli()
			items, err := trendlymodels.ListContentInRange(ctx, brandID, start, end, true)
			if err != nil {
				return nil, err
			}
			limit := argInt(args, "limit", 40)
			out := make([]map[string]any, 0, limit)
			for _, ct := range items {
				for _, a := range ct.Attachments {
					if len(out) >= limit {
						break
					}
					url := a.ImageURL
					if url == "" {
						url = a.PlayURL
					}
					if url == "" {
						continue
					}
					out = append(out, map[string]any{
						"contentId": ct.ID, "title": ct.Title, "type": a.Type, "url": url,
					})
				}
			}
			return map[string]any{"count": len(out), "assets": out}, nil
		},
	}
}

func generatedAssets() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_generated_assets",
			"List AI-generated assets for the brand: generated audio (music/voiceover), and — when a contentId is given — that content's HTML design revisions.",
			openrouter.ObjectSchema(map[string]any{
				"kind":      openrouter.EnumProp("Optional audio kind filter.", []string{"music", "voiceover"}),
				"contentId": openrouter.StringProp("Optional content ID to also return its design revisions."),
				"limit":     openrouter.NumberProp("Max items per category (default 20)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			limit := argInt(args, "limit", 20)
			audio, err := trendlymodels.ListGeneratedAudio(brandID, argStr(args, "kind"), limit)
			if err != nil {
				return nil, err
			}
			audioOut := make([]map[string]any, 0, len(audio))
			for _, a := range audio {
				audioOut = append(audioOut, map[string]any{
					"id": a.ID, "kind": a.Kind, "prompt": a.Prompt, "url": a.URL,
					"durationMs": a.DurationMs, "status": a.Status,
				})
			}
			res := map[string]any{"audio": audioOut}
			if cid := argStr(args, "contentId"); cid != "" {
				revs, rerr := trendlymodels.ListDesignRevisions(brandID, cid, limit)
				if rerr == nil {
					designOut := make([]map[string]any, 0, len(revs))
					for _, r := range revs {
						designOut = append(designOut, map[string]any{
							"id": r.ID, "docType": r.DocType, "slideCount": r.SlideCount,
							"renderUrl": r.RenderURL, "createdAt": r.CreatedAt,
						})
					}
					res["designs"] = designOut
				}
			}
			return res, nil
		},
	}
}
