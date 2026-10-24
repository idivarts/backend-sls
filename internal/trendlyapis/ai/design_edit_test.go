package ai

import (
	"strings"
	"testing"
)

const editDoc = `<div data-carousel>
<section data-slide="0" data-duration="3000" class="scene">
  <h1 data-el="hook" style="font-size:72px">Old headline</h1>
</section>
<section data-slide="1" data-duration="3000" class="scene">
  <p data-el="cta" style="font-size:40px">Shop now</p>
</section>
</div>`

func TestApplySnippetEdits_SingleReplacement(t *testing.T) {
	got, err := applySnippetEdits(editDoc, []designEdit{
		{OldText: "Old headline", NewText: "New headline"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "New headline") {
		t.Error("replacement text missing")
	}
	if strings.Contains(got, "Old headline") {
		t.Error("original text survived")
	}
	// Everything else must be untouched — that is the whole point of the change.
	if !strings.Contains(got, `data-el="cta"`) || !strings.Contains(got, `data-duration="3000"`) {
		t.Error("an unrelated part of the document was lost")
	}
}

func TestApplySnippetEdits_AppliedInOrder(t *testing.T) {
	got, err := applySnippetEdits(editDoc, []designEdit{
		{OldText: "Old headline", NewText: "Interim"},
		{OldText: "Interim", NewText: "Final"},
		{OldText: "Shop now", NewText: "Buy now"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "Final") || !strings.Contains(got, "Buy now") {
		t.Errorf("edits did not all apply: %s", got)
	}
}

func TestApplySnippetEdits_RejectsAmbiguousAnchor(t *testing.T) {
	// class="scene" appears on both sections; replacing the first silently is
	// exactly the failure this guard exists to prevent.
	_, err := applySnippetEdits(editDoc, []designEdit{
		{OldText: `class="scene"`, NewText: `class="scene dark"`},
	})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	if !strings.Contains(err.Error(), "appears 2 times") {
		t.Errorf("error should say how many times it matched, got: %v", err)
	}
}

func TestApplySnippetEdits_RejectsMissingAnchor(t *testing.T) {
	_, err := applySnippetEdits(editDoc, []designEdit{
		{OldText: "text that is not there", NewText: "x"},
	})
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if !strings.Contains(err.Error(), "not found verbatim") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApplySnippetEdits_AllOrNothing(t *testing.T) {
	// The second edit is bad. The first must not be observable in the result,
	// because the caller discards the document on error and persists nothing.
	got, err := applySnippetEdits(editDoc, []designEdit{
		{OldText: "Old headline", NewText: "New headline"},
		{OldText: "missing anchor", NewText: "x"},
	})
	if err == nil {
		t.Fatal("expected an error from the second edit")
	}
	if got != "" {
		t.Error("a failed batch must return no document")
	}
}

func TestApplySnippetEdits_EmptyNewTextDeletes(t *testing.T) {
	got, err := applySnippetEdits(editDoc, []designEdit{
		{OldText: ` style="font-size:40px"`, NewText: ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "font-size:40px") {
		t.Error("snippet was not deleted")
	}
	if !strings.Contains(got, `data-el="cta"`) {
		t.Error("deletion took more than it should have")
	}
}

func TestApplySnippetEdits_RejectsEmptyAnchor(t *testing.T) {
	if _, err := applySnippetEdits(editDoc, []designEdit{{OldText: "", NewText: "x"}}); err == nil {
		t.Fatal("an empty oldText must be refused, not matched everywhere")
	}
}

func TestApplySnippetEdits_NoEditsIsIdentity(t *testing.T) {
	got, err := applySnippetEdits(editDoc, nil)
	if err != nil || got != editDoc {
		t.Fatalf("no edits should return the document unchanged, got err=%v", err)
	}
}
