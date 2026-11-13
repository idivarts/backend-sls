package design

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// The real guard refuses a loopback host, and should: these tests serve
// their assets from one, so they lift it for the duration.
// No test may reach the real Google Fonts endpoint: a unit suite that needs
// the network is a suite that fails on a train. Pointed at a loopback port
// nothing listens on, which the asset guard refuses before any dial anyway.
// The one test that exercises the Google path stubs it deliberately.
func TestMain(m *testing.M) {
	googleFontsCSS = "https://127.0.0.1:1/css2"
	os.Exit(m.Run())
}

// kit.CSS is itself wrapped in <style>…</style>, so "contains </style>" is
// true of every kit. What matters is whether a VALUE added a second one, or
// any tag at all.
func escapedTheStyleBlock(css string) bool {
	return strings.Contains(css, "<script") ||
		strings.Count(css, "</style>") != 1 ||
		strings.Count(css, "<style>") != 1
}

func allowLoopbackAssets(t *testing.T) {
	t.Helper()
	prev := assetURLAllowed
	assetURLAllowed = func(string) bool { return true }
	t.Cleanup(func() { assetURLAllowed = prev })
}

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
	allowLoopbackAssets(t)
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
	if !strings.Contains(kit.CSS, "@font-face{font-family:'Clash Display';src:url('assets/fonts/clash-display-700.woff2')") {
		t.Fatalf("no @font-face for the fetched file: %s", kit.CSS)
	}
	if !strings.Contains(kit.CSS, "font-weight:700") {
		t.Fatalf("weight dropped: %s", kit.CSS)
	}
	if len(kit.Files) != 1 || kit.Files[0].Path != "assets/fonts/clash-display-700.woff2" {
		t.Fatalf("the font file did not travel with the project: %+v", kit.Files)
	}
}

func TestLogoTravelsAsAFileAndThePromptNamesIt(t *testing.T) {
	allowLoopbackAssets(t)
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
	allowLoopbackAssets(t)
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
	allowLoopbackAssets(t)
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

// The guard itself, on the real function rather than the test override.
func TestOnlyPublicHTTPSAssetsMayBeFetched(t *testing.T) {
	for _, bad := range []string{
		"http://example.com/logo.png",              // plain http
		"https://127.0.0.1/logo.png",               // loopback
		"https://localhost/logo.png",               // loopback by name
		"https://169.254.169.254/latest/meta-data", // the metadata address
		"https://10.0.0.5/logo.png",                // private
		"https://192.168.1.10/logo.png",            // private
		"not a url",
		"",
	} {
		if publicHTTPS(bad) {
			t.Errorf("the guard allowed %q", bad)
		}
	}
	if !publicHTTPS("https://example.com/logo.png") {
		t.Error("the guard refused an ordinary public https URL")
	}
}

// A hex is the one value that reaches a stylesheet unescaped, and the design
// system is written client-side with nothing validating it.
func TestAColourThatIsNotAColourNeverReachesTheStylesheet(t *testing.T) {
	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Palette: []trendlymodels.DSColor{
			{Role: "primary", Hex: "red}</style><script>alert(1)</script><style>{"},
			{Role: "accent", Hex: "#00FF00"},
		},
	})
	if kit == nil {
		t.Fatal("the good colour was lost with the bad one")
	}
	if escapedTheStyleBlock(kit.CSS) {
		t.Fatalf("the stylesheet was escaped: %s", kit.CSS)
	}
	if !strings.Contains(kit.CSS, "--brand-accent:#00FF00") {
		t.Fatalf("the valid colour did not survive: %s", kit.CSS)
	}
}

func TestAFamilyOrWeightThatIsNotOneIsDropped(t *testing.T) {
	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Palette: []trendlymodels.DSColor{{Role: "primary", Hex: "#111111"}},
		Fonts: []trendlymodels.DSFont{
			{Role: "heading", Family: "Inter';}</style><script>x</script>{'"},
			{Role: "body", Family: "Inter", Weight: "700;}</style>", Fallback: "serif}"},
		},
	})
	if kit == nil {
		t.Fatal("no kit")
	}
	if escapedTheStyleBlock(kit.CSS) {
		t.Fatalf("a font value escaped the stylesheet: %s", kit.CSS)
	}
	// The bad family is dropped entirely; the good one keeps a safe stack
	// and loses only the unusable weight and fallback.
	// Neither family is DECLARED here — no URL, and the Google resolver is
	// stubbed out for the suite — so both reduce to a generic stack. What
	// this test is about is that nothing escaped, and that the unsafe
	// family never reaches a token at all.
	if !strings.Contains(kit.CSS, "--brand-font-body:sans-serif") {
		t.Fatalf("the valid role lost its token: %s", kit.CSS)
	}
	// The token is always DEFINED — the prompt lists these names, and a
	// var() pointing at nothing is an invalid declaration rather than a
	// fallback — but it must carry the generic stack only.
	if !strings.Contains(kit.CSS, "--brand-font-heading:sans-serif") {
		t.Fatalf("the unsafe family was not reduced to its fallback: %s", kit.CSS)
	}
	if strings.Contains(kit.CSS, "--brand-font-heading:'") {
		t.Fatalf("the unsafe family was kept: %s", kit.CSS)
	}
}

