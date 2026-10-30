package trendlymodels

import (
	"context"
	"math"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// OrgTokenWallet is the single shared AI-token wallet for an organization — all
// brands in the org draw from one balance. Tokens are "model-weighted": one
// wallet token == one token of usage on the baseline/default model (Gemini 3.5
// Flash). AI actions deduct the real provider cost converted via TokensForCost
// — at a margin, see BillingMarginX — so the wallet drains in proportion to
// actual spend regardless of which model (or how much context) was used, and
// draining it leaves us ahead rather than square. See the Credit ticket §5b.
type OrgTokenWallet struct {
	Balance          int64 `json:"balance" firestore:"balance"`                   // monthly allotment remaining this period
	MonthlyAllotment int64 `json:"monthlyAllotment" firestore:"monthlyAllotment"` // refilled on the 1st of the month
	PeriodResetAt    int64 `json:"periodResetAt" firestore:"periodResetAt"`       // epoch ms of the next reset (the 1st)
	TopupBalance     int64 `json:"topupBalance" firestore:"topupBalance"`         // purchased packs; spent after Balance, NOT reset monthly
}

// WalletTokenValueUSD is the USD value of one wallet token: the blended in/out
// cost of one token on the baseline/default model (Gemini 3.5 Flash, ~$5.25 per
// 1M tokens). AI usage is metered by deducting round-up(Usage.Cost / this).
// If the free/default model changes, re-check ONLY this constant (Credit §5b).
const WalletTokenValueUSD = 0.00000525

// BillingMarginX is the margin on every provider cost, and the reason it lives
// here rather than at each call site.
//
// Before this, "charge the real cost" was taken literally: an OpenRouter call
// deducted exactly what it cost, so draining a wallet left us at precisely
// break-even — and below it, once the costs nobody bills for are counted
// (failed calls, retries, the OpenRouter platform fee on top of the model
// price, the Firestore and Lambda work around every request). Only the render
// and generated-video paths carried a markup, because those happened to be
// written later.
//
// 1.30 charges 130% of measured cost, which is a 23% margin on what the wallet
// is worth — inside the 20-25% target. It is applied HERE, in the one function
// every paid path already converts through, so a new provider cannot be wired
// up at cost by forgetting a multiplier.
//
// ⚠️ It stacks with renderCostMarkupX (2x), deliberately: that one covers
// retries and failures we swallow, which is a different thing from profit. A
// render therefore sells at 2.6x modelled cost. Over-charging is the intended
// direction of error.
const BillingMarginX = 1.30

// TokensForCost converts a real provider USD cost into model-weighted wallet
// tokens, with the margin applied.
//
// Rounds UP fractional tokens, which matters more than it looks: the smallest
// AI calls in the product cost a fraction of a cent, and truncating them would
// bill a long tail of real spend as zero.
func TokensForCost(costUSD float64) int64 {
	if costUSD <= 0 {
		return 0
	}
	t := int64(math.Ceil((costUSD * BillingMarginX) / WalletTokenValueUSD))
	if t < 0 {
		return 0
	}
	return t
}

// GetTokenWallet returns the org's wallet (nil if the org has no wallet yet).
func GetTokenWallet(orgID string) (*OrgTokenWallet, error) {
	var o Organization
	if err := o.Get(orgID); err != nil {
		return nil, err
	}
	return o.TokenWallet, nil
}

// HasTokens reports whether the org has any spendable tokens left (monthly
// balance + top-up). This is the pre-call gate before running an AI action; an
// org with no wallet (or an exhausted one) is blocked → upgrade prompt.
func HasTokens(orgID string) (bool, error) {
	w, err := GetTokenWallet(orgID)
	if err != nil {
		return false, err
	}
	if w == nil {
		return false, nil
	}
	return (w.Balance + w.TopupBalance) > 0, nil
}

// DeductTokens atomically subtracts `tokens` from the org wallet, draining the
// monthly Balance first then the TopupBalance. Metering is post-paid (we deduct
// AFTER the model call), so the LAST call of a period can overshoot — Balance
// floors at 0 and the remainder comes off TopupBalance, which may go slightly
// negative by at most one call's cost. The pre-call HasTokens gate then blocks
// the next call. Returns the remaining spendable total (Balance + TopupBalance).
func DeductTokens(orgID string, tokens int64) (int64, error) {
	if tokens <= 0 {
		w, err := GetTokenWallet(orgID)
		if err != nil || w == nil {
			return 0, err
		}
		return w.Balance + w.TopupBalance, nil
	}
	ctx := context.Background()
	ref := firestoredb.Client.Collection(orgCollection).Doc(orgID)
	var remaining int64
	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var o Organization
		if err := snap.DataTo(&o); err != nil {
			return err
		}
		w := o.TokenWallet
		if w == nil {
			w = &OrgTokenWallet{}
		}
		if w.Balance >= tokens {
			w.Balance -= tokens
		} else {
			rem := tokens - w.Balance
			w.Balance = 0
			w.TopupBalance -= rem // tolerated: bounded by one call's cost
		}
		remaining = w.Balance + w.TopupBalance
		return tx.Update(ref, []firestore.Update{{Path: "tokenWallet", Value: w}})
	})
	return remaining, err
}

