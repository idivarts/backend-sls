package design

// What one design may spend on media.
//
// ── The decision, and why it is this one ──────────────────────────────────
// The plan says "a per-project credit ceiling written from the org balance".
// Taken literally that needs an exchange rate between our AI-token wallet
// and HeyGen credits, and nobody has set one. Inventing a rate here would
// mean inventing what a customer is charged, which is a pricing decision
// with real money attached and not mine to make.
//
// So the ceiling is a FIXED per-design cap, and the wallet is consulted only
// for the question it can actually answer: does this org have standing to
// spend at all?
//
// That is not a compromise on safety. The purpose of a ceiling is to bound
// the blast radius of one bad run — a brief that asks for twelve generated
// shots, a retry loop, a model that declares media it does not need. A fixed
// cap does that exactly as well as a converted one, and it does it without
// pretending to a precision the inputs do not have.
//
// When a rate is agreed, this is the one function to change.

import (
	"os"
	"strconv"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// defaultDesignCreditCeiling is what one design may spend if nothing says
// otherwise.
//
// Deliberately small. A catalogue search — which is what the common case
// resolves to, and what every design on dev has used so far — costs nothing,
// so this ceiling only ever binds on generation. Twenty is enough for a
// couple of generated assets in one film and nowhere near enough for a
// runaway job to matter.
const defaultDesignCreditCeiling = 20

// MediaCreditCeiling is what to pass the worker for this org's design.
//
// Zero means "no media spending": the worker still resolves from anything
// free, and Hyperframes' own credit-limit refuses every paid call. An org
// out of tokens therefore keeps working — it simply stops buying — which is
// the behaviour the entitlements rule asks for: pre-empt, do not punish.
func MediaCreditCeiling(orgID string) int {
	if orgID == "" || trendlymodels.TokensExhausted(orgID) {
		return 0
	}
	return designCreditCeiling()
}

// designCreditCeiling reads the cap, so it can be lowered without a deploy
// if a run ever looks expensive.
func designCreditCeiling() int {
	raw := os.Getenv("DESIGN_MEDIA_CREDIT_CEILING")
	if raw == "" {
		return defaultDesignCreditCeiling
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return defaultDesignCreditCeiling
	}
	return n
}
