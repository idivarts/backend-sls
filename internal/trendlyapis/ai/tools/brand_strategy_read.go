package tools

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func brandProfile() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_brand_profile",
			"Fetch the brand's core profile: name, about, industries, website, target-audience preferences (categories, languages, locations, platforms), survey purpose and AI voice. Ground content generation in this.",
			openrouter.ObjectSchema(map[string]any{}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			b := &trendlymodels.Brand{}
			if err := b.Get(brandID); err != nil {
				return nil, err
			}
			res := map[string]any{"name": b.Name, "country": deref(b.Country), "aiVoice": deref(b.AIVoice)}
			if b.Profile != nil {
				res["about"] = deref(b.Profile.About)
				res["website"] = deref(b.Profile.Website)
				res["industries"] = b.Profile.Industries
			}
			if b.Preferences != nil {
				res["categories"] = b.Preferences.InfluencerCategories
				res["languages"] = b.Preferences.Languages
				res["locations"] = b.Preferences.Locations
				res["platforms"] = b.Preferences.Platforms
				res["contentVideoType"] = b.Preferences.ContentVideoType
			}
			if b.Survey != nil {
				res["purpose"] = deref(b.Survey.Purpose)
				res["source"] = deref(b.Survey.Source)
			}
			return res, nil
		},
	}
}

func brandMemory() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_brand_memory",
			"Fetch the brand's long-term AI memory blob (durable facts, preferences, do's and don'ts captured across past conversations).",
			openrouter.ObjectSchema(map[string]any{}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			b := &trendlymodels.Brand{}
			if err := b.Get(brandID); err != nil {
				return nil, err
			}
			return map[string]any{"memory": deref(b.AIMemory), "hasMemory": b.AIMemory != nil && *b.AIMemory != ""}, nil
		},
	}
}

func summarizeStrategy(s trendlymodels.Strategy, includeBody bool) map[string]any {
	m := map[string]any{
		"id": s.ID, "name": s.Name, "objective": s.Objective, "status": s.Status,
		"platforms": s.Platforms, "contentFormats": s.ContentFormats,
		"createdAt": s.CreatedAt, "updatedAt": s.UpdatedAt,
	}
	if s.Timeline != nil {
		m["timeline"] = map[string]any{"startDate": s.Timeline.StartDate, "endDate": s.Timeline.EndDate}
	}
	if includeBody {
		m["markdownContent"] = s.MarkdownContent
	}
	return m
}

func activeStrategy() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_active_strategy",
			"Fetch the brand's most recent content strategy document, including its full markdown body. Use it to keep calendar/content aligned to the current strategy.",
			openrouter.ObjectSchema(map[string]any{}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			list, err := trendlymodels.ListStrategies(ctx, brandID, 1)
			if err != nil {
				return nil, err
			}
			if len(list) == 0 {
				return map[string]any{"error": "no strategy found for this brand"}, nil
			}
			return summarizeStrategy(list[0], true), nil
		},
	}
}

func pastStrategies() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_past_strategies",
			"List the brand's strategy documents (compact — no bodies) to learn from prior direction. Use get_active_strategy or get_strategy_content for a full body.",
			openrouter.ObjectSchema(map[string]any{
				"limit": openrouter.NumberProp("Max strategies to return (default 10)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			limit := argInt(args, "limit", 10)
			list, err := trendlymodels.ListStrategies(ctx, brandID, limit)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(list))
			for _, s := range list {
				out = append(out, summarizeStrategy(s, false))
			}
			return map[string]any{"count": len(out), "strategies": out}, nil
		},
	}
}