// RefundTokens gives tokens back after work that was charged for and did not
// happen — a generation the provider failed, which it does not charge us for
// either.
//
// Returns them to the MONTHLY balance rather than the top-up balance, even
// when the deduction came partly out of top-up. That is deliberate and in the
// user's favour: monthly tokens expire at the reset and top-up tokens do not,
// so crediting the wrong one would be the version that quietly takes value
// away. The amounts are a single generation's cost, so the drift is bounded.
//
// Callers must guard against refunding twice — see MarkMediaJobRefunded. A
// webhook and a poll can both decide the same job failed.
func RefundTokens(orgID string, tokens int64) (int64, error) {
	if tokens <= 0 || orgID == "" {
		return 0, nil
	}
	ctx := context.Background()
	ref := firestoredb.Client.Collection(orgCollection).Doc(orgID)
	var remaining int64
	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var o Organization
		if err := snap.DataTo(&o); err != nil {
			return err
		}
		w := o.TokenWallet
		if w == nil {
			w = &OrgTokenWallet{}
		}
		w.Balance += tokens
		remaining = w.Balance + w.TopupBalance
		return tx.Update(ref, []firestore.Update{{Path: "tokenWallet", Value: w}})
	})
	return remaining, err
}

// RefillWallet resets the monthly Balance to `allotment` and sets the next reset
// timestamp, stamping MonthlyAllotment. TopupBalance is preserved (top-ups do
// not reset monthly). Called by the 1st-of-month billing cron and on a plan
// change. Pass periodResetAt = 0 to leave the existing reset timestamp intact.
func RefillWallet(orgID string, allotment int64, periodResetAt int64) error {
	ctx := context.Background()
	ref := firestoredb.Client.Collection(orgCollection).Doc(orgID)
	return firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var o Organization
		if err := snap.DataTo(&o); err != nil {
			return err
		}
		topup := int64(0)
		reset := periodResetAt
		if o.TokenWallet != nil {
			topup = o.TokenWallet.TopupBalance
			if reset == 0 {
				reset = o.TokenWallet.PeriodResetAt
			}
		}
		w := &OrgTokenWallet{
			Balance:          allotment,
			MonthlyAllotment: allotment,
			PeriodResetAt:    reset,
			TopupBalance:     topup,
		}
		return tx.Update(ref, []firestore.Update{{Path: "tokenWallet", Value: w}})
	})
}

// AddTopup atomically adds purchased tokens to the org's TopupBalance (overage
// top-up packs — see the Credit ticket: $10 → 1M tokens).
func AddTopup(orgID string, tokens int64) error {
	if tokens <= 0 {
		return nil
	}
	ctx := context.Background()
	ref := firestoredb.Client.Collection(orgCollection).Doc(orgID)
	return firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var o Organization
		if err := snap.DataTo(&o); err != nil {
			return err
		}
		w := o.TokenWallet
		if w == nil {
			w = &OrgTokenWallet{}
		}
		w.TopupBalance += tokens
		return tx.Update(ref, []firestore.Update{{Path: "tokenWallet", Value: w}})
	})
}
