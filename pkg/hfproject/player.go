package hfproject

// Serving a design as a LIVE, scrubbable film — before it is rendered.
//
// ── What this is ──────────────────────────────────────────────────────────
// Hyperframes ships a web component, `<hyperframes-player>`, that plays a
// composition in any browser. `hyperframes play` serves exactly four things:
// the player, the runtime, a page that puts them together, and the project.
// Nothing about it needs their desktop app or their dev server; it is static
// files, so CloudFront can serve the same four.
//
// ── The one thing that is not obvious ─────────────────────────────────────
// The composition page must LOAD THE RUNTIME ITSELF. Their dev server quietly
// injects that script tag into the HTML it serves, so a project that plays
// perfectly under `hyperframes play` comes up inert from static files:
// `window.__timelines` is undefined, every scene sits at its CSS default, and
// seeking does nothing. Measured, before the tag was added and after.
//
// So the runtime tag goes into a SEPARATE preview.html rather than into
// index.html. index.html is what the renderer opens, and the CLI injects its
// own runtime there — adding a second one to the file the render path uses
// would be meddling with the one path that is currently working, to fix a
// different one.
//
// ── The second thing that is not obvious: GSAP ────────────────────────────
// A composition animates with `gsap.timeline()` and Hyperframes' own docs
// show exactly that with NO GSAP script tag, so that is what the model
// writes. Their `init` scaffold, by contrast, loads GSAP off jsdelivr — the
// docs and the scaffold disagree, and the project we store follows the docs.
//
// The RENDER path survives this because the worker copies GSAP into its own
// materialised project and patches every document before it calls the CLI
// (services/render-worker/src/hyperframes.ts, vendorGsap). That copy is a
// temp directory. What sits in S3 is the project AS AUTHORED, with no GSAP
// anywhere — so the player loaded it, the inline script threw on the first
// `gsap.timeline()`, no timeline was ever registered, and the film came up
// BLANK while the very same project rendered correctly.
//
// So the player page vendors GSAP the same way the renderer does. The two
// paths now agree; player_test.go pins the file to the worker's byte for
// byte so they cannot drift apart again.
//
// ── Why the scripts are shared, not per project ───────────────────────────
// They are 849 KB together and identical for every design. Copying them into
// each project prefix would add that to every authoring run and every S3
// bill, for bytes CloudFront would then cache separately per design. They go
// once to a versioned shared prefix and every project points at them.

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/idivarts/backend-sls/pkg/mediastore"
)

//go:embed playerassets/player.js
var playerJS []byte

//go:embed playerassets/runtime.js
var runtimeJS []byte

// gsapJS is a byte-for-byte copy of the renderer's vendored GSAP.
//
// Duplicated rather than shared because `go:embed` cannot reach outside its
// own package directory and will not follow a symlink, and the renderer's
// copy is where it is because a Dockerfile COPYs it into the image. The
// duplication is held honest by a test, not by this comment.
//
//go:embed playerassets/gsap.js
var gsapJS []byte

// playerAssetVersion scopes the shared assets so an upgrade cannot change what
// an existing design plays. A project written against one runtime keeps that
// runtime until something re-uploads it.
const playerAssetVersion = "0.9.39"

func sharedAssetKey(name string) string {
	return fmt.Sprintf("design-player/%s/%s", playerAssetVersion, name)
}

var headOpenRe = regexp.MustCompile(`(?i)<head[^>]*>`)
var bodyOpenRe = regexp.MustCompile(`(?i)<body[^>]*>`)

// gsapLoadedRe matches a script tag already pulling GSAP in, local or CDN.
var gsapLoadedRe = regexp.MustCompile(`(?i)<script[^>]+src=["'][^"']*gsap[^"']*["']`)

// withScripts returns index.html with the Hyperframes runtime and GSAP loaded.
//
// Both must come before anything that uses them: the composition's own script
// builds a timeline with `gsap.timeline()` and registers it on
// `window.__timelines`, which the runtime creates. Either tag after that
// script and the page throws on load — which is precisely the blank-player
// bug described at the top of this file.
//
// A project that already loads GSAP itself — one that came from `hyperframes
// init`, or a model that remembered the tag — is left alone. Two copies of
// GSAP on one page is a second global clobbering the first, and the one we
// would add loads last.
func withScripts(indexHTML, runtimeURL, gsapURL string) string {
	tags := fmt.Sprintf(`<script src=%q></script>`, runtimeURL)
	if !gsapLoadedRe.MatchString(indexHTML) {
		tags += "\n    " + fmt.Sprintf(`<script src=%q></script>`, gsapURL)
	}
	return InjectIntoHead(indexHTML, tags)
}

