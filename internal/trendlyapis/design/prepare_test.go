package design

import "testing"

// The paths a film asks for, read out of its own source.
//
// ── The case this exists for ─────────────────────────────────────────────
// A film edited before backend-sls#584 has a revision whose CSS still says
// url('assets/fonts/space-grotesk-700.woff2') and which does not contain
// that file. The gate then refuses the film, and no amount of editing by
// the model fixes it: the family is named by the brand kit's own !important
// block. Reading the reference back out is how the bytes get put where the
// CSS is already pointing.
func TestAssetRefsInReadsThePathsAFilmAsksFor(t *testing.T) {
	src := `<style>@font-face{font-family:'Space Grotesk';` +
		`src:url('assets/fonts/space-grotesk-700.woff2');}` +
		`:root{--brand-logo:url("assets/brand-logo.png")}</style>` +
		`<img src="assets/fonts/space-grotesk-700.woff2">`

	got := assetRefsIn(src)
	if len(got) != 2 {
		t.Fatalf("assetRefsIn = %v, want the font and the logo once each", got)
	}
	want := map[string]bool{
		"assets/fonts/space-grotesk-700.woff2": true,
		"assets/brand-logo.png":                true,
	}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("%q is not a path this source mentions", g)
		}
	}
}

// Only the documents are worth reading; the media beside them is not.
func TestIsSourceFile(t *testing.T) {
	for _, src := range []string{"index.html", "scenes/1.htm", "style.css", "data.json", "app.js"} {
		if !isSourceFile(src) {
			t.Fatalf("%q can reference an asset and must be scanned", src)
		}
	}
	for _, asset := range []string{
		"assets/brand-logo.png", "assets/fonts/inter-400.woff2",
		".media/audio/bgm/bgm_001.wav", "frames/0001.png", "",
	} {
		if isSourceFile(asset) {
			t.Fatalf("%q is not source; reading it back is bytes through a regex for nothing", asset)
		}
	}
}
