package ai

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// brandPlan resolves a brand's subscription plan (free/pro/team/agency) from its
// organization's billing. Falls back to free when anything is missing.
func brandPlan(brandID string) openrouter.Plan {
	if brandID == "" {
		return openrouter.PlanFree
	}
	b, err := loadBrand(brandID)
	if err != nil || b.OrganizationID == nil || *b.OrganizationID == "" {
		return openrouter.PlanFree
	}
	org := &trendlymodels.Organization{}
	if err := org.Get(*b.OrganizationID); err != nil {
		return openrouter.PlanFree
	}
	if org.Billing != nil && org.Billing.PlanKey != nil {
		return openrouter.PlanFromKey(*org.Billing.PlanKey)
	}
	return openrouter.PlanFree
}

// Token budget for the design lane.
//
// These exist because an UNSET max_tokens is not "no limit" — OpenRouter
// authorizes the request against the model's full output ceiling and refuses it
// outright when the balance will not cover that. On Claude Opus that ceiling is
// 65536 tokens, so a design turn demanded a 65k float to make a request that
// would really spend a fraction of it, and failed with a 402 while the account
// still had credit.
//
// The split is stated rather than inferred, which is the other half of the same
// problem. Anthropic counts thinking INSIDE max_tokens, and `effort` hands the
// provider a proportion of it — so capping max_tokens while asking for high
// effort would have quietly spent most of the budget thinking and truncated the
// HTML document mid-element. An explicit reasoning budget leaves a known
// remainder for the answer.
const (
	// Enough for a long multi-slide document plus its thinking, and well under
	// the model ceiling so the authorization is not the binding constraint.
	designMaxTokens = 32000
	// A real budget for laying out a frame and timing a scene, leaving ~24k for
	// the document itself.
	designReasoningTokens = 8000
)

// reasoningFor returns the thinking budget a task should run with, or nil for
// the provider default.
//
// Only the tasks where thinking demonstrably changes the OUTPUT get it, because
// reasoning tokens are billed and metered against the org's wallet like any
// other. Design is the clear case: fitting text inside a frame, balancing a
// layout and timing a scene against its duration are spatial problems a model
// gets measurably better at when it is allowed to work them through. Ordinary
// chat is not — thinking there buys a slower reply that says the same thing.
//
// Exclude is always set: the reasoning is for the model, and streaming it into
// the chat bubble would show the brand manager a monologue they did not ask for.
//
// MaxTokens, not Effort: see the note on designMaxTokens. Send one or the
// other — OpenRouter rejects a request carrying both.
func reasoningFor(task openrouter.TaskType) *openrouter.Reasoning {
	switch task {
	case openrouter.TaskDesign:
		return &openrouter.Reasoning{MaxTokens: designReasoningTokens, Exclude: true}
	default:
		return nil
	}
}

// maxTokensFor bounds the completion a task may ask for. 0 leaves it unset,
// which lets OpenRouter authorize against the model's ceiling.
//
// Only design is capped here. The other tasks are left alone deliberately:
// strategy and script produce long documents, and guessing a ceiling for them
// risks truncating output mid-sentence — a worse failure than the one this
// fixes. They remain exposed to the same authorization problem on a
// large-ceiling model, which is worth revisiting with real usage numbers.
func maxTokensFor(task openrouter.TaskType) int {
	switch task {
	case openrouter.TaskDesign:
		return designMaxTokens
	default:
		return 0
	}
}

// pickModel resolves the model to use for a task, enforcing per-plan gating from
// the Firestore-backed registry. locked=true means the brand's plan unlocks no
// model allowed for this task (the caller must surface an upgrade prompt instead
// of running the model).
func pickModel(ctx context.Context, brandID string, task openrouter.TaskType, requested string) (model string, locked bool) {
	openrouter.EnsureRegistry(ctx)
	return openrouter.Resolve(task, brandPlan(brandID), requested)
}
