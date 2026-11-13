package design

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func TestNoDesignSystemIsNotAnError(t *testing.T) {
	if kit := buildBrandKit(context.Background(), nil); kit != nil {
		t.Fatal("a brand with no design system must author exactly as it did before")
	}
	// A document with nothing in it is the same case: there is no brand to
	// put on the film, and an empty <style> block is noise in every render.
	if kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{}); kit != nil {
		t.Fatal("an empty design system produced a kit")
	}
}

func TestColoursBecomeTokensByRoleAndByNumber(t *testing.T) {
	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Palette: []trendlymodels.DSColor{
			{Role: "primary", Hex: "#E4B34F"},
			{Role: "background", Hex: "#0F0B08"},
			// No role. It must still be reachable, or a brand that filled in
			// a swatch without naming it silently loses the colour.
			{Hex: "#FFFFFF"},
		},
	})
	if kit == nil {
		t.Fatal("no kit")
	}
	for _, want := range []string{"--brand-primary:#E4B34F", "--brand-background:#0F0B08", "--brand-color-3:#FFFFFF"} {
		if !strings.Contains(kit.CSS, want) {
			t.Fatalf("missing %s in %s", want, kit.CSS)
		}
	}
}

// A role is brand-authored text. "Primary Brand" must not become a custom
// property no selector can reference.
func TestRoleNamesAreMadeSafeForCSS(t *testing.T) {
	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Palette: []trendlymodels.DSColor{{Role: "Primary Brand!", Hex: "#111111"}},
	})
	if kit == nil || !strings.Contains(kit.CSS, "--brand-primary-brand:#111111") {
		t.Fatalf("unsafe or missing token: %v", kit)
	}
}

func TestFontBecomesAStackAndAFaceWhenItHasAFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not really a font, but bytes"))
	}))
	defer srv.Close()

	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Fonts: []trendlymodels.DSFont{
			{Role: "heading", Family: "Clash Display", Weight: "700", URL: srv.URL + "/clash.woff2"},
			// Google-sourced: the renderer resolves these itself, so a stack
			// is all it needs and fetching one would be wasted bytes.
			{Role: "body", Family: "Inter", Fallback: "sans-serif"},
		},
	})
	if kit == nil {
		t.Fatal("no kit")
	}
	if !strings.Contains(kit.CSS, "--brand-font-heading:'Clash Display',sans-serif") {
		t.Fatalf("heading stack missing: %s", kit.CSS)
	}
	if !strings.Contains(kit.CSS, "@font-face{font-family:'Clash Display';src:url('assets/fonts/clash-display.woff2')") {
		t.Fatalf("no @font-face for the fetched file: %s", kit.CSS)
	}
	if !strings.Contains(kit.CSS, "font-weight:700") {
		t.Fatalf("weight dropped: %s", kit.CSS)
	}
	if len(kit.Files) != 1 || kit.Files[0].Path != "assets/fonts/clash-display.woff2" {
		t.Fatalf("the font file did not travel with the project: %+v", kit.Files)
	}
}

func TestLogoTravelsAsAFileAndThePromptNamesIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("PNG bytes"))
	}))
	defer srv.Close()

	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Logos: []trendlymodels.DSLogo{
			{Variant: "icon", URL: srv.URL + "/icon.png"},
			{Variant: "primary", URL: srv.URL + "/primary.svg"},
		},
	})
	if kit == nil {
		t.Fatal("no kit")
	}
	// The primary variant wins over one that merely came first.
	if len(kit.Files) != 1 || kit.Files[0].Path != "assets/brand-logo.svg" {
		t.Fatalf("wrong logo chosen: %+v", kit.Files)
	}
	if !strings.Contains(kit.CSS, "--brand-logo:url('assets/brand-logo.svg')") {
		t.Fatalf("no logo token: %s", kit.CSS)
	}
	// A model reaches for <img src=…>, and var() is not valid in src, so the
	// path has to be in the prompt as a path.
	if !strings.Contains(kit.Prompt, "assets/brand-logo.svg") {
		t.Fatalf("the prompt never names the logo file: %s", kit.Prompt)
	}
	if !strings.Contains(kit.Prompt, "never a remote URL") {
		t.Fatalf("the prompt does not forbid a remote reference: %s", kit.Prompt)
	}
}

// A render must work with no network. An asset that could not be fetched is
// left out entirely rather than referenced and missing at render time.
func TestAnAssetThatCannotBeFetchedIsLeftOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Palette: []trendlymodels.DSColor{{Role: "primary", Hex: "#123456"}},
		Logos:   []trendlymodels.DSLogo{{Variant: "primary", URL: srv.URL + "/gone.png"}},
	})
	if kit == nil {
		t.Fatal("a failed logo must not lose the whole kit")
	}
	if len(kit.Files) != 0 {
		t.Fatalf("a file was added for an asset that 404'd: %+v", kit.Files)
	}
	if strings.Contains(kit.CSS, "--brand-logo") {
		t.Fatalf("the CSS references a logo that is not there: %s", kit.CSS)
	}
	if !strings.Contains(kit.CSS, "--brand-primary:#123456") {
		t.Fatal("the colours were lost with the logo")
	}
}

// The project is capped at 4 MiB in total. An oversized asset is refused
// rather than truncated into a corrupt file.
func TestAnOversizedAssetIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, maxAssetBytes+1))
	}))
	defer srv.Close()

	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Palette: []trendlymodels.DSColor{{Role: "primary", Hex: "#123456"}},
		Logos:   []trendlymodels.DSLogo{{URL: srv.URL + "/huge.png"}},
	})
	if kit == nil || len(kit.Files) != 0 {
		t.Fatalf("an oversized asset was accepted: %+v", kit)
	}
}

func TestImageryBecomesLookAndBannedLines(t *testing.T) {
	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Palette: []trendlymodels.DSColor{{Role: "primary", Hex: "#000000"}},
		Imagery: &trendlymodels.DSImagery{
			MoodKeywords:   []string{"warm", "handmade"},
			ColorTreatment: "high contrast",
			Avoid:          []string{"stock smiles", "lens flare"},
		},
	})
	if kit == nil {
		t.Fatal("no kit")
	}
	if !strings.Contains(kit.Prompt, "LOOK: warm, handmade. Treatment: high contrast") {
		t.Fatalf("no LOOK line: %s", kit.Prompt)
	}
	if !strings.Contains(kit.Prompt, "BANNED: stock smiles, lens flare") {
		t.Fatalf("no BANNED line: %s", kit.Prompt)
	}
}
