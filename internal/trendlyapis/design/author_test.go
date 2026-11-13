package design

import (
	"strings"
	"testing"

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
