package hfproject

import (
	"strings"
	"testing"
)

func idx(extra string) File {
	return File{Path: "index.html", Content: `<html><body><div id="root" data-composition-id="main" data-start="0" data-duration="12" data-width="1080" data-height="1920">` + extra + `</div></body></html>`}
}

func TestBuildAcceptsAMinimalProject(t *testing.T) {
	p, err := Build([]File{idx("")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.DurationMs != 12000 {
		t.Fatalf("DurationMs = %d, want 12000", p.DurationMs)
	}
}

func TestBuildRequiresAnIndex(t *testing.T) {
	_, err := Build([]File{{Path: "compositions/hook.html", Content: "<template></template>"}})
	if err == nil || !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("expected a complaint about index.html, got %v", err)
	}
}

// The root's duration is the film's length and the wallet's input. A scene's
// duration is not, and taking one would both mis-charge and cut the film.
func TestBuildReadsTheROOTDurationNotASceneDuration(t *testing.T) {
	root := File{Path: "index.html", Content: `
<html><body>
  <div id="root" data-composition-id="main" data-start="0" data-duration="20" data-width="1080" data-height="1920">
    <div id="hook" data-composition-id="hook" data-composition-src="compositions/hook.html"
         data-start="0" data-duration="3" data-width="1080" data-height="1920"></div>
  </div>
</body></html>`}
	scene := File{Path: "compositions/hook.html", Content: "<template></template>"}
	p, err := Build([]File{root, scene})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.DurationMs != 20000 {
		t.Fatalf("DurationMs = %d, want 20000 (the root), not 3000 (the first scene)", p.DurationMs)
	}
}

func TestBuildRejectsAMissingDuration(t *testing.T) {
	_, err := Build([]File{{Path: "index.html", Content: `<div id="root" data-composition-id="main"></div>`}})
	if err == nil || !strings.Contains(err.Error(), "data-duration") {
		t.Fatalf("expected a complaint about data-duration, got %v", err)
	}
}

// A mounted scene that is not in the project does not fail in the CLI — it
// waits 45 seconds and renders static frames, which reads as a rendering
// problem rather than a missing file.
func TestBuildRejectsAMountedSceneThatIsNotThere(t *testing.T) {
	_, err := Build([]File{idx(`<div data-composition-src="compositions/missing.html"></div>`)})
	if err == nil || !strings.Contains(err.Error(), "compositions/missing.html") {
		t.Fatalf("expected a complaint naming the missing scene, got %v", err)
	}
}

func TestBuildAcceptsAMountedSceneThatIsThere(t *testing.T) {
	_, err := Build([]File{
		idx(`<div data-composition-src="compositions/hook.html"></div>`),
		{Path: "compositions/hook.html", Content: "<template></template>"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildRejectsPathsThatEscapeTheProject(t *testing.T) {
	for _, bad := range []string{"../secrets.txt", "/etc/passwd", "compositions\\hook.html", "", "   "} {
		_, err := Build([]File{idx(""), {Path: bad, Content: "x"}})
		if err == nil {
			t.Fatalf("path %q was accepted", bad)
		}
	}
}

func TestBuildRejectsDuplicatePaths(t *testing.T) {
	_, err := Build([]File{idx(""), {Path: "a.html", Content: "1"}, {Path: "./a.html", Content: "2"}})
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("expected a duplicate-path complaint, got %v", err)
	}
}

// A model that loops must not be able to fill the bucket; the failure would
// look like a slow render rather than what it is.
func TestBuildBoundsSizeAndCount(t *testing.T) {
	big := []File{idx(""), {Path: "big.html", Content: strings.Repeat("x", MaxBytes+1)}}
	if _, err := Build(big); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("expected a size limit, got %v", err)
	}

	many := []File{idx("")}
	for i := 0; i < MaxFiles; i++ {
		many = append(many, File{Path: "f" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".html", Content: "x"})
	}
	if _, err := Build(many); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("expected a file-count limit, got %v", err)
	}
}

func TestPrefixIsScopedToBrandAndDesign(t *testing.T) {
	got := Prefix("b1", "d1")
	if got != "design-projects/b1/d1" {
		t.Fatalf("Prefix = %q", got)
	}
	// The render task role is scoped to design-projects/* — a prefix outside
	// it would be unreadable by the worker, with a 403 at render time.
	if !strings.HasPrefix(got, "design-projects/") {
		t.Fatalf("prefix must stay under design-projects/, got %q", got)
	}
}
