package ai

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// orgIDForBrand resolves a brand to its parent organization id. ok=false for a
// brand with no org yet (legacy / in-flight onboarding) — callers then fail OPEN
// (don't block, don't meter) during the token-wallet rollout.
func orgIDForBrand(brandID string) (string, bool) {
	b, err := loadBrand(brandID)
	if err != nil || b.OrganizationID == nil || *b.OrganizationID == "" {
		return "", false
	}
	return *b.OrganizationID, true
}

// aiTokensExhausted is the pre-call gate. It returns true ONLY when the org has
// a provisioned wallet that is out of tokens (monthly balance + top-up <= 0).
// Orgs without a wallet yet (pre-rollout / not migrated) are never blocked, so
// shipping the meter does not lock anyone out before wallets are provisioned.
func aiTokensExhausted(orgID string) bool {
	if orgID == "" {
		return false
	}
	w, err := trendlymodels.GetTokenWallet(orgID)
	if err != nil || w == nil {
		return false
	}
	return (w.Balance + w.TopupBalance) <= 0
}

// meterAIUsage deducts the real OpenRouter USD cost of a completed AI call from
// the org's token wallet (converted to model-weighted tokens via TokensForCost).
// No-op for orgs without a wallet yet, and for zero/unknown cost. Best-effort:
// metering must never fail the user's AI action, so errors are only logged.
func meterAIUsage(orgID string, usage *openrouter.Usage) {
	if orgID == "" || usage == nil || usage.Cost <= 0 {
		return
	}
	w, err := trendlymodels.GetTokenWallet(orgID)
	if err != nil || w == nil {
		return
	}
	if _, err := trendlymodels.DeductTokens(orgID, trendlymodels.TokensForCost(usage.Cost)); err != nil {
		log.Printf("ai meter: deduct tokens for org %s: %v", orgID, err)
	}
}
