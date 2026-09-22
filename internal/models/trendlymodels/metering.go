package trendlymodels

// This file exposes the token-wallet metering seam for NON-ai handler packages
// (media/audio, canva, render) so they can gate + debit the org wallet without
// re-implementing brand→org resolution. The ai package keeps its own private
// copies; these are the shared, exported equivalents.

// OrgIDForBrand resolves a brand to its parent organization id. The second
// return is false when the brand has no org yet (rollout-safe: callers should
// then NOT block — a brand without an org predates the billing system).
func OrgIDForBrand(brandID string) (string, bool) {
	b := &Brand{}
	if err := b.Get(brandID); err != nil || b.OrganizationID == nil || *b.OrganizationID == "" {
		return "", false
	}
	return *b.OrganizationID, true
}

// TokensExhausted is the pre-call gate. It returns true only when the org has a
// wallet AND it is empty. No wallet ⇒ never block (rollout-safe).
func TokensExhausted(orgID string) bool {
	if orgID == "" {
		return false
	}
	w, err := GetTokenWallet(orgID)
	if err != nil || w == nil {
		return false
	}
	return (w.Balance + w.TopupBalance) <= 0
}

// MeterUSD debits the org wallet for a USD cost, best-effort (never fails the
// user's request on a metering error). Rounds the cost UP to margin-safe tokens.
func MeterUSD(orgID string, costUSD float64) {
	if orgID == "" || costUSD <= 0 {
		return
	}
	if w, err := GetTokenWallet(orgID); err != nil || w == nil {
		return
	}
	_, _ = DeductTokens(orgID, TokensForCost(costUSD))
}
