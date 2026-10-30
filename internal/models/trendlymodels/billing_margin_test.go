package trendlymodels

import (
	"math"
	"testing"

	"github.com/idivarts/backend-sls/pkg/elevenlabs"
)

// The margin is the whole point of the wallet: a user who drains it should
// leave us ahead, not square. These tests are the tripwire for that quietly
// becoming untrue — every one of them failed before the margin was applied.

func TestEveryPaidPathChargesAboveCost(t *testing.T) {
	// The target is 20-25% of revenue. 1.30x of cost is 23%.
	const wantMin = 1.20

	cases := []struct {
		name    string
		costUSD float64
		tokens  int64
	}{
		{"a cheap AI call", 0.0004, TokensForCost(0.0004)},
		{"a large design turn", 0.42, TokensForCost(0.42)},
		{"a 20s music bed", elevenlabs.MusicCostUSD(20_000), TokensForCost(elevenlabs.MusicCostUSD(20_000))},
		{"a 60s music bed", elevenlabs.MusicCostUSD(60_000), TokensForCost(elevenlabs.MusicCostUSD(60_000))},
		{"a 300-char voiceover", elevenlabs.TTSCostUSD(string(make([]rune, 300))), TokensForCost(elevenlabs.TTSCostUSD(string(make([]rune, 300))))},
		{"a 4-minute transcription", elevenlabs.STTCostUSD(240_000), TokensForCost(elevenlabs.STTCostUSD(240_000))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charged := float64(c.tokens) * WalletTokenValueUSD
			if c.costUSD <= 0 {
				t.Fatalf("%s has no cost to charge against", c.name)
			}
			ratio := charged / c.costUSD
			if ratio < wantMin {
				t.Fatalf("%s charges %.3fx cost ($%.6f for $%.6f); want at least %.2fx",
					c.name, ratio, charged, c.costUSD, wantMin)
			}
		})
	}
}

func TestMarginIsTheOneYouThinkItIs(t *testing.T) {
	// 1.30x of cost = 23.1% of revenue, which is the 20-25% the wallet was
	// designed around. If someone moves BillingMarginX, this says what moved.
	marginOfRevenue := (BillingMarginX - 1) / BillingMarginX
	if marginOfRevenue < 0.20 || marginOfRevenue > 0.25 {
		t.Fatalf("BillingMarginX %.2f gives a %.1f%% margin; the target is 20-25%%",
			BillingMarginX, marginOfRevenue*100)
	}
}

func TestARenderStacksTheMarginOnItsOwnMarkup(t *testing.T) {
	// renderCostMarkupX covers retries and failures we swallow, which is not
	// profit; the margin sits on top of it. A reel therefore sells at 2.6x
	// modelled cost, and that is intended.
	tokens := RenderTokenCost("video", 1, 15_000)
	modelled := renderVideoFixedUSD + renderVideoPerSecondUSD*15
	charged := float64(tokens) * WalletTokenValueUSD
	want := modelled * renderCostMarkupX * BillingMarginX
	if math.Abs(charged-want)/want > 0.01 {
		t.Fatalf("a 15s reel charges $%.6f; want ~$%.6f (2x markup x %.2f margin)",
			charged, want, BillingMarginX)
	}
}

func TestTokensForCostNeverRoundsRealSpendToZero(t *testing.T) {
	// The smallest calls in the product cost a fraction of a cent. Truncating
	// instead of rounding up would bill a long tail of real spend as free.
	if got := TokensForCost(0.0000001); got < 1 {
		t.Fatalf("a tiny cost billed %d tokens; want at least 1", got)
	}
	if got := TokensForCost(0); got != 0 {
		t.Fatalf("no cost must bill nothing, got %d", got)
	}
	if got := TokensForCost(-5); got != 0 {
		t.Fatalf("a negative cost must bill nothing, got %d", got)
	}
}

// What a plan's whole allotment can cost us, which is the question that
// matters for the free tier: it is given away.
func TestFreePlanWorstCaseCostIsBounded(t *testing.T) {
	free := PlanLimitsMap["free"]
	walletValue := float64(free.MonthlyAllotment) * WalletTokenValueUSD
	ourCost := walletValue / BillingMarginX

	// $1.05 of wallet value at a 1.30x margin is ~$0.81 of provider spend.
	// Pinned so that raising the free allotment is a deliberate act with a
	// number attached, rather than a one-character edit.
	if ourCost > 1.00 {
		t.Fatalf("a free user draining their wallet costs us $%.2f; that is above the $1.00 line",
			ourCost)
	}
}
