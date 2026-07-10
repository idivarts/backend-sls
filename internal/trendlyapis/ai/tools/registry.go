// Package tools holds the read-only "fetch" tools the planner AI can call on
// demand to ground its output in the brand's real data (content, analytics,
// inbox, strategy, account, assets, billing). Each tool is side-effect-free and
// scoped to a single brand. Handlers are dispatched from the AI chat loop via
// AllTools()/Dispatch().
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
	// Content & calendar
	contentInTimeframe(),
	contentDetails(),
	contentGaps(),
	draftBacklog(),
	contentPillars(),
	contentPillarBalance(),

	// Analytics & performance
	accountOverview(),
	topPerformingPosts(),
	underperformers(),
	followerGrowth(),
	engagementTrends(),
	formatPerformance(),
	bestPostingTimes(),
	hashtagPerformance(),
	comparePeriods(),
	audienceDemographics(),
	postAnalytics(),

	// Inbox / comments / community
	inboxConversations(),
	inboxThread(),
	postComments(),
	unansweredQuestions(),
	communitySentiment(),

	// Account & audience
	connectedAccounts(),
	accountProfile(),

	// Strategy & brand knowledge
	brandProfile(),
	brandMemory(),
	activeStrategy(),
	pastStrategies(),
	strategyContent(),

	// Assets & media
	mediaLibrary(),
	generatedAssets(),

	// Ops & scheduling intelligence
	publishFailures(),
	scheduleConflicts(),
	suggestOptimalSlot(),

	// Billing / self-governance
	entitlementsUsage(),
}

// ErrToolNotFound is returned by Dispatch when no registered tool matches the
// name, so callers can distinguish "not one of ours" from an execution error.
var ErrToolNotFound = errors.New("tool not found")

// AllTools returns the OpenRouter tool definitions for every registered fetch
// tool (attached to the model's tool list per module).
func AllTools() []openrouter.Tool {
	out := make([]openrouter.Tool, 0, len(registry))
	for _, r := range registry {
		out = append(out, r.Tool)
	}
	return out
}

// Has reports whether name is a registered fetch tool.
func Has(name string) bool {
	for _, r := range registry {
		if r.Tool.Function.Name == name {
			return true
		}
	}
	return false
}

// Dispatch runs a registered fetch tool and returns its result JSON-encoded for
// the model. Returns ErrToolNotFound when the name isn't registered.
func Dispatch(ctx context.Context, brandID, name, arguments string) (string, error) {
	for _, r := range registry {
		if r.Tool.Function.Name != name {
			continue
		}
		var args map[string]any
		if arguments != "" {
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return "", err
			}
		}
		res, err := r.Handler(ctx, brandID, args)
		if err != nil {
			return "", err
		}
		b, mErr := json.Marshal(res)
		if mErr != nil {
			return "", mErr
		}
		return string(b), nil
	}
	return "", ErrToolNotFound
}
