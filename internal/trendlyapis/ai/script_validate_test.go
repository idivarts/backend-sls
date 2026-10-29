package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A script that should pass everything, used as the base for the negative
// cases below so each test varies exactly one thing.
const goodScript = `## Hook (first 3 seconds)
**On-screen text:** "18 hours for one cup."
**VO:** "Eighteen hours for one glass of cold brew. Here's why we don't rush it."

## Body
**Scene 1 — the assumption (4s)**
**On-screen text:** "Heat pulls bitterness out fast."
**VO:** "Hot water is fast, but it drags out the burnt notes too."

**Scene 2 — the switch (4s)**
**On-screen text:** "Cold water plus time."
**VO:** "Cold water only takes what dissolves gently."

**Scene 3 — the payoff (5s)**
**On-screen text:** "Smooth, never bitter."
**VO:** "So we give it eighteen hours of cold steeping, and what's left is smooth."

## CTA
**VO:** "Send this to the one person who thinks they hate cold brew."
`

func problemIDs(ps []ScriptProblem) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func hasProblem(ps []ScriptProblem, id string) bool {
	for _, p := range ps {
		if p.ID == id {
			return true
		}
	}
	return false
}

func TestValidateScriptAcceptsAGoodScript(t *testing.T) {
	if got := ValidateScript(goodScript, "reel"); len(got) != 0 {
		t.Errorf("expected a clean script, got problems %v", problemIDs(got))
	}
}

func TestValidateScriptEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\n"} {
		got := ValidateScript(in, "reel")
		if !hasProblem(got, "D1") || !HasFatal(got) {
			t.Errorf("empty script %q should be a fatal D1, got %v", in, problemIDs(got))
		}
	}
}

func TestValidateScriptMissingSections(t *testing.T) {
	noHook := strings.Replace(goodScript, "## Hook (first 3 seconds)", "## Opening", 1)
	if got := ValidateScript(noHook, "reel"); !hasProblem(got, "D1") || !HasFatal(got) {
		t.Errorf("a script with no Hook heading should be a fatal D1, got %v", problemIDs(got))
	}

	noCTA := goodScript[:strings.Index(goodScript, "## CTA")]
	if got := ValidateScript(noCTA, "reel"); !hasProblem(got, "D7") || !HasFatal(got) {
		t.Errorf("a script with no CTA should be a fatal D7, got %v", problemIDs(got))
	}
}

func TestValidateScriptThroatClearing(t *testing.T) {
	openers := []string{
		`**VO:** "Hey guys, today I want to talk about cold brew."`,
		`**VO:** "Hi everyone! Let's look at cold brew."`,
		`**VO:** "In this video I'll show you cold brew."`,
		`**VO:** "So basically cold brew takes a while."`,
		`**VO:** "What's up! Cold brew time."`,
		`**VO:** "Welcome back to the channel."`,
	}
	for _, opener := range openers {
		s := strings.Replace(goodScript,
			`**VO:** "Eighteen hours for one glass of cold brew. Here's why we don't rush it."`,
			opener, 1)
		if got := ValidateScript(s, "reel"); !hasProblem(got, "D2") {
			t.Errorf("opener %q should trip D2, got %v", opener, problemIDs(got))
		}
	}
}

// D3 was removed: see the comment in ValidateScript. This locks the decision
// in, so a future change cannot quietly reintroduce a word-count proxy for
// "the subject arrives late" and start flagging good hooks again.
func TestNoRamblingOpeningCheck(t *testing.T) {
	// A real hook from dev that the six-second version wrongly flagged.
	good := strings.Replace(goodScript,
		`**VO:** "Eighteen hours for one glass of cold brew. Here's why we don't rush it."`,
		`**Spoken:** "Our second location cost 40% more to open than our first — and it had nothing to do with rent."`, 1)
	for _, p := range ValidateScript(good, "reel") {
		if p.ID == "D3" {
			t.Errorf("D3 is gone; judging a brisk opening belongs to the Phase 2 scorer: %s", p.Message)
		}
	}
}

// The check is anchored to the hook's opening so a script ABOUT greeting
// people is not flagged for containing a greeting.
func TestValidateScriptDoesNotFlagGreetingsInTheBody(t *testing.T) {
	s := strings.Replace(goodScript,
		`**VO:** "Hot water is fast, but it drags out the burnt notes too."`,
		`**VO:** "Our baristas say hey everyone the moment you walk in."`, 1)
	if got := ValidateScript(s, "reel"); hasProblem(got, "D2") {
		t.Errorf("a greeting in the body is not throat-clearing, got %v", problemIDs(got))
	}
}

