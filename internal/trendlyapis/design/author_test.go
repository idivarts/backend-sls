package design

import (
	"strings"
	"testing"
	"time"

	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// Models fence JSON and add a sentence before it however firmly they are told
// not to. Refusing a correct project over its wrapper would waste a
// frontier-model call and tell the user their design was bad, so the parser
// is tolerant of the wrapper and strict about the contents.

func TestTruncationIsItsOwnFailure(t *testing.T) {
	resp := &openrouter.ChatResponse{
		Choices: []openrouter.Choice{{FinishReason: "length"}},
	}
	if finishReason(resp) != "length" {
		t.Fatalf("finishReason = %q, want length", finishReason(resp))
	}
	// And a normal stop is not mistaken for one.
	ok := &openrouter.ChatResponse{Choices: []openrouter.Choice{{FinishReason: "stop"}}}
	if finishReason(ok) == "length" {
		t.Fatal("a completed answer was read as truncated")
	}
	if finishReason(nil) != "" || finishReason(&openrouter.ChatResponse{}) != "" {
		t.Fatal("a missing response should have no finish reason, not panic")
	}
}

// The snippets are the only evidence of a parse failure once the answer is
// gone. The TAIL is where truncation shows and the HEAD is where a refusal or
// a stray preamble does, so both are kept and neither may panic on a short
// answer.
func TestSnippetsSurviveShortAnswers(t *testing.T) {
	if got := snippet("abc", 240); got != "abc" {
		t.Fatalf("snippet shortened a short string: %q", got)
	}
	if got := tailSnippet("abc", 240); got != "abc" {
		t.Fatalf("tailSnippet shortened a short string: %q", got)
	}
	if got := snippet("abcdef", 3); got != "abc" {
		t.Fatalf("snippet = %q, want the head", got)
	}
	if got := tailSnippet("abcdef", 3); got != "def" {
		t.Fatalf("tailSnippet = %q, want the tail", got)
	}
	if snippet("", 10) != "" || tailSnippet("", 10) != "" {
		t.Fatal("empty answers should snippet to empty")
	}
}

// Truncated JSON is exactly what a cut-off answer looks like, and the parser
// must refuse it rather than return half a project.

const goodIndex = `<html><body>
  <div id="root" data-composition-id="main" data-start="0" data-duration="8" data-width="1080" data-height="1920">
    <section id="hook" data-scene="hook" data-start="0" data-duration="8"></section>
  </div>
  <script>window.__timelines["main"] = gsap.timeline({ paused: true });</script>
</body></html>`

func TestParseAcceptsABareDocument(t *testing.T) {
	got, err := parseAuthoredIndex(goodIndex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "<html") || !strings.HasSuffix(got, "</html>") {
		t.Fatalf("the document was mangled: %q", cut(got, 80))
	}
}

// Models fence code and add a sentence however firmly they are told not to.
// Refusing a correct document over its wrapper would waste a frontier call
// and tell the user their design was bad.
func TestParseStripsAFenceAndPreamble(t *testing.T) {
	for _, raw := range []string{
		"Here is the film:\n\n```html\n" + goodIndex + "\n```",
		"```\n" + goodIndex + "\n```",
		"Sure!\n" + goodIndex + "\nLet me know what you think.",
	} {
		got, err := parseAuthoredIndex(raw)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", cut(raw, 40), err)
		}
		if !strings.Contains(got, "data-composition-id") || strings.Contains(got, "```") {
			t.Fatalf("not cleanly extracted: %q", cut(got, 80))
		}
	}
}

// The whole reason the JSON wrapper went. A 9 KB answer of HTML, CSS and JS
// died on one backslash that is legal in a CSS content property or a JS regex
// and illegal in JSON — finish_reason "stop", braces balanced, shape right,
// and unreadable. As HTML there is nothing to escape.
func TestParseAcceptsContentThatWouldBreakJSONEscaping(t *testing.T) {
	awkward := `<html><body>
  <style>.q::before { content: "\201C"; }</style>
  <div id="root" data-composition-id="main" data-start="0" data-duration="5" data-width="1080" data-height="1920"></div>
  <script>const re = /\d+\s*"x"/g; window.__timelines["main"] = gsap.timeline({ paused: true });</script>
</body></html>`
	got, err := parseAuthoredIndex(awkward)
	if err != nil {
		t.Fatalf("content that only JSON would choke on was refused: %v", err)
	}
	if !strings.Contains(got, `content: "\201C"`) || !strings.Contains(got, `/\d+\s*"x"/g`) {
		t.Fatal("the awkward sequences did not survive extraction")
	}
}

