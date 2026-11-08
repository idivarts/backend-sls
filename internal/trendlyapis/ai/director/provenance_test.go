package director

import "testing"

// Realistic narration lines, with what SHOULD happen to each.
//
// This test exists because the first version of inventedSpecific refused
// "Back in 2024 we had no idea" as an invented figure — fatally, so one
// ordinary line about WHEN something happened killed a whole plan. A rule this
// sharp needs a table of real sentences, not just the cases it was written for.
func TestInventedSpecificOnRealisticLines(t *testing.T) {
	clean := []string{
		"Three things nobody tells you about launching a brand.",
		"Day one was the hardest.",
		"We shot this in 2 takes.",
		"Here are the 5 steps.",
		"It took about 20 minutes.",
		"Swipe up for the full guide.",
		"I started this in 2019.",      // a YEAR, not a claim
		"Back in 2024 we had no idea.", // a YEAR, not a claim
		"Chapter 12, if you're following along",
		"Our team of 8 does all of it.",
		"Rated number 1 by our customers.",
		"The 90s called and they want it back.",
	}
	flagged := map[string]string{
		"We saved them $4,000 a month.":      "a price",
		"Conversion went up 32%.":            "a percentage",
		"Over 10000 founders use this.":      "a figure",
		"It costs ₹499 a month.":             "a price",
		"We grew 1,200 followers in a week.": "a figure",
		"A 2.5% lift, every single time.":    "a percentage",
	}

	for _, l := range clean {
		if got := inventedSpecific(l); got != "" {
			t.Errorf("FALSE POSITIVE %q -> %q", l, got)
		}
	}
	for l, want := range flagged {
		got := inventedSpecific(l)
		if got == "" {
			t.Errorf("MISSED %q (wanted %q)", l, want)
		} else if got != want {
			t.Logf("note: %q -> %q (wanted %q) — still refused, different label", l, got, want)
		}
	}
}