func TestValidateScriptEngagementBait(t *testing.T) {
	baits := []string{
		"Like this if you agree.",
		"Tag someone who needs this.",
		`Comment "YES" if you want the recipe.`,
		"Share if you love coffee.",
		"Double tap if you agree.",
		"Like and subscribe for more.",
	}
	for _, bait := range baits {
		s := strings.Replace(goodScript,
			`**VO:** "Send this to the one person who thinks they hate cold brew."`,
			`**VO:** "`+bait+`"`, 1)
		got := ValidateScript(s, "reel")
		if !hasProblem(got, "D8") {
			t.Errorf("bait %q should trip D8, got %v", bait, problemIDs(got))
		}
	}
}

func TestValidateScriptFollowForMoreIsAWarningNotACap(t *testing.T) {
	s := strings.Replace(goodScript,
		`**VO:** "Send this to the one person who thinks they hate cold brew."`,
		`**VO:** "Follow for more coffee tips."`, 1)
	got := ValidateScript(s, "reel")
	if !hasProblem(got, "D7b") {
		t.Fatalf("expected D7b, got %v", problemIDs(got))
	}
	for _, p := range got {
		if p.ID == "D7b" && p.Severity != severityWarn {
			t.Errorf("D7b should be a warning, got %v", p.Severity)
		}
	}
}

func TestValidateScriptCountsAsks(t *testing.T) {
	s := strings.Replace(goodScript,
		`**VO:** "Send this to the one person who thinks they hate cold brew."`,
		`**VO:** "Send this to a friend, save it for later, and follow us."`, 1)
	if got := ValidateScript(s, "reel"); !hasProblem(got, "D7") {
		t.Errorf("three asks should trip D7, got %v", problemIDs(got))
	}
}

func TestValidateScriptMutedReadability(t *testing.T) {
	s := goodScript
	for _, cue := range []string{
		`**On-screen text:** "18 hours for one cup."` + "\n",
		`**On-screen text:** "Heat pulls bitterness out fast."` + "\n",
		`**On-screen text:** "Cold water plus time."` + "\n",
		`**On-screen text:** "Smooth, never bitter."` + "\n",
	} {
		s = strings.Replace(s, cue, "", 1)
	}
	if got := ValidateScript(s, "reel"); !hasProblem(got, "D10") {
		t.Errorf("a script with no on-screen text cues should trip D10, got %v", problemIDs(got))
	}
}

func TestValidateScriptLengthBand(t *testing.T) {
	long := goodScript + strings.Repeat("\n**VO:** "+strings.Repeat("word ", 40), 8)
	if got := ValidateScript(long, "reel"); !hasProblem(got, "D13") {
		t.Errorf("a very long reel should trip D13, got %v", problemIDs(got))
	}

	// An unknown format is not guessed at.
	if got := ValidateScript(long, ""); hasProblem(got, "D13") {
		t.Error("length must not be checked when the format is unknown")
	}
	if got := ValidateScript(long, "carousel"); hasProblem(got, "D13") {
		t.Error("length must not be checked for a format with no band")
	}
}

// Production notes are not speech. Counting them would make every script read
// as twice its real length and fire the length warning constantly.
func TestSpokenWordsIgnoresProductionNotes(t *testing.T) {
	withNotes := `**VO:** "Three words here."
**Visual:** a long description of the shot that goes on and on and on and should not count
**On-screen text:** "more words that are not spoken aloud by anyone at all"
(a parenthetical stage direction nobody says out loud either)`
	if n := spokenWords(withNotes); n > 10 {
		t.Errorf("production notes leaked into the word count: got %d", n)
	}
}

