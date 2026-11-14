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

// ⚠️ The contract's own examples are copied by the model, so an example that
// fails the gate fails every film that follows it.
//
// This is not hypothetical. The <audio> examples carried no data-start, and
// check refuses untimed media the moment it has a file to play — so the first
// design authored after the rewrite loop shipped was refused with
// `<audio id="bgm"> has src but no data-start`, which is exactly what the
// contract had told it to write.
func TestEveryMediaExampleInTheContractIsTimed(t *testing.T) {
	c := AuthoringContract
	els := mediaElements(c)
	// A finder that finds nothing passes every assertion below it.
	if len(els) < 3 {
		t.Fatalf("found %d media examples; the contract shows an image, a score and a voiceover", len(els))
	}
	for _, el := range els {
		if !strings.Contains(el, "data-start") {
			t.Errorf("this example is untimed, and check refuses untimed media:\n  %s", el)
		}
		if !strings.Contains(el, "data-duration") {
			t.Errorf("this example has a start but no duration:\n  %s", el)
		}
	}
	// A caption track is timed too, and it declares no media of its own.
	if !strings.Contains(c, `data-captions="vo" data-start=`) {
		t.Error("the caption-track example is untimed")
	}
	// And the rule is stated, not only demonstrated: a model that copies the
	// example gets it right, a model that writes its own needs telling.
	if !strings.Contains(c, "untimed media") {
		t.Error("the contract shows timed media but never says it is required")
	}
}

// mediaElements returns the full text of every element in the contract that
// declares media — from its opening "<" to the ">" that closes the tag, so a
// declaration wrapped across lines is still read as one thing.
func mediaElements(c string) []string {
	var out []string
	for i := 0; ; {
		j := strings.Index(c[i:], `data-media="`)
		if j < 0 {
			return out
		}
		at := i + j
		start := strings.LastIndex(c[:at], "<")
		end := strings.Index(c[at:], ">")
		if start < 0 || end < 0 {
			return out
		}
		out = append(out, strings.Join(strings.Fields(c[start:at+end+1]), " "))
		i = at + end + 1
	}
}