func TestParseRejectsEmptyAndProse(t *testing.T) {
	for _, raw := range []string{"", "   ", "I could not write that design."} {
		if _, err := parseAuthoredIndex(raw); err == nil {
			t.Fatalf("expected a refusal for %q", raw)
		}
	}
}

// Prose that merely contains a bracket is not a film. Without this the real
// validator's errors would be about having been handed a sentence.
func TestParseRejectsSomethingWithoutARootComposition(t *testing.T) {
	if _, err := parseAuthoredIndex("<p>I think a reel would work well here</p>"); err == nil {
		t.Fatal("a document with no root composition was accepted")
	}
}

func TestParseFailuresReadAsSentences(t *testing.T) {
	for _, raw := range []string{"", "no document here", "<p>hello</p>"} {
		_, err := parseAuthoredIndex(raw)
		if err == nil {
			continue
		}
		m := err.Error()
		if m == "" || m[len(m)-1] != '.' || strings.ToUpper(m[:1]) != m[:1] {
			t.Fatalf("a user reads this verbatim; %q is not a sentence", m)
		}
	}
}

// cut shortens a string for a failure message.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ── The automatic rewrite ─────────────────────────────────────────────────
//
// Roughly half the designs written against a real brief came back refused by
// Hyperframes' gate — a contrast ratio a few tenths under the bar, two text
// blocks overlapping — and the user was handed a finished-looking film with an
// error under it. The job now reads the refusal and writes the film again. The
// tests below pin the parts of that which are cheap to get wrong and expensive
// to notice: what the model is told, and how long it is allowed to take.

func TestFirstAttemptSendsTheBriefAlone(t *testing.T) {
	a := &authorRun{brief: "a 9s reel about cold brew", attempt: 1}
	if got := a.userMessage(); got != "a 9s reel about cold brew" {
		t.Fatalf("the first attempt added something to the brief: %q", got)
	}
}

// A rewrite is a REPAIR, not a fresh roll of the dice: the model needs its own
// document back, or it discards a film that was wrong in one measurable way
// and returns one wrong in a different way.
func TestARewriteCarriesTheDocumentAndEveryFinding(t *testing.T) {
	a := &authorRun{
		brief:    "a 9s reel about cold brew",
		attempt:  2,
		previous: "<html data-composition-id=\"main\">THE OLD FILM</html>",
		fix: []string{
			"index.html: Contrast is 2.83:1; WCAG AA requires 3:1.",
			"index.html: Two text blocks overlap and may render unreadable.",
		},
	}
	got := a.userMessage()

	for _, want := range []string{
		"a 9s reel about cold brew",
		"THE OLD FILM",
		"1. index.html: Contrast is 2.83:1; WCAG AA requires 3:1.",
		"2. index.html: Two text blocks overlap and may render unreadable.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("the rewrite prompt is missing %q:\n%s", want, got)
		}
	}
	// Told to keep the film it already is. Without this the repair drifts:
	// the gate passes and the user gets a different video than the one they
	// were looking at.
	if !strings.Contains(got, "Keep the film it already is") {
		t.Fatal("the rewrite prompt does not ask for a minimal change")
	}
	// And still asks for the same answer shape the parser expects.
	if !strings.Contains(got, "complete corrected index.html") {
		t.Fatal("the rewrite prompt does not ask for the whole document")
	}
}

// Half a rewrite is a dice roll with extra steps. Either both the findings and
// the document that produced them are in hand, or this is a first attempt.
func TestARewriteNeedsBothHalvesOrItIsNotOne(t *testing.T) {
	onlyFindings := &authorRun{brief: "brief", attempt: 2, fix: []string{"Contrast is 2.83:1."}}
	if got := onlyFindings.userMessage(); got != "brief" {
		t.Fatalf("findings with no document produced a rewrite prompt: %q", got)
	}
	onlyDocument := &authorRun{brief: "brief", attempt: 2, previous: "<html></html>"}
	if got := onlyDocument.userMessage(); got != "brief" {
		t.Fatalf("a document with no findings produced a rewrite prompt: %q", got)
	}
}