// InjectIntoHead puts a snippet immediately after <head>.
//
// Falling back to <body>, and failing both, to the very top of the document —
// a model's answer is not guaranteed to have either, and a snippet that lands
// nowhere is worse than one that lands early. Early is always safe for the
// things that go through here: they define what the rest of the document
// uses, so nothing can want them later.
//
// Exported because brand tokens take exactly the same route as the player's
// script tags, and two implementations of "put this at the top of the head"
// would be two chances to put it in the wrong place.
func InjectIntoHead(html, snippet string) string {
	if loc := headOpenRe.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + "\n    " + snippet + html[loc[1]:]
	}
	if loc := bodyOpenRe.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + "\n    " + snippet + html[loc[1]:]
	}
	return snippet + "\n" + html
}

// bridgeJS lets the app that embeds this page drive the film.
//
// ── Why a bridge of our own ───────────────────────────────────────────────
// <hyperframes-player> has seek/play/pause as METHODS, and the app is on a
// different origin from this page, so it cannot reach them. The element does
// listen for postMessage, but `_handleRuntimeMessage` checks the sender is
// the composition iframe's contentWindow — a message from the parent is
// dropped. There is no parent-facing protocol to use, so this adds the
// smallest one that does the job.
//
// ── What crosses, and in which direction ──────────────────────────────────
// In: seek / play / pause, accepted only from window.parent — the frame that
// embedded us, which could already drive playback through the controls.
// Out: the playhead, so the app's timeline follows the film rather than
// guessing at it. Position and duration only; nothing about the design.
//
// The first reply goes to "*" because the embedder's origin is not known
// until it speaks. It carries a time and a duration, which is why that is
// acceptable; everything after a command goes back to that command's origin.
const bridgeJS = `
(function () {
  var el = document.querySelector("hyperframes-player");
  if (!el || window.parent === window) return;
  var origin = null;
  var RING = "__trendly_selection_ring";

  function post(type, extra) {
    var msg = {
      source: "trendly-player",
      type: type,
      timeMs: Math.round((el.currentTime || 0) * 1000),
      durationMs: Math.round((el.duration || 0) * 1000),
      paused: !!el.paused
    };
    if (extra) { for (var k in extra) { msg[k] = extra[k]; } }
    try { window.parent.postMessage(msg, origin || "*"); } catch (e) {}
  }

  // The composition is a nested iframe on the same origin as this page, so
  // its document is reachable. Wrapped because a cross-origin or not-yet-
  // loaded frame throws rather than returning null.
  function compositionDoc() {
    try {
      var f = el.shadowRoot ? el.shadowRoot.querySelector("iframe") : null;
      return f && f.contentDocument;
    } catch (e) { return null; }
  }

  // ── What the user clicked ───────────────────────────────────────────────
  // Walking UP from the event target, because a click lands on whatever is
  // topmost — a text node's span, a glyph, a background layer — and what the
  // app can act on is the nearest element with an id. Our authoring contract
  // gives every addressable element one; Hyperframes' own data-hf-id is a
  // Studio attribute our films do not carry.
  function describe(node) {
    if (!node || !node.closest) return null;
    var target = node.closest("[id]");
    if (!target || target.id === RING) return null;
    var scene = target.closest("[data-scene]");
    var r = target.getBoundingClientRect();
    return {
      elementId: target.id,
      tag: target.tagName.toLowerCase(),
      sceneId: scene ? scene.getAttribute("data-scene") : "",
      text: (target.textContent || "").replace(/\s+/g, " ").trim().slice(0, 240),
      box: { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) }
    };
  }

  // Capture phase, so a composition that stops propagation on its own
  // handlers cannot swallow the selection.
  function attachPicker() {
    var d = compositionDoc();
    if (!d || d.__trendlyPicker) return;
    d.__trendlyPicker = true;
    d.addEventListener("click", function (ev) {
      var info = describe(ev.target);
      post("select", info || { elementId: "", sceneId: "", tag: "", text: "" });
    }, true);
  }

  // The ring lives in the composition document and is created at runtime, so
  // it is in the player only. index.html is what the renderer opens and it
  // never sees this.
  function highlight(id) {
    var d = compositionDoc();
    if (!d || !d.body) return;
    var prev = d.getElementById(RING);
    if (prev && prev.parentNode) prev.parentNode.removeChild(prev);
    if (!id) return;
    var t = d.getElementById(id);
    if (!t) return;
    var r = t.getBoundingClientRect();
    var ring = d.createElement("div");
    ring.id = RING;
    ring.style.cssText = "position:fixed;pointer-events:none;z-index:2147483647;" +
      "border:2px solid #4C8DFF;border-radius:4px;box-shadow:0 0 0 2px rgba(76,141,255,0.25);" +
      "left:" + r.left + "px;top:" + r.top + "px;width:" + r.width + "px;height:" + r.height + "px;";
    d.body.appendChild(ring);
  }

  el.addEventListener("ready", function () { post("ready"); attachPicker(); });
  el.addEventListener("durationchange", function () { post("ready"); });
  el.addEventListener("timeupdate", function () { post("time"); });
  // The composition can mount after "ready" on a reload, so try again when
  // the frame loads rather than only once.
  window.addEventListener("load", attachPicker);
  setTimeout(attachPicker, 1200);

  window.addEventListener("message", function (e) {
    if (e.source !== window.parent) return;
    var d = e.data;
    if (!d || d.source !== "trendly-studio") return;
    origin = e.origin;
    try {
      if (d.type === "seek" && typeof el.seek === "function") {
        el.seek(Math.max(0, (d.timeMs || 0) / 1000));
      } else if (d.type === "play" && typeof el.play === "function") {
        el.play();
      } else if (d.type === "pause" && typeof el.pause === "function") {
        el.pause();
      } else if (d.type === "highlight") {
        attachPicker();
        highlight(d.elementId || "");
        return;
      }
    } catch (err) {}
    post(d.type === "hello" ? "ready" : "time");
  });
})();
`

