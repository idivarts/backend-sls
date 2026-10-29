package ai

import (
	"strings"
	"testing"
)

func TestImageDesignHTML(t *testing.T) {
	html := imageDesignHTML([]string{"https://cdn.example.com/a.png"}, 1080, 1350)

	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(html)), "<!doctype") {
		t.Fatalf("expected a full document from wrapHTML, got: %.80s", html)
	}
	for _, want := range []string{
		`data-carousel`,
		`data-slide`,
		`width:1080px`,
		`height:1350px`,
		`src="https://cdn.example.com/a.png"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("design html missing %q", want)
		}
	}
	// The render worker captures each [data-slide] as an element, so the slide
	// must carry its own size rather than relying on the viewport.
	if strings.Contains(html, "vh") || strings.Contains(html, "vw") {
		t.Error("design html must not size slides from the viewport")
	}
}

func TestImageDesignHTMLAddressableElements(t *testing.T) {
	// Every image needs its own data-el, or the model cannot swap one photo
	// later without rewriting the whole document.
	html := imageDesignHTML([]string{"https://x/1.png", "https://x/2.png"}, 1080, 1350)
	for _, want := range []string{`data-el="photo-1"`, `data-el="photo-2"`} {
		if !strings.Contains(html, want) {
			t.Errorf("design html missing %q", want)
		}
	}
	if got := strings.Count(html, "<section data-slide"); got != 2 {
		t.Errorf("expected 2 slides, got %d", got)
	}
}

func TestImageDesignHTMLEscapesURL(t *testing.T) {
	// A signed URL carries & between query params; unescaped it would truncate
	// the src at the first entity and the slide would render blank.
	html := imageDesignHTML([]string{`https://x/a.png?sig=1&exp=2"onerror="x`}, 1080, 1080)
	if strings.Contains(html, `&exp=2`) {
		t.Error("ampersand in the URL was not escaped")
	}
	if strings.Contains(html, `"onerror="`) {
		t.Error("quote in the URL was not escaped — attribute injection")
	}
	if !strings.Contains(html, "&amp;exp=2") {
		t.Error("expected the ampersand to be escaped as &amp;")
	}
}

func TestSizeForFormatPx(t *testing.T) {
	cases := map[string][2]int{
		"reel":     {1080, 1920},
		"story":    {1080, 1920},
		"video":    {1920, 1080},
		"post":     {1080, 1350},
		"carousel": {1080, 1350},
		"":         {1080, 1080},
	}
	for format, want := range cases {
		w, h := sizeForFormatPx(format)
		if w != want[0] || h != want[1] {
			t.Errorf("%q: got %dx%d, want %dx%d", format, w, h, want[0], want[1])
		}
	}
}
