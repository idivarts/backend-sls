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

// The app and the player are on different origins, so the app can only drive
// the film through this. Each assertion is a way the bridge has failed before
// it reaches a browser.
func TestPlayerPageCarriesTheBridge(t *testing.T) {
	page := playerPage("https://cdn/player.js")

	// The player must be defined before the bridge queries for it.
	if strings.Index(page, "player.js") > strings.Index(page, "querySelector") {
		t.Fatal("the bridge runs before the player script that defines the element")
	}
	for _, want := range []string{
		`d.type === "seek"`, // the command this exists for
		`d.type === "play"`,
		`d.type === "pause"`,
		`"timeupdate"`, // and the playhead coming back
		`source: "trendly-player"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the bridge is missing %s", want)
		}
	}
}

// Anything may embed a published player page. Commands are taken only from
// the frame that embedded it, and only when they carry our own marker.
func TestBridgeOnlyObeysItsEmbedder(t *testing.T) {
	page := playerPage("https://cdn/player.js")
	if !strings.Contains(page, "if (e.source !== window.parent) return;") {
		t.Fatal("the bridge accepts commands from senders other than its embedder")
	}
	if !strings.Contains(page, `d.source !== "trendly-studio"`) {
		t.Fatal("the bridge does not check the sender's marker")
	}
}

// Phase 11. Selection has to come out of the player, because the app cannot
// reach into a cross-origin frame to find out what was clicked.
func TestTheBridgeReportsAndRingsASelection(t *testing.T) {
	page := playerPage("https://cdn/player.js")
	for _, want := range []string{
		`post("select"`,            // out: what was clicked
		`d.type === "highlight"`,   // in: ring this element
		`node.closest("[id]")`,     // our films carry ids, not data-hf-id
		`closest("[data-scene]")`,  // and the scene that owns it
		`addEventListener("click"`, //
		`}, true);`,                // capture phase
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the bridge is missing %s", want)
		}
	}
}

// The ring is created at runtime in the player's own copy of the document.
// index.html is what the renderer opens, and it must never see it.
func TestTheSelectionRingIsNeverPartOfTheProject(t *testing.T) {
	page := playerPage("https://cdn/player.js")
	if !strings.Contains(page, `d.createElement("div")`) {
		t.Fatal("the ring should be created at runtime, not authored")
	}
	if !strings.Contains(page, "pointer-events:none") {
		t.Fatal("the ring would swallow the next click")
	}
	// It must also not be selectable itself, or clicking a ringed element
	// would select the ring.
	if !strings.Contains(page, "target.id === RING") {
		t.Fatal("the ring can be selected")
	}
}
