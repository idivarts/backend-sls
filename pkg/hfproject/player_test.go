package hfproject

import (
	"strings"
	"testing"
)

// The bug this whole file exists for. Their dev server injects the runtime
// into the HTML it serves; static files get no such treatment, so without
// this the composition comes up with window.__timelines undefined, every
// scene at its CSS default, and seeking does nothing.
func TestWithRuntimeAddsTheScript(t *testing.T) {
	out := withRuntime(`<html><head><title>x</title></head><body></body></html>`, "https://cdn/runtime.js")
	if !strings.Contains(out, `<script src="https://cdn/runtime.js"></script>`) {
		t.Fatalf("no runtime tag: %s", out)
	}
}

// Before anything that uses it: the composition's own script registers its
// timeline on window.__timelines, which the runtime creates. Put the runtime
// after that and the page throws on load.
func TestWithRuntimeGoesFirst(t *testing.T) {
	src := `<html><head></head><body><script>window.__timelines["main"]=gsap.timeline()</script></body></html>`
	out := withRuntime(src, "https://cdn/runtime.js")
	if strings.Index(out, "runtime.js") > strings.Index(out, "__timelines") {
		t.Fatalf("the runtime loads after the script that needs it: %s", out)
	}
}

func TestWithRuntimeFallsBackToBodyThenPrepend(t *testing.T) {
	out := withRuntime(`<body><script>gsap.timeline()</script></body>`, "https://cdn/runtime.js")
	if !strings.Contains(out, "runtime.js") || strings.Index(out, "runtime.js") > strings.Index(out, "gsap") {
		t.Fatalf("body fallback failed: %s", out)
	}
	bare := withRuntime(`<div>no head or body</div>`, "https://cdn/runtime.js")
	if !strings.HasPrefix(bare, `<script src="https://cdn/runtime.js">`) {
		t.Fatalf("prepend fallback failed: %s", bare)
	}
}

// index.html is what the RENDERER opens, and the CLI injects its own runtime
// there. The player's copy must be a separate file, or we would be meddling
// with the one path that currently works to fix a different one.
func TestWithRuntimeDoesNotMutateItsInput(t *testing.T) {
	src := `<html><head></head><body></body></html>`
	_ = withRuntime(src, "https://cdn/runtime.js")
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
