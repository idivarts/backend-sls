package hfproject

import (
	"strings"
	"testing"
)

// The contrast bar belongs HERE, not in the brand kit.
//
// It used to live only in brandPromptBlock, which is built from a brand's
// design system — so a brand without one was asked to write a film with no
// mention of contrast at all, and then had it refused by a gate measuring
// exactly that. The gate is the same for every design; the instruction has to
// be too.
func TestTheContractNamesTheContrastBarEveryDesignIsHeldTo(t *testing.T) {
	c := AuthoringContract
	for _, want := range []string{"3:1", "4.5:1", "WCAG AA"} {
		if !strings.Contains(c, want) {
			t.Fatalf("the authoring contract never mentions %q, but check measures it", want)
		}
	}
	// Naming the threshold is not enough: a tint over a photograph is the
	// pairing that actually fails, and a model told only the number keeps
	// reaching for it.
	if !strings.Contains(c, "scrim") && !strings.Contains(c, "solid panel") {
		t.Fatal("the contract names the bar but not how to clear it over an image")
	}
}

// Every rule in the list is enforced by check, so the list has to stay a list:
// a rule appended without a number reads as prose and gets skimmed.
func TestTheContractRulesAreNumberedWithoutAGap(t *testing.T) {
	for n := 1; n <= 16; n++ {
		marker := "\n" + itoa(n) + ". "
		if !strings.Contains(AuthoringContract, marker) {
			t.Fatalf("rule %d is missing from the contract", n)
		}
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