// One family at two weights needs two files, or every weight renders from
// whichever face won.
func TestTwoWeightsOfOneFamilyGetTwoFiles(t *testing.T) {
	allowLoopbackAssets(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("font bytes"))
	}))
	defer srv.Close()

	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Fonts: []trendlymodels.DSFont{
			{Role: "heading", Family: "Inter", Weight: "700", URL: srv.URL + "/b.woff2"},
			{Role: "body", Family: "Inter", Weight: "400", URL: srv.URL + "/r.woff2"},
		},
	})
	if kit == nil {
		t.Fatal("no kit")
	}
	if len(kit.Files) != 2 {
		t.Fatalf("expected a file per weight, got %+v", kit.Files)
	}
	for _, want := range []string{"assets/fonts/inter-700.woff2", "assets/fonts/inter-400.woff2"} {
		if !hasFile(kit.Files, want) {
			t.Fatalf("missing %s in %+v", want, kit.Files)
		}
	}
}

// Build refuses a project over 4 MiB AFTER the model has been charged, so the
// kit stops at its own budget rather than walking a design into that.
func TestTheKitStopsAtItsBudget(t *testing.T) {
	allowLoopbackAssets(t)
	big := make([]byte, maxAssetBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	var fonts []trendlymodels.DSFont
	for i := 0; i < 8; i++ {
		fonts = append(fonts, trendlymodels.DSFont{
			Role: fmt.Sprintf("role%d", i), Family: "Inter",
			Weight: fmt.Sprintf("%d00", i+1),
			URL:    fmt.Sprintf("%s/f%d.woff2", srv.URL, i),
		})
	}
	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{Fonts: fonts})
	if kit == nil {
		t.Fatal("no kit")
	}
	total := 0
	for _, f := range kit.Files {
		total += len(f.Content)
	}
	if total > maxKitBytes {
		t.Fatalf("the kit added %d bytes, over its %d budget", total, maxKitBytes)
	}
	if len(kit.Files) == 0 {
		t.Fatal("the budget swallowed every font")
	}
}

// A family the brand named but gave no file for is resolved from Google, so
// that it can be DECLARED. Undeclared, `check` refuses the whole project:
// "used without @font-face declaration … not in the auto-resolved font list".
// Space Grotesk is a Google family and is not auto-resolved, which is how
// this was found — on dev, as a failed preview.
func TestAFamilyWithNoURLIsResolvedAndDeclared(t *testing.T) {
	allowLoopbackAssets(t)
	var asked, srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".woff2") {
			_, _ = w.Write([]byte("woff2 bytes"))
			return
		}
		asked = r.URL.String()
		// Shaped like the real stylesheet: one @font-face per subset, latin
		// last, which is the one an English film needs.
		_, _ = fmt.Fprintf(w, `
@font-face{font-family:'Space Grotesk';src:url(%[1]s/cyrillic.woff2) format('woff2');}
@font-face{font-family:'Space Grotesk';src:url(%[1]s/latin.woff2) format('woff2');}`, srvURL)
	}))
	defer srv.Close()

	srvURL = srv.URL
	prev := googleFontsCSS
	googleFontsCSS = srv.URL + "/css2"
	t.Cleanup(func() { googleFontsCSS = prev })

	// The fetch of the gstatic URL has to land somewhere too.
	prevAllowed := assetURLAllowed
	assetURLAllowed = func(string) bool { return true }
	t.Cleanup(func() { assetURLAllowed = prevAllowed })

	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Fonts: []trendlymodels.DSFont{{Role: "heading", Family: "Space Grotesk", Weight: "700"}},
	})
	if kit == nil {
		t.Fatal("no kit")
	}
	if !strings.Contains(asked, "family=Space+Grotesk:wght@700") {
		t.Fatalf("the stylesheet was asked for the wrong thing: %q", asked)
	}
	if !strings.Contains(kit.CSS, "@font-face{font-family:'Space Grotesk'") {
		t.Fatalf("the family was not declared: %s", kit.CSS)
	}
	if !strings.Contains(kit.CSS, "--brand-font-heading:'Space Grotesk',sans-serif") {
		t.Fatalf("the family was not named in its token: %s", kit.CSS)
	}
	if len(kit.Files) != 1 {
		t.Fatalf("the font file did not travel with the project: %+v", kit.Files)
	}
}

// And when Google does not have it either, the film still authors — with the
// generic stack, rather than a family `check` would refuse.
func TestAFamilyNobodyCanSupplyFallsBackInsteadOfFailing(t *testing.T) {
	kit := buildBrandKit(context.Background(), &trendlymodels.BrandDesignSystem{
		Palette: []trendlymodels.DSColor{{Role: "primary", Hex: "#101010"}},
		Fonts:   []trendlymodels.DSFont{{Role: "heading", Family: "Totally Made Up", Fallback: "serif"}},
	})
	if kit == nil {
		t.Fatal("no kit")
	}
	if !strings.Contains(kit.CSS, "--brand-font-heading:serif") {
		t.Fatalf("expected the generic stack alone: %s", kit.CSS)
	}
	if strings.Contains(kit.CSS, "@font-face") {
		t.Fatalf("declared a face for a file it never got: %s", kit.CSS)
	}
}