func TestSeverityOrdering(t *testing.T) {
	// No hook and no CTA (both fatal) plus bait (cap) plus no on-screen text.
	s := "Some prose with no sections at all. Like this if you agree."
	got := ValidateScript(s, "reel")
	if len(got) < 2 {
		t.Fatalf("expected several problems, got %v", problemIDs(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Severity > got[i-1].Severity {
			t.Errorf("problems are not sorted most-severe-first: %v", problemIDs(got))
			break
		}
	}
}

func TestRepairInstructionSkipsWarnings(t *testing.T) {
	ps := []ScriptProblem{
		{ID: "D1", Severity: severityFatal, Message: "fatal thing"},
		{ID: "D10", Severity: severityWarn, Message: "warn thing"},
	}
	out := repairInstruction(ps)
	if !strings.Contains(out, "fatal thing") {
		t.Error("repair instruction should carry fatal problems")
	}
	if strings.Contains(out, "warn thing") {
		t.Error("repair instruction should not spend a turn on warnings")
	}
	if repairInstruction([]ScriptProblem{{ID: "D10", Severity: severityWarn, Message: "x"}}) != "" {
		t.Error("warnings alone should not produce a repair instruction")
	}
}

func TestClosesItsLoop(t *testing.T) {
	if got := ValidateScript(goodScript, "reel"); hasProblem(got, "D9") {
		t.Error("the good script closes its loop (cold brew, eighteen hours recur)")
	}

	unclosed := `## Hook
**VO:** "The unexpected ingredient nobody mentions in artisanal fermentation."

## Body
**Scene 1 (4s)**
**On-screen text:** "Something else entirely."
**VO:** "Let's talk about our opening hours instead."

## CTA
**VO:** "Send this to a friend."
`
	if got := ValidateScript(unclosed, "reel"); !hasProblem(got, "D9") {
		t.Errorf("a hook whose promise never recurs should trip D9, got %v", problemIDs(got))
	}
}

func TestScriptProblemMarshalsSeverityAsAName(t *testing.T) {
	b, err := json.Marshal(ScriptProblem{ID: "D1", Severity: severityFatal, Message: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"severity":"fatal"`) {
		t.Errorf("severity should marshal as its name, got %s", b)
	}
}

func TestEmptyCTASectionIsFatal(t *testing.T) {
	s := goodScript[:strings.Index(goodScript, "## CTA")] + "## CTA\n"
	got := ValidateScript(s, "reel")
	if !hasProblem(got, "D7") || !HasFatal(got) {
		t.Errorf("an empty CTA section should be a fatal D7, got %v", problemIDs(got))
	}
}

// Regression fixtures taken verbatim from real dev output. Every one of these
// was a FALSE POSITIVE the handcrafted fixtures above could not surface: the
// model wrote a good script and then talked to the user about it, and the
// checks read the conversation as if it were the script.

// The model correctly avoided engagement bait, then explained that it had —
// and got flagged for the phrase it was declining to use.
const scriptWithBaitInItsOwnExplanation = `Here is a draft.

---

## Hook (first 3 seconds)
**On-screen text + VO:** "Our second cafe cost 40% more than the first. The rent wasn't the reason."

## Body
**Scene 1 — the assumption (0:03-0:12)**
*Cue: you to camera, or B-roll of the new site mid-fit-out.*
VO: "We budgeted the second location like the first."

**Scene 2 — the turn (0:12-0:28)**
VO: "What blew the budget was people, not the lease."

---

## CTA
VO: "Curious what blindsided you on an expansion — the cost nobody warns you about?"

*Cue: end on you back in the finished space. Put any resource link in the first comment, not the post body.*

---

**Caption first line (second hook):** "The lease was the easy part."

Two notes:
- Kept it to **~55 seconds** — right for reaching beyond your followers on LinkedIn.
- The ask is an **opinion prompt**, not engagement bait — it invites a real reply, which drives the dwell/comment signal LinkedIn rewards without tripping its suppression of "comment YES" prompts.
`

func TestValidateScriptIgnoresBaitInsideCommentary(t *testing.T) {
	got := ValidateScript(scriptWithBaitInItsOwnExplanation, "video")
	if hasProblem(got, "D8") {
		t.Error("D8 fired on the phrase 'comment YES' inside the model's note about AVOIDING bait")
	}
	if hasProblem(got, "D7") {
		t.Errorf("D7 counted asks from the commentary rather than the CTA, got %v", problemIDs(got))
	}
}

// "Pin this" is a real ask on X. Leaving it out of the ask vocabulary made a
// perfectly good CTA look like no CTA at all.
func TestValidateScriptRecognisesPinAsAnAsk(t *testing.T) {
	s := strings.Replace(goodScript,
		`**VO:** "Send this to the one person who thinks they hate cold brew."`,
		`**VO:** "Pin this the next time someone says a cafe coffee is a rip-off."`, 1)
	for _, p := range ValidateScript(s, "reel") {
		if p.ID == "D7" {
			t.Errorf("'pin this' is an ask: %s", p.Message)
		}
	}
}

// Production notes and the trailing chat message are not speech. Counting them
// turned a ~20 second reel into "about 80 seconds" and fired a length warning
// on a script that was the right length.
func TestValidateScriptLengthIgnoresCommentary(t *testing.T) {
	padded := goodScript + "\n\n*Note: every line above is burned in, so the script works with sound off, and the first caption line should lead with the number rather than a thread emoji.*\n\n---\n\nWant me to draft the full thread version so it stands alone without the video, or tighten this to 30 seconds first?\n"
	if got := ValidateScript(padded, "reel"); hasProblem(got, "D13") {
		t.Errorf("commentary inflated the spoken-word count into a length warning, got %v", problemIDs(got))
	}
}

// Over-stripping would be as bad as under-stripping: a real script must
// survive the cleaner intact.
func TestStripCommentaryKeepsTheScript(t *testing.T) {
	out := stripCommentary(goodScript)
	for _, must := range []string{"## Hook", "## Body", "## CTA", "Eighteen hours", "Send this to"} {
		if !strings.Contains(out, must) {
			t.Errorf("stripCommentary removed %q from a clean script", must)
		}
	}
	// Bold production labels are structure, not commentary.
	if !strings.Contains(out, "**VO:**") {
		t.Error("stripCommentary removed a bold VO label")
	}
}

func TestStripCommentaryDoesNotEmptyAShortScript(t *testing.T) {
	// A leading rule must not truncate everything after it.
	s := "---\n" + goodScript
	if out := stripCommentary(s); !strings.Contains(out, "## CTA") {
		t.Error("a leading horizontal rule truncated the whole script")
	}
}

// Real scripts the model produced on dev, kept as testdata.
//
// Every one was read and judged good by hand, so ANY problem reported against
// them is a false positive. They are here because handcrafted fixtures missed
// four rounds of bugs in a row: a real generation is wrapped in commentary and
// written in whichever of about fourteen markdown dialects the model felt like
// that minute, and a clean invented script has neither.
func TestValidateScriptAgainstRealDevScripts(t *testing.T) {
	clean, _ := filepath.Glob("testdata/devscripts/clean/*.md")
	if len(clean) == 0 {
		t.Fatal("no clean corpus found")
	}
	for _, f := range clean {
		t.Run("clean/"+filepath.Base(f), func(t *testing.T) {
			for _, p := range ValidateScript(readScript(t, f), formatFor(f)) {
				t.Errorf("false positive [%s] %s: %s", p.Severity, p.ID, p.Message)
			}
		})
	}

	// The other direction matters just as much. Four rounds of this validator
	// were spent removing false positives, and a validator that has stopped
	// reporting anything at all would have passed every one of those rounds.
	flagged, _ := filepath.Glob("testdata/devscripts/flagged/*.md")
	if len(flagged) == 0 {
		t.Fatal("no flagged corpus found: without one, a validator that reports nothing looks perfect")
	}
	for _, f := range flagged {
		t.Run("flagged/"+filepath.Base(f), func(t *testing.T) {
			if len(ValidateScript(readScript(t, f), formatFor(f))) == 0 {
				t.Error("this script really does have a problem and the validator missed it")
			}
		})
	}
}

// A real generation that asks for a save AND a send — two different actions,
// which is two asks however confidently the model's own note claims one.
func TestValidateScriptCatchesTheRealTwoAskScript(t *testing.T) {
	f := "testdata/devscripts/flagged/two-asks-save-and-send-reel.md"
	var found bool
	for _, p := range ValidateScript(readScript(t, f), "reel") {
		if p.ID == "D7" && strings.Contains(p.Message, "2 separate asks") {
			found = true
		}
	}
	if !found {
		t.Error("want D7 reporting two asks")
	}
}

func readScript(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func formatFor(path string) string {
	if strings.Contains(path, "reel") || strings.Contains(path, "short") {
		return "reel"
	}
	return "video"
}

// The dialects seen on dev, each of which broke at least one check before
// normalisation. If the model invents another, add it here.
func TestNormaliseScriptHandlesEveryDialect(t *testing.T) {
	cases := []struct {
		line, wantBody       string
		wantSpoken, wantNote bool
	}{
		{`**VO:** "Said aloud."`, "Said aloud.", true, false},
		{`*VO:* "Said aloud."`, "Said aloud.", true, false},
		{`- *VO:* "Said aloud."`, "Said aloud.", true, false},
		{`**Spoken:** "Said aloud."`, "Said aloud.", true, false},
		{`**Line (on-screen + VO):** "Said aloud."`, "Said aloud.", true, false},
		{`**On-screen text:** "Read, not heard."`, "Read, not heard.", false, true},
		{`*On-screen:* "Read, not heard."`, "Read, not heard.", false, true},
		{`- *On-screen:* "Read, not heard."`, "Read, not heard.", false, true},
		{`**Visual:** A wide shot.`, "A wide shot.", false, true},
		{`(a bare stage direction)`, "(a bare stage direction)", false, true},
		{`Just a plain sentence.`, "Just a plain sentence.", false, false},
	}
	for _, c := range cases {
		got := normaliseScript(c.line)
		if len(got) != 1 {
			t.Errorf("%q produced %d lines", c.line, len(got))
			continue
		}
		if got[0].Body != c.wantBody {
			t.Errorf("%q body = %q, want %q", c.line, got[0].Body, c.wantBody)
		}
		if got[0].Spoken != c.wantSpoken || got[0].Note != c.wantNote {
			t.Errorf("%q spoken=%v note=%v, want spoken=%v note=%v",
				c.line, got[0].Spoken, got[0].Note, c.wantSpoken, c.wantNote)
		}
	}
}
