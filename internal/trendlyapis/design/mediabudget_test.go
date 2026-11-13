package design

import (
	"os"
	"testing"
)

// The cap can be lowered without a deploy, which is the whole reason it reads
// an env var — so the parsing has to be the boring kind that never surprises.
func TestTheCeilingFallsBackToTheDefaultOnAnythingOdd(t *testing.T) {
	for _, raw := range []string{"", "not a number", "-5", "12.5"} {
		t.Setenv("DESIGN_MEDIA_CREDIT_CEILING", raw)
		if got := designCreditCeiling(); got != defaultDesignCreditCeiling {
			t.Errorf("%q gave %d, expected the default %d", raw, got, defaultDesignCreditCeiling)
		}
	}
}

func TestTheCeilingHonoursAValueThatMakesSense(t *testing.T) {
	t.Setenv("DESIGN_MEDIA_CREDIT_CEILING", "3")
	if got := designCreditCeiling(); got != 3 {
		t.Fatalf("got %d, expected 3", got)
	}
	// Zero is a legitimate setting: resolve from anything free, buy nothing.
	t.Setenv("DESIGN_MEDIA_CREDIT_CEILING", "0")
	if got := designCreditCeiling(); got != 0 {
		t.Fatalf("got %d, expected 0 to be honoured rather than treated as unset", got)
	}
}

// An org with no id cannot be billed, so it cannot be allowed to spend.
func TestNoOrgMeansNoSpending(t *testing.T) {
	_ = os.Unsetenv("DESIGN_MEDIA_CREDIT_CEILING")
	if got := MediaCreditCeiling(""); got != 0 {
		t.Fatalf("got %d, expected 0 for a design with no org", got)
	}
}
