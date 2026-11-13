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
	if loc := headOpenRe.FindStringIndex(indexHTML); loc != nil {
		return indexHTML[:loc[1]] + "\n    " + tags + indexHTML[loc[1]:]
	}
	if loc := bodyOpenRe.FindStringIndex(indexHTML); loc != nil {
		return indexHTML[:loc[1]] + "\n    " + tags + indexHTML[loc[1]:]
	}
	// No head and no body. Prepending still puts both ahead of everything,
	// which is all that is required.
	return tags + "\n" + indexHTML
}

// playerPage is the page the app embeds. Deliberately bare: it is shown inside
// an iframe that already has a frame around it, so any chrome here would be a
// second one.
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
    <hyperframes-player src="preview.html" controls muted></hyperframes-player>
    <script src="` + playerURL + `"></script>
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
