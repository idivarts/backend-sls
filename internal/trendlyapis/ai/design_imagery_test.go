package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/pkg/mediagen"
	"golang.org/x/net/html"
)

func parseDoc(t *testing.T, s string) *html.Node {
	t.Helper()
	n, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return n
}

func TestCollectPlaceholders(t *testing.T) {
	doc := parseDoc(t, `<html><body>
		<img data-stock="empty cafe at dawn" data-el="bg">
		<img data-gen="our can on wet stone" data-el="hero">
		<img src="https://cdn/already.png" data-el="logo">
	</body></html>`)

	got := collectPlaceholders(doc)
	if len(got) != 2 {
		t.Fatalf("expected 2 placeholders, got %d", len(got))
	}
	if !got[0].stock || got[0].prompt != "empty cafe at dawn" {
		t.Errorf("first should be the stock one, got %+v", got[0])
	}
	if got[1].stock || got[1].prompt != "our can on wet stone" {
		t.Errorf("second should be the generated one, got %+v", got[1])
	}
}

func TestCollectPlaceholdersIgnoresResolvedAndEmpty(t *testing.T) {
	// An empty attribute is not a request for an image — filling it would mean
	// searching for "" and placing something arbitrary.
	doc := parseDoc(t, `<html><body>
		<img data-stock="" data-el="a">
		<img data-gen="   " data-el="b">
		<div data-stock="not an img"></div>
	</body></html>`)
	if got := collectPlaceholders(doc); len(got) != 0 {
		t.Errorf("expected no placeholders, got %d: %+v", len(got), got)
	}
}

func TestCollectPlaceholdersPrefersStock(t *testing.T) {
	// If the model writes both, stock wins: it is the cheaper of the two and
	// resolving both would place one image and pay for another nobody sees.
	doc := parseDoc(t, `<html><body><img data-stock="a street" data-gen="a street"></body></html>`)
	got := collectPlaceholders(doc)
	if len(got) != 1 || !got[0].stock {
		t.Fatalf("expected one stock placeholder, got %+v", got)
	}
}

func TestFillDesignImageryLeavesDocumentAloneWithNoPlaceholders(t *testing.T) {
	in := `<!doctype html><html><head></head><body><section data-slide><h1>Hi</h1></section></body></html>`
	out, res := fillDesignImagery(context.Background(), "brand", "content-1", "4:5", in)
	if out != in {
		t.Error("a document with no placeholders must be returned byte-identical")
	}
	if res.touched() {
		t.Errorf("expected an untouched result, got %+v", res)
	}
}

func TestFillDesignImageryKeepsUnresolvedPlaceholders(t *testing.T) {
	// With no Pexels key and no image model reachable in a unit test, nothing
	// resolves — and the placeholder must survive. Deleting it would silently
	// change the layout the model designed, with nothing to point at.
	in := `<!doctype html><html><body><img data-stock="a street at dusk" data-el="bg"></body></html>`
	out, res := fillDesignImagery(context.Background(), "brand-does-not-exist", "content-1", "4:5", in)
	if !strings.Contains(out, `data-stock="a street at dusk"`) {
		t.Error("unresolved placeholder was dropped from the document")
	}
	if strings.Contains(out, `src=`) {
		t.Error("an unresolved placeholder must not gain a src")
	}
	if res.Failed != 1 {
		t.Errorf("expected Failed=1, got %+v", res)
	}
}

func TestSetAttr(t *testing.T) {
	doc := parseDoc(t, `<html><body><img data-el="x" src="old"></body></html>`)
	imgs := collectAllImgs(doc)
	if len(imgs) != 1 {
		t.Fatalf("expected 1 img, got %d", len(imgs))
	}
	setAttr(imgs[0], "src", "new")
	setAttr(imgs[0], "data-credit", "Photo by A on Pexels")
	if v, _ := attr(imgs[0], "src"); v != "new" {
		t.Errorf("src not replaced, got %q", v)
	}
	if v, _ := attr(imgs[0], "data-credit"); v != "Photo by A on Pexels" {
		t.Errorf("credit not added, got %q", v)
	}
	// Replacing must not duplicate the attribute.
	n := 0
	for _, a := range imgs[0].Attr {
		if a.Key == "src" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("src appears %d times, want 1", n)
	}
}

