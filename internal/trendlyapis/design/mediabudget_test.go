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

// A design the gate refuses is written again, and each attempt resolves its
// media from scratch. Handing every attempt the full ceiling would multiply
// the cap by the number of attempts — real money, for a feature that was
// supposed to cost tokens.
func TestARunNeverSpendsMoreThanOneDesignsCeiling(t *testing.T) {
	const whole = 20
	for attempts := 1; attempts <= 4; attempts++ {
		per := splitCeiling(whole, attempts)
		if total := per * attempts; total > whole {
			t.Fatalf("%d attempts of %d could spend %d, over the %d a design is allowed",
				attempts, per, total, whole)
		}
	}
	if got := splitCeiling(whole, 1); got != whole {
		t.Fatalf("a single attempt got %d, not the whole ceiling %d", got, whole)
	}
	// The number the loop actually uses must still buy something.
	if got := splitCeiling(defaultDesignCreditCeiling, maxAuthorAttempts); got <= 0 {
		t.Fatalf("the default ceiling split %d ways buys nothing at all", maxAuthorAttempts)
	}
}

// A nonsensical attempt count must not divide by zero or hand out a negative
// ceiling, which the worker would read as an unset limit.
func TestThePerAttemptCeilingSurvivesANonsenseCount(t *testing.T) {
	for _, n := range []int{0, -1, -1000} {
		if got := splitCeiling(20, n); got != 20 {
			t.Fatalf("%d attempts gave %d, expected the whole ceiling", n, got)
		}
	}
	// And an org that may not spend still may not spend.
	for _, n := range []int{1, 2, -1} {
		if got := splitCeiling(0, n); got != 0 {
			t.Fatalf("a zero ceiling split %d ways gave %d", n, got)
		}
	}
	if got := MediaCreditCeilingPerAttempt("", maxAuthorAttempts); got != 0 {
		t.Fatalf("a design with no org got a ceiling of %d", got)
	}
}

// A ceiling too small to divide comes out at zero, which means "buy nothing"
// — the same safe degradation an org out of tokens gets, not a failure.
func TestATinyCeilingDegradesToFreeOnly(t *testing.T) {
	if got := splitCeiling(1, 2); got != 0 {
		t.Fatalf("got %d, expected 0 rather than a rounded-up credit", got)
	}
}
