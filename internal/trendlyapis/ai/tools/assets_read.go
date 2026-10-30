package tools

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func mediaLibrary() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_media_library",
			"List the brand's reusable media library — uploads, stock photography and generated images — so you can reuse an existing asset instead of making a new one. Assets are brand-wide: one uploaded while editing any post is available to every post.",
			openrouter.ObjectSchema(map[string]any{
				"source": openrouter.EnumProp(
					"Optional filter by where the asset came from.",
					[]string{"brand", "upload", "stock", "generated"}),
				"limit": openrouter.NumberProp("Max assets to return (default 40)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			limit := argInt(args, "limit", 40)
			list, err := trendlymodels.ListBrandAssets(ctx, brandID, argStr(args, "source"), limit)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(list))
			for _, a := range list {
				row := map[string]any{
					"url": a.URL, "kind": a.Kind, "source": a.Source,
				}
				// Only what is actually set, so a long list stays cheap to read.
				for k, v := range map[string]string{
					"id": a.ID, "title": a.Title, "alt": a.Alt,
					"contentId": a.OriginContentID, "credit": a.Attribution,
				} {
					if v != "" {
						row[k] = v
					}
				}
				// The model must know this BEFORE it proposes using a photo as
				// a reference, or it will suggest something the generation path
				// then refuses — which reads to the user as a broken feature
				// rather than a licence.
				if a.NoAIInput {
					row["usableAsReference"] = false
					row["note"] = "licensed for placing in a design, but NOT as an input to image generation"
				}
				out = append(out, row)
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
