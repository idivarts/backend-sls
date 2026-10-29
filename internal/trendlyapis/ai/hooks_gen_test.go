package ai

import (
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func TestHookProblems(t *testing.T) {
	cases := []struct {
		name string
		hook string
		want []string
	}{
		{
			name: "a clean hook trips nothing",
			hook: "Eighteen hours for one glass of cold brew. Here's why we don't rush it.",
		},
		{
			name: "throat-clearing is capped",
			hook: "Hey guys, welcome back to the channel! Today we're talking cold brew.",
			want: []string{"D2"},
		},
		{
			name: "bait is capped",
			hook: "Comment YES if you want the recipe for this cold brew.",
			want: []string{"D8"},
		},
		{
			name: "an over-long hook is only a warning",
			hook: strings.Repeat("word ", 25),
			want: []string{"D14"},
		},
		{
			name: "an empty hook is fatal",
			hook: "   ",
			want: []string{"D1"},
		},
		{
			// A hook arriving from the editor is HTML, like any other script
			// text, and the checks have to see through it.
			name: "html is read as the text it renders",
			hook: "<p><strong>Hey guys,</strong> welcome back!</p>",
			want: []string{"D2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := problemIDs(hookProblems(c.hook))
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("hookProblems(%q) = %v, want %v", c.hook, got, c.want)
			}
		})
	}
}

// D1 fires on an empty hook and on nothing else — a check that fired on every
// candidate would rank them all identically and tell the user nothing.
func TestHookProblemsDoesNotDemandABody(t *testing.T) {
	for _, hook := range []string{
		"Eighteen hours for one glass of cold brew.",
		"We tested nine grinders. Eight of them ruined the cup.",
		"This costs us 40% more to make. Here's why we kept it.",
	} {
		if ids := problemIDs(hookProblems(hook)); len(ids) != 0 {
			t.Errorf("%q should be clean, got %v", hook, ids)
		}
	}
}

func TestApplyHookVerdictScores(t *testing.T) {
	d := hookDimension()
	full := make([]judgeVerdict, 0, len(d.Criteria))
	for _, c := range d.Criteria {
		full = append(full, judgeVerdict{ID: c.ID, Score: 2})
	}

	t.Run("all exemplary is 100", func(t *testing.T) {
		c := HookCandidate{Text: "x"}
		applyHookVerdict(&c, d, full)
		if c.Score != 100 {
			t.Errorf("want 100, got %d", c.Score)
		}
	})

	t.Run("all adequate is 50", func(t *testing.T) {
		half := make([]judgeVerdict, 0, len(d.Criteria))
		for _, cr := range d.Criteria {
			half = append(half, judgeVerdict{ID: cr.ID, Score: 1})
		}
		c := HookCandidate{Text: "x"}
		applyHookVerdict(&c, d, half)
		if c.Score != 50 {
			t.Errorf("want 50, got %d", c.Score)
		}
	})

	t.Run("a skipped criterion scores zero and says so", func(t *testing.T) {
		c := HookCandidate{Text: "x"}
		applyHookVerdict(&c, d, full[:len(full)-1])
		if c.Score == 100 {
			t.Error("a dropped criterion must not leave the score untouched")
		}
		var found bool
		for _, cr := range c.Criteria {
			if cr.Note == "not assessed" {
				found = true
			}
		}
		if !found {
			t.Error("the skipped criterion should be recorded as not assessed")
		}
	})

	t.Run("a non-warning problem caps a strong hook", func(t *testing.T) {
		c := HookCandidate{
			Text:     "Hey guys, welcome back!",
			Problems: hookProblems("Hey guys, welcome back!"),
		}
		applyHookVerdict(&c, d, full)
		if c.Score > 60 {
			t.Errorf("throat-clearing should cap at 60, got %d", c.Score)
		}
	})

	t.Run("a warning does not cap", func(t *testing.T) {
		long := strings.Repeat("word ", 25)
		c := HookCandidate{Text: long, Problems: hookProblems(long)}
		applyHookVerdict(&c, d, full)
		if c.Score != 100 {
			t.Errorf("a warning should leave the score alone, got %d", c.Score)
		}
	})

	t.Run("notes are kept only for marked-down criteria", func(t *testing.T) {
		mixed := []judgeVerdict{
			{ID: d.Criteria[0].ID, Score: 2, Note: "should be dropped"},
			{ID: d.Criteria[1].ID, Score: 0, Note: "no specific claim"},
		}
		c := HookCandidate{Text: "x"}
		applyHookVerdict(&c, d, mixed)
		if len(c.Notes) != 1 || c.Notes[0] != "no specific claim" {
			t.Errorf("want the one marked-down note, got %v", c.Notes)
		}
	})
}

func TestSumUsageHandlesNils(t *testing.T) {
	if got := sumUsage(nil, nil); got != nil {
		t.Errorf("two nils should fold to nil, got %v", got)
	}
	one := &openrouter.Usage{TotalTokens: 10, PromptTokens: 7, CompletionTokens: 3}
	if got := sumUsage(nil, one); got != one {
		t.Error("a nil on the left should pass the other through untouched")
	}
	if got := sumUsage(one, nil); got != one {
		t.Error("a nil on the right should pass the other through untouched")
	}
	// Folding must not mutate either operand: the generation usage is metered
	// on the failure path too, so a mutated copy would double-count it.
	two := &openrouter.Usage{TotalTokens: 5, PromptTokens: 4, CompletionTokens: 1}
	got := sumUsage(one, two)
	if got.TotalTokens != 15 || got.PromptTokens != 11 || got.CompletionTokens != 4 {
		t.Errorf("unexpected fold: %+v", got)
	}
	if one.TotalTokens != 10 || two.TotalTokens != 5 {
		t.Error("sumUsage must not mutate its operands")
	}
}
