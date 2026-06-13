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
// Flash). AI actions deduct the real OpenRouter cost converted via TokensForCost,
// so the wallet drains in proportion to actual spend regardless of which model
// (or how much context) was used. See the Credit ticket §5b.
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

// TokensForCost converts a real OpenRouter USD cost into model-weighted wallet
// tokens. Rounds UP fractional tokens (margin-safe) and never returns negative.
func TokensForCost(costUSD float64) int64 {
	if costUSD <= 0 {
		return 0
	}
	t := int64(math.Ceil(costUSD / WalletTokenValueUSD))
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
