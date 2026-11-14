package design

import (
	"strings"
	"testing"
)

// The click that reaches the root: the player's bridge resolves through
// closest("[id]"), so clicking the film's background selects the element
// carrying data-composition-id.
func TestTheRootCompositionCannotBeEdited(t *testing.T) {
	for _, req := range []editElementRequest{
		{ElementID: "root", Text: str("gone")},
		{ElementID: "root", Style: map[string]string{"color": "#fff"}},
		{ElementID: "root", StartMs: num(0)},
	} {
		_, err := applyElementEdit(filmHTML, req)
		if err == nil {
			t.Fatalf("accepted an edit to the root composition: %+v", req)
		}
		if !strings.Contains(err.Error(), "whole film") {
			t.Fatalf("error does not say why: %v", err)
		}
	}
}

// Replacing the text of a container replaces everything in it. On a
// headline that is the point; on a scene it is the scene gone.
func TestTextCannotBeSetOnAGroupOfElements(t *testing.T) {
	_, err := applyElementEdit(filmHTML, editElementRequest{ElementID: "s1", Text: str("flat")})
	if err == nil {
		t.Fatal("accepted a text edit on a whole scene")
	}
	if !strings.Contains(err.Error(), "no words of its own") {
		t.Fatalf("error does not say why: %v", err)
	}
}

// …but its colour and its timing are perfectly reasonable things to change.
func TestAGroupCanStillBeStyledAndRetimed(t *testing.T) {
	out, err := applyElementEdit(filmHTML, editElementRequest{
		ElementID: "s1",
		Style:     map[string]string{"background": "#054463"},
		StartMs:   num(500),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "background:#054463") || !strings.Contains(out, `data-start="0.5"`) {
		t.Fatalf("the edit did not land:\n%s", out)
	}
	// And its children are untouched.
	if !strings.Contains(out, "Hit the 2 PM wall?") || !strings.Contains(out, "Cold brew, no sugar") {
		t.Fatal("styling a scene lost its contents")
	}
}

func TestAReplyCarryingAScriptOrAHandlerIsRefused(t *testing.T) {
	bad := map[string]string{
		`<h1 id="title" onclick="fetch('/x')">A</h1>`:        "an inline handler",
		`<h1 id="title" ONERROR="x()">A</h1>`:                "a shouty handler",
		`<h1 id="title">A<script>x()</script></h1>`:          "a script child",
		`<h1 id="title"><a href="javascript:x()">A</a></h1>`: "a javascript url",
	}
	for reply, why := range bad {
		if _, err := parseElementReply(reply, "title", "h1"); err == nil {
			t.Fatalf("accepted %q (%s)", reply, why)
		}
	}
	// A data attribute that merely starts with "on" is not a handler.
	if _, err := parseElementReply(`<h1 id="title" data-online="yes">A</h1>`, "title", "h1"); err != nil {
		t.Fatalf("refused an ordinary attribute: %v", err)
	}
}
