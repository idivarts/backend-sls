package ai

import (
	"context"
	"strings"
	"testing"

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
	out, res := fillDesignImagery(context.Background(), "brand", "4:5", in)
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
	out, res := fillDesignImagery(context.Background(), "brand-does-not-exist", "4:5", in)
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

func TestOrientationForAspect(t *testing.T) {
	cases := map[string]string{
		"9:16":    "portrait",
		"4:5":     "portrait",
		"16:9":    "landscape",
		"1:1":     "square",
		"unknown": "",
	}
	for aspect, want := range cases {
		if got := orientationForAspect(aspect); got != want {
			t.Errorf("%q: got %q, want %q", aspect, got, want)
		}
	}
}
