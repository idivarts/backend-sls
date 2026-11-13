package design

import (
	"strings"
	"testing"
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
