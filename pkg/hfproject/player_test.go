package hfproject

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bug this whole file exists for. Their dev server injects the runtime
// into the HTML it serves; static files get no such treatment, so without
// this the composition comes up with window.__timelines undefined, every
// scene at its CSS default, and seeking does nothing.
func TestWithScriptsAddsTheScript(t *testing.T) {
	out := withScripts(`<html><head><title>x</title></head><body></body></html>`, "https://cdn/runtime.js", "https://cdn/gsap.js")
	if !strings.Contains(out, `<script src="https://cdn/runtime.js"></script>`) {
		t.Fatalf("no runtime tag: %s", out)
	}
}

// Before anything that uses it: the composition's own script registers its
// timeline on window.__timelines, which the runtime creates. Put the runtime
// after that and the page throws on load.
func TestWithScriptsGoesFirst(t *testing.T) {
	src := `<html><head></head><body><script>window.__timelines["main"]=gsap.timeline()</script></body></html>`
	out := withScripts(src, "https://cdn/runtime.js", "https://cdn/gsap.js")
	if strings.Index(out, "runtime.js") > strings.Index(out, "__timelines") {
		t.Fatalf("the runtime loads after the script that needs it: %s", out)
	}
}

func TestWithScriptsFallsBackToBodyThenPrepend(t *testing.T) {
	out := withScripts(`<body><script>gsap.timeline()</script></body>`, "https://cdn/runtime.js", "https://cdn/gsap.js")
	if !strings.Contains(out, "runtime.js") || strings.Index(out, "runtime.js") > strings.Index(out, "gsap") {
		t.Fatalf("body fallback failed: %s", out)
	}
	bare := withScripts(`<div>no head or body</div>`, "https://cdn/runtime.js", "https://cdn/gsap.js")
	if !strings.HasPrefix(bare, `<script src="https://cdn/runtime.js">`) {
		t.Fatalf("prepend fallback failed: %s", bare)
	}
}

// index.html is what the RENDERER opens, and the CLI injects its own runtime
// there. The player's copy must be a separate file, or we would be meddling
// with the one path that currently works to fix a different one.
func TestWithScriptsDoesNotMutateItsInput(t *testing.T) {
	src := `<html><head></head><body></body></html>`
	_ = withScripts(src, "https://cdn/runtime.js", "https://cdn/gsap.js")
	if strings.Contains(src, "runtime.js") {
		t.Fatal("the caller's index.html was modified in place")
	}
}

func TestPlayerPagePointsAtThePreviewCopy(t *testing.T) {
	page := playerPage("https://cdn/player.js")
	if !strings.Contains(page, `src="preview.html"`) {
		t.Fatal("the player must play preview.html, which carries the runtime, not index.html")
	}
	if !strings.Contains(page, `<script src="https://cdn/player.js"></script>`) {
		t.Fatal("the player script is missing")
	}
}

// Versioned so an upgrade cannot change what an existing design plays.
func TestSharedAssetKeyIsVersioned(t *testing.T) {
	k := sharedAssetKey("runtime.js")
	if !strings.Contains(k, playerAssetVersion) || !strings.HasSuffix(k, "runtime.js") {
		t.Fatalf("unexpected key %q", k)
	}
}

// 776 KB of JavaScript that has to actually be in the binary.
func TestTheAssetsAreEmbedded(t *testing.T) {
	if len(playerJS) < 100_000 {
		t.Fatalf("player.js looks wrong: %d bytes", len(playerJS))
	}
	if len(runtimeJS) < 500_000 {
		t.Fatalf("runtime.js looks wrong: %d bytes", len(runtimeJS))
	}
}

// The blank-player bug. The composition calls gsap.timeline() and the project
// as authored loads GSAP nowhere — the renderer patches its own temp copy, so
// renders worked while the player showed an empty black frame.
func TestWithScriptsAddsGsapBeforeTheCompositionScript(t *testing.T) {
	src := `<html><head></head><body><script>const tl = gsap.timeline({paused:true});</script></body></html>`
	out := withScripts(src, "https://cdn/runtime.js", "https://cdn/gsap.js")
	if !strings.Contains(out, `<script src="https://cdn/gsap.js"></script>`) {
		t.Fatalf("no gsap tag: %s", out)
	}
	if strings.Index(out, "gsap.js") > strings.Index(out, "gsap.timeline") {
		t.Fatalf("gsap loads after the script that needs it: %s", out)
	}
}

// Two copies of GSAP on one page is a second global clobbering the first, and
// ours would load last. A project that already brings its own is left alone.
func TestWithScriptsLeavesAnExistingGsapAlone(t *testing.T) {
	for _, existing := range []string{
		`<script src="https://cdn.jsdelivr.net/npm/gsap@3.15.0/dist/gsap.min.js"></script>`,
		`<script src="assets/vendor/gsap-3.15.0.min.js"></script>`,
	} {
		out := withScripts(`<html><head>`+existing+`</head><body></body></html>`,
			"https://cdn/runtime.js", "https://cdn/gsap.js")
		if strings.Contains(out, "https://cdn/gsap.js") {
			t.Fatalf("added a second GSAP next to %s:\n%s", existing, out)
		}
		if !strings.Contains(out, "runtime.js") {
			t.Fatalf("dropped the runtime: %s", out)
		}
	}
}

// The renderer vendors its own GSAP and the player vendors this one. They must
// be the same bytes, or a film plays one way and renders another — the exact
// class of bug this whole change is fixing.
func TestVendoredGsapMatchesTheRenderWorkersCopy(t *testing.T) {
	theirs, err := os.ReadFile(filepath.Join("..", "..", "services", "render-worker", "vendor", "gsap-3.15.0.min.js"))
	if err != nil {
		t.Fatalf("cannot read the render worker's GSAP: %v", err)
	}
	if !bytes.Equal(theirs, gsapJS) {
		t.Fatalf("the player's GSAP (%d bytes) is not the renderer's (%d bytes)", len(gsapJS), len(theirs))
	}
}
