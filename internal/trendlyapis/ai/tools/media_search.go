package tools

// search_media — find a piece of the brand's media by what it is.
//
// get_media_library lists the newest rows, which answers "what is there" and
// not "where is the one I mean". The difference matters as soon as media is a
// HISTORY rather than a recent-uploads tray: the still of the founder was made
// three weeks and sixty generations ago, and the planner needs it by
// description, not by scrolling.
//
// It searches titles, alt text, the PROMPT that produced a generation, and —
// for footage — the transcript and the shot descriptions. The prompt is the
// important one: a generated clip has no filename anybody chose, so the only
// words attached to it are the words it was asked to be.

import (
	"context"
	"fmt"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func searchMedia() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"search_media",
			"Search the brand's media library by description — titles, alt text, the prompt a generated image or clip was made from, and what is said or shown in uploaded footage. Use this to REUSE something that already exists ('the still of the founder at her desk', 'the clip where the cup is picked up') before paying to generate it again. Returns ready media only; a still mid-upload or an archived row is not offered.",
			openrouter.ObjectSchema(map[string]any{
				"query": openrouter.StringProp("What to look for, in plain words. Every word must appear somewhere on the row, so prefer two or three distinctive ones over a sentence."),
				"kind":  openrouter.EnumProp("Restrict to one kind of media.", []string{"image", "video", "audio"}),
				"limit": openrouter.NumberProp("Max rows to return (default 20)."),
			}, []string{"query"}),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			query := argStr(args, "query")
			if query == "" {
				return nil, fmt.Errorf("search_media needs a query")
			}
			rows, err := trendlymodels.SearchAssets(ctx, brandID, query,
				argStr(args, "kind"), argInt(args, "limit", 20))
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"count": len(rows),
				"media": mediaRows(rows),
				// Said explicitly because an empty result is a decision point:
				// the planner should generate, not search again with synonyms.
				"note": "An empty result means nothing matches; generate it instead of searching again.",
			}, nil
		},
	}
}

// mediaRows flattens media for a model to read.
//
// Only fields that are set, because a library list is read in full on every
// turn it is called and empty keys are tokens spent saying nothing.
func mediaRows(rows []trendlymodels.Asset) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		row := map[string]any{"id": a.ID, "kind": a.Kind, "url": a.URL}
		for k, v := range map[string]string{
			"title": a.Title, "alt": a.Alt, "source": a.Source,
		} {
			if v != "" {
				row[k] = v
			}
		}
		if a.Width > 0 && a.Height > 0 {
			row["size"] = fmt.Sprintf("%dx%d", a.Width, a.Height)
		}
		if a.DurationMs > 0 {
			row["durationMs"] = a.DurationMs
		}
		if a.Origin != nil {
			org := map[string]any{"kind": a.Origin.Kind}
			for k, v := range map[string]string{
				"model": a.Origin.Model, "prompt": a.Origin.Prompt,
				"provider": a.Origin.Provider, "sourceMediaId": a.Origin.SourceMediaID,
			} {
				if v != "" {
					org[k] = v
				}
			}
			if a.Origin.AtMs > 0 {
				org["atMs"] = a.Origin.AtMs
			}
			row["origin"] = org
		}
		// The model has to know this BEFORE it proposes a photo as a
		// reference, or the generation path refuses it afterwards and the
		// refusal reads to the user as a broken feature rather than a licence.
		if a.NoAIInput {
			row["usableAsReference"] = false
			row["note"] = "may be placed in a design but NOT fed to a model as an input"
		}
		out = append(out, row)
	}
	return out
}