// The clamp is what keeps the loop inside the Lambda. studio_sqs stops at 900s
// and the queue redelivers at 960s, so a rewrite started with four minutes
// left must not sit in a ten-minute call.
func TestGenerationTimeoutNeverExceedsWhatIsLeft(t *testing.T) {
	cases := []struct {
		name      string
		remaining time.Duration
		want      time.Duration
	}{
		{"a fresh run gets the full call", authorLoopBudget, maxGenerationTime},
		{"a late rewrite is clamped", 5 * time.Minute, 5 * time.Minute},
		{"exactly the cap is the cap", maxGenerationTime, maxGenerationTime},
		// ⚠️ A spent budget must fail SMALL. Reading a non-positive remainder
		// as "no constraint" handed the full ten minutes to the one case that
		// most needed a short call, and twenty-two minutes of run against a
		// fifteen-minute Lambda leaves the design at "writing" for ever.
		{"an overshot budget is floored, not unlimited", -time.Minute, minGenerationTime},
		{"an unset budget is floored, not unlimited", 0, minGenerationTime},
		{"a sliver is floored rather than cut off mid-document", 10 * time.Second, minGenerationTime},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &authorRun{remaining: c.remaining}
			got := a.generationTimeout()
			if got != c.want {
				t.Fatalf("generationTimeout() = %s, want %s", got, c.want)
			}
			if got <= 0 {
				t.Fatal("a non-positive timeout would fail the call before it is made")
			}
			if got > maxGenerationTime {
				t.Fatalf("generationTimeout() = %s, over the cap %s", got, maxGenerationTime)
			}
		})
	}
}

// The whole run has to finish inside the Lambda, or the design is stranded:
// studio_sqs stops at 900s, SQS redelivers at 960s, and the redelivery's
// idempotency guard sees "writing" and skips — nobody finishes the rewrite.
//
// This is arithmetic on the constants rather than a comment claiming they fit,
// because the first version of them did not: minRetryBudget was the same four
// minutes as prepareWait, so a rewrite could clear the check with exactly
// enough budget to wait and none left to write.
func TestTheLoopCannotOutliveTheLambda(t *testing.T) {
	const lambdaTimeout = 15 * time.Minute
	// Upload, revision commit and dispatch, per attempt. Generous.
	const overhead = 30 * time.Second

	if maxAuthorAttempts < 2 {
		t.Fatal("with fewer than two attempts there is no rewrite at all")
	}
	if minRetryBudget <= prepareWait {
		t.Fatalf("a retry can begin with %s, all of which the %s wait can eat",
			minRetryBudget, prepareWait)
	}
	// What is left when the wait has taken its share still has to be worth a
	// call — otherwise the floor in generationTimeout is what overshoots.
	if left := minRetryBudget - prepareWait; left < minGenerationTime {
		t.Fatalf("a retry begun at the limit leaves %s for a call that needs %s", left, minGenerationTime)
	}

	// The worst case the loop permits: the first attempt runs right up to the
	// point where a retry is still allowed, the wait takes all of prepareWait,
	// and the rewrite takes everything the clamp then gives it.
	firstAttempt := authorLoopBudget - minRetryBudget
	rewrite := minRetryBudget - prepareWait
	worst := firstAttempt + overhead + prepareWait + rewrite + overhead
	if worst >= lambdaTimeout {
		t.Fatalf("the worst run takes %s against a %s Lambda", worst, lambdaTimeout)
	}
}

// Three versions all labelled with the same brief tell the user nothing about
// why there are three.
func TestARewriteSaysSoInTheRevisionList(t *testing.T) {
	first := &authorRun{brief: "a 9s reel about cold brew", attempt: 1}
	if got := first.revisionNote(); got != "a 9s reel about cold brew" {
		t.Fatalf("the first version was labelled as a fix: %q", got)
	}
	second := &authorRun{brief: "a 9s reel about cold brew", attempt: 2}
	if got := second.revisionNote(); !strings.HasPrefix(got, "Fix 1 ·") {
		t.Fatalf("the first rewrite is not labelled as one: %q", got)
	}
	third := &authorRun{brief: "a 9s reel about cold brew", attempt: 3}
	if got := third.revisionNote(); !strings.HasPrefix(got, "Fix 2 ·") {
		t.Fatalf("the second rewrite is not labelled as one: %q", got)
	}
}
