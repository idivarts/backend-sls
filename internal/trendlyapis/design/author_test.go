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

func TestParseAcceptsBareJSON(t *testing.T) {
	p, err := parseAuthoredProject(`{"files":[{"path":"index.html","content":"<html></html>"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Files) != 1 || p.Files[0].Path != "index.html" {
		t.Fatalf("parsed wrong: %+v", p.Files)
	}
}

func TestParseAcceptsAFencedBlock(t *testing.T) {
	raw := "Here is the project:\n\n```json\n{\"files\":[{\"path\":\"index.html\",\"content\":\"<html></html>\"}]}\n```\n"
	p, err := parseAuthoredProject(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Files) != 1 {
		t.Fatalf("parsed wrong: %+v", p.Files)
	}
}

func TestParseAcceptsAnUnlabelledFence(t *testing.T) {
	raw := "```\n{\"files\":[{\"path\":\"a.html\",\"content\":\"x\"}]}\n```"
	if _, err := parseAuthoredProject(raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsEmptyAndProse(t *testing.T) {
	for _, raw := range []string{"", "   ", "I can't do that."} {
		if _, err := parseAuthoredProject(raw); err == nil {
			t.Fatalf("%q was accepted", raw)
		}
	}
}

func TestParseRejectsAProjectWithNoFiles(t *testing.T) {
	_, err := parseAuthoredProject(`{"files":[]}`)
	if err == nil || !strings.Contains(err.Error(), "no files") {
		t.Fatalf("expected a no-files complaint, got %v", err)
	}
}

// Every failure message is read by a user verbatim, so each must be a
// sentence rather than a developer's shorthand.
func TestParseFailuresReadAsSentences(t *testing.T) {
	for _, raw := range []string{"", "nope", `{"files":[]}`, "{not json}"} {
		_, err := parseAuthoredProject(raw)
		if err == nil {
			continue
		}
		m := err.Error()
		if m == "" || m[len(m)-1] != '.' || strings.ToUpper(m[:1]) != m[:1] {
			t.Fatalf("message %q does not read as a sentence", m)
		}
	}
}

// A truncated answer must not be reported as a bad one. The model ran out of
// room mid-JSON, so the project is unparseable for a reason the user can act
// on — a shorter film — and telling them their answer "could not be read"
// sends them to rewrite a brief that was never the problem.
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
func TestParseRefusesTruncatedJSON(t *testing.T) {
	_, err := parseAuthoredProject(`{"files":[{"path":"index.html","content":"<html><body`)
	if err == nil {
		t.Fatal("half a JSON document parsed as a project")
	}
}