// playerPage is the page the app embeds. Deliberately bare: it is shown inside
// an iframe that already has a frame around it, so any chrome here would be a
// second one.
//
// ── `interactive` is what makes selection possible ───────────────────────
// The player's composition iframe is `pointer-events: none` by default, so a
// click on the film reaches the player's own controls and toggles playback —
// which is what happened the first time selection was tried on dev: the
// headline could not be clicked, only played. Their stylesheet has an opt-in
// for it, `:host([interactive])`, described as being for "an interactive
// composition (e.g. a live slideshow/app with playable media or controls)".
// A composition the user is editing is exactly that.
//
// `controls` stays: the controls layer has its own pointer-events and sits
// above the composition, so the transport still works and the film below it
// becomes clickable.
func playerPage(playerURL string) string {
	return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Preview</title>
    <style>
      html, body { margin: 0; height: 100%; background: #000; overflow: hidden; }
      hyperframes-player { width: 100%; height: 100%; display: block; }
    </style>
  </head>
  <body>
    <hyperframes-player src="preview.html" controls muted interactive></hyperframes-player>
    <script src="` + playerURL + `"></script>
    <script>` + bridgeJS + `</script>
  </body>
</html>
`
}

// EnsureSharedPlayerAssets puts the player and the runtime where every project
// can reach them, and returns their URLs.
//
// Re-uploaded on each authoring run rather than checked first. A HEAD per
// asset costs a round trip to save a PUT of bytes that are identical every
// time, and a missing asset breaks every preview at once — the failure that
// matters here is the one where an upload was skipped because something
// thought the object was already there.
func EnsureSharedPlayerAssets(ctx context.Context, brandID string) (playerURL, runtimeURL, gsapURL string, err error) {
	put := func(name string, data []byte) (string, error) {
		s, err := mediastore.Put(ctx, mediastore.Spec{
			BrandID:     brandID,
			Target:      mediastore.TargetAttachment,
			ExactKey:    sharedAssetKey(name),
			Filename:    name,
			ContentType: "text/javascript; charset=utf-8",
			NoLibrary:   true,
		}, data)
		if err != nil {
			return "", fmt.Errorf("hfproject: shared %s: %w", name, err)
		}
		return s.URL, nil
	}
	if playerURL, err = put("player.js", playerJS); err != nil {
		return "", "", "", err
	}
	if runtimeURL, err = put("runtime.js", runtimeJS); err != nil {
		return "", "", "", err
	}
	// Named for its version so a GSAP upgrade cannot change what an existing
	// design plays, the same reason the prefix carries playerAssetVersion.
	if gsapURL, err = put("gsap-3.15.0.min.js", gsapJS); err != nil {
		return "", "", "", err
	}
	return playerURL, runtimeURL, gsapURL, nil
}

// UploadPlayer writes the two pages that make a project playable and returns
// the URL of the one to embed.
//
// Best-effort by contract: a design whose player could not be written still
// renders, previews and exports. The caller logs and carries on rather than
// failing an authoring run over the preview surface.
func UploadPlayer(ctx context.Context, brandID, designID string, p *Project) (string, error) {
	var indexHTML string
	for _, f := range p.Files {
		if strings.EqualFold(f.Path, "index.html") {
			indexHTML = f.Content
			break
		}
	}
	if indexHTML == "" {
		return "", fmt.Errorf("hfproject.UploadPlayer: the project has no index.html")
	}

	playerURL, runtimeURL, gsapURL, err := EnsureSharedPlayerAssets(ctx, brandID)
	if err != nil {
		return "", err
	}

	prefix := Prefix(brandID, designID)
	write := func(name, body string) (string, error) {
		s, err := mediastore.Put(ctx, mediastore.Spec{
			BrandID:     brandID,
			Target:      mediastore.TargetAttachment,
			ExactKey:    prefix + "/" + name,
			Filename:    name,
			ContentType: "text/html; charset=utf-8",
			NoLibrary:   true,
		}, []byte(body))
		if err != nil {
			return "", fmt.Errorf("hfproject.UploadPlayer: %s: %w", name, err)
		}
		return s.URL, nil
	}

	if _, err := write("preview.html", withScripts(indexHTML, runtimeURL, gsapURL)); err != nil {
		return "", err
	}
	return write("player.html", playerPage(playerURL))
}
