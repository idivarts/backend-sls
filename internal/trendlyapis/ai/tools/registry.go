package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type Handler func(ctx context.Context, brandID string, args map[string]any) (any, error)

type Registered struct {
	Tool    openrouter.Tool
	Handler Handler
}

var registry = []Registered{
	campaignPerformance(),
	activeCollaborations(),
	influencerStats(),
	searchInfluencers(),
	contractHistory(),
	strategyContent(),
	calendarPosts(),
}

func AllTools() []openrouter.Tool {
	out := make([]openrouter.Tool, 0, len(registry))
	for _, r := range registry {
		out = append(out, r.Tool)
	}
	return out
}

func Dispatch(ctx context.Context, brandID, name, arguments string) (any, error) {
	var args map[string]any
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return nil, err
		}
	}
	for _, r := range registry {
		if r.Tool.Function.Name == name {
			return r.Handler(ctx, brandID, args)
		}
	}
	return nil, errors.New("tool not found: " + name)
}