func collectAllImgs(root *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

func TestAspectForSize(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{1080, 1920, "9:16"},
		{1080, 1350, "4:5"},
		{1080, 1080, "1:1"},
		{1920, 1080, "16:9"},
		{0, 0, "1:1"},
	}
	for _, c := range cases {
		if got := aspectForSize(c.w, c.h); got != c.want {
			t.Errorf("%dx%d: got %q, want %q", c.w, c.h, got, c.want)
		}
	}
}

// ── The visible credit ──────────────────────────────────────────────────────
//
// A data-credit attribute satisfied nobody: nothing in the product ever painted
// it, so in practice the credit did not exist. Unsplash's §9 and Guideline 3
// require it to be DISPLAYED where the photo is, which makes this a rendering
// concern rather than a metadata one.

func firstImg(t *testing.T, root *html.Node) *html.Node {
	t.Helper()
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatal("no img in document")
	}
	return found
}

func render(t *testing.T, root *html.Node) string {
	t.Helper()
	var b strings.Builder
	if err := html.Render(&b, root); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestPlaceCreditLandsInsideTheSlideAndLinksThePhotographer(t *testing.T) {
	doc := parseDoc(t, `<html><body><section data-slide="0" style="position:relative">
		<img data-el="bg" src="https://images.unsplash.com/regular">
	</section></body></html>`)
	placeCredit(firstImg(t, doc), "Photo by Ada on Unsplash",
		"https://unsplash.com/@ada?utm_source=trendly&utm_medium=referral")

	out := render(t, doc)
	if !strings.Contains(out, `data-el="stock-credit-0"`) {
		t.Fatalf("no credit element:\n%s", out)
	}
	if !strings.Contains(out, "Photo by Ada on Unsplash") {
		t.Errorf("credit text missing:\n%s", out)
	}
	// The referral tag is a term of use, not a nicety.
	if !strings.Contains(out, "utm_source=trendly") || !strings.Contains(out, "utm_medium=referral") {
		t.Errorf("the credit link is not referral-tagged:\n%s", out)
	}
	// It has to sit in the slide, or it will not travel into the render.
	slideIdx := strings.Index(out, `data-slide="0"`)
	creditIdx := strings.Index(out, `data-el="stock-credit-0"`)
	if slideIdx < 0 || creditIdx < slideIdx {
		t.Errorf("credit is not inside the slide:\n%s", out)
	}
}

func TestPlaceCreditCombinesTwoPhotosOnOneSlide(t *testing.T) {
	doc := parseDoc(t, `<html><body><section data-slide="0" style="position:relative">
		<img data-el="bg"><img data-el="inset">
	</section></body></html>`)
	img := firstImg(t, doc)
	placeCredit(img, "Photo by Ada on Unsplash", "https://unsplash.com/@ada")
	placeCredit(img, "Photo by Bo on Unsplash", "https://unsplash.com/@bo")

	out := render(t, doc)
	// One line naming both, not two captions stacked in a corner.
	if n := strings.Count(out, `data-el="stock-credit-0"`); n != 1 {
		t.Errorf("want one credit element, got %d:\n%s", n, out)
	}
	for _, want := range []string{"Ada", "Bo", " · "} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestPlaceCreditFallsBackToTheBodyWithNoSlide(t *testing.T) {
	doc := parseDoc(t, `<html><body><img data-el="bg"></body></html>`)
	placeCredit(firstImg(t, doc), "Photo by Ada on Unsplash", "")
	out := render(t, doc)
	if !strings.Contains(out, `data-el="stock-credit"`) {
		t.Errorf("a design with no slides should still be credited:\n%s", out)
	}
	// No link to make, so no empty anchor either.
	if strings.Contains(out, "<a") {
		t.Errorf("an unknown photographer should not produce a link:\n%s", out)
	}
}

// ── Hotlink vs re-host ──────────────────────────────────────────────────────

type fakeStock struct {
	asset    mediagen.Asset
	reported *int
}

func (fakeStock) Name() string                  { return "fake-stock" }
func (fakeStock) Supports(k mediagen.Kind) bool { return k == mediagen.KindStock }
func (f fakeStock) Submit(context.Context, mediagen.Spec) (mediagen.Result, error) {
	return mediagen.Result{Status: mediagen.StatusSucceeded, Assets: []mediagen.Asset{f.asset}}, nil
}
func (fakeStock) Get(context.Context, string) (mediagen.Result, error) {
	return mediagen.Result{}, mediagen.ErrNotAsynchronous
}
func (f fakeStock) ReportUsed(context.Context, mediagen.Asset) error {
	*f.reported++
	return nil
}

func TestResolveStockHotlinksAndReportsThePlacementOnce(t *testing.T) {
	reported := 0
	mediagen.Register(fakeStock{
		asset: mediagen.Asset{
			URL:         "https://images.unsplash.com/regular",
			Alt:         "a cat",
			Hotlink:     true,
			Attribution: "Photo by Ada on Unsplash",
			CreatorURL:  "https://unsplash.com/@ada?utm_source=trendly&utm_medium=referral",
			TrackingRef: "https://api.unsplash.com/photos/abc/download",
		},
		reported: &reported,
	})
	t.Setenv("STOCK_PROVIDER", "fake-stock")

	src, alt, credit, creditURL := resolveStock(context.Background(), "brand-1", "content-1", "a cat", "1:1")

	// Hotlinked: the URL is used as-is. Copying it would be a material breach
	// of the API terms, and any rewrite here is the symptom.
	if src != "https://images.unsplash.com/regular" {
		t.Errorf("a hotlinked photo must keep its own url, got %q", src)
	}
	if alt != "a cat" || credit != "Photo by Ada on Unsplash" {
		t.Errorf("alt/credit did not travel: %q / %q", alt, credit)
	}
	if !strings.Contains(creditURL, "utm_source=trendly") {
		t.Errorf("credit link lost its referral tag: %q", creditURL)
	}
	// Placing a photo is one use, however many results the search returned.
	if reported != 1 {
		t.Errorf("the placement should report exactly once, got %d", reported)
	}
}

func TestResolveStockSurvivesAProviderWithNothingToGive(t *testing.T) {
	mediagen.Register(emptyStock{})
	t.Setenv("STOCK_PROVIDER", "empty-stock")
	// A missing key or no match leaves the placeholder alone rather than
	// failing the design — an <img> with no src is a visible gap the user can
	// ask about, which beats a layout quietly changed.
	src, _, credit, _ := resolveStock(context.Background(), "brand-1", "content-1", "a cat", "1:1")
	if src != "" || credit != "" {
		t.Errorf("want empty results, got %q / %q", src, credit)
	}
}

type emptyStock struct{}

func (emptyStock) Name() string                  { return "empty-stock" }
func (emptyStock) Supports(k mediagen.Kind) bool { return k == mediagen.KindStock }
func (emptyStock) Submit(context.Context, mediagen.Spec) (mediagen.Result, error) {
	return mediagen.Result{Status: mediagen.StatusFailed, Error: "stock photography is not configured"}, nil
}
func (emptyStock) Get(context.Context, string) (mediagen.Result, error) {
	return mediagen.Result{}, mediagen.ErrNotAsynchronous
}

func TestCreditIDsAreUniquePerSlide(t *testing.T) {
	// A data-el is how the editor selects an element and how a comment anchors
	// to one. Two slides sharing an id would make both uneditable and pin every
	// comment to whichever matched first.
	doc := parseDoc(t, `<html><body>
		<section data-slide="0" style="position:relative"><img data-el="a"></section>
		<section data-slide="1" style="position:relative"><img data-el="b"></section>
	</body></html>`)
	var imgs []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			imgs = append(imgs, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	placeCredit(imgs[0], "Photo by Ada on Unsplash", "")
	placeCredit(imgs[1], "Photo by Bo on Unsplash", "")

	out := render(t, doc)
	for _, want := range []string{`data-el="stock-credit-0"`, `data-el="stock-credit-1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s:\n%s", want, out)
		}
	}
}
