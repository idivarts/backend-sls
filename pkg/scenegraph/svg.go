package scenegraph

import (
	"fmt"
	"html"
	"strings"
)

// Resolver maps brand-kit tokens and media refs to concrete render values. The
// AI emits tokens ("brand.navy", "brand.logo") so it never needs the brand's
// real hex/URLs; the handler supplies a Resolver built from the brand kit.
type Resolver struct {
	// Color maps a color token to a hex string. Unknown tokens should be
	// returned unchanged (so literal "#RRGGBB" passes through).
	Color func(token string) string
	// Font maps a font token to a font-family string.
	Font func(token string) string
	// Media maps an element's src/slot to a resolvable URL.
	Media func(src *Src, slot string) string
}

// DefaultResolver passes hex/URLs through and falls back to Trendly brand
// tokens from branding/ so a document renders even without a brand kit.
func DefaultResolver() Resolver {
	colors := map[string]string{
		"brand.navy":    "#054463",
		"brand.accent":  "#02537D",
		"brand.ink":     "#16242E",
		"brand.white":   "#FFFFFF",
		"brand.primary": "#054463",
	}
	fonts := map[string]string{
		"brand.heading": "Quicksand, sans-serif",
		"brand.body":    "Quicksand, sans-serif",
	}
	return Resolver{
		Color: func(t string) string {
			if v, ok := colors[t]; ok {
				return v
			}
			return t
		},
		Font: func(t string) string {
			if v, ok := fonts[t]; ok {
				return v
			}
			if t == "" {
				return "Quicksand, sans-serif"
			}
			return t
		},
		Media: func(src *Src, slot string) string {
			if src != nil {
				return src.Ref
			}
			return slot
		},
	}
}

// RenderSVG serializes a document to an SVG string using the given layout and
// resolver. SVG is resolution-independent and rasterizes to PNG (via the render
// worker's resvg/CanvasKit step) with preview==export fidelity.
func RenderSVG(d *Document, r Resolver) string {
	if r.Color == nil {
		r = DefaultResolver()
	}
	l := ComputeLayout(d)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		d.Size.W, d.Size.H, d.Size.W, d.Size.H)

	writeBackground(&b, d, r)

	for _, box := range l.Boxes {
		el := d.FindElement(box.ID)
		if el == nil {
			continue
		}
		switch el.Type {
		case ElImage, ElLogo:
			writeImage(&b, el, box, r)
		case ElText:
			writeText(&b, el, box, r)
		case ElShape:
			writeShape(&b, el, box, r)
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func writeBackground(b *strings.Builder, d *Document, r Resolver) {
	switch d.Background.Kind {
	case "gradient":
		c1 := r.Color(d.Background.Value)
		c2 := r.Color(d.Background.Value2)
		if c2 == "" {
			c2 = c1
		}
		fmt.Fprintf(b, `<defs><linearGradient id="bg" gradientTransform="rotate(%d)">`, d.Background.Angle)
		fmt.Fprintf(b, `<stop offset="0%%" stop-color="%s"/><stop offset="100%%" stop-color="%s"/></linearGradient></defs>`,
			esc(c1), esc(c2))
		fmt.Fprintf(b, `<rect width="%d" height="%d" fill="url(#bg)"/>`, d.Size.W, d.Size.H)
	case "image":
		fmt.Fprintf(b, `<image href="%s" width="%d" height="%d" preserveAspectRatio="xMidYMid slice"/>`,
			esc(r.Media(&Src{Kind: "generated", Ref: d.Background.Value}, "")), d.Size.W, d.Size.H)
	default: // color
		c := r.Color(d.Background.Value)
		if c == "" {
			c = "#FFFFFF"
		}
		fmt.Fprintf(b, `<rect width="%d" height="%d" fill="%s"/>`, d.Size.W, d.Size.H, esc(c))
	}
}

func writeImage(b *strings.Builder, el *Element, box LayoutBox, r Resolver) {
	url := r.Media(el.Src, el.Slot)
	if url == "" {
		return
	}
	fit := "xMidYMid slice" // cover
	if el.Transforms != nil && el.Transforms.Fit == "contain" {
		fit = "xMidYMid meet"
	}
	fmt.Fprintf(b, `<image href="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" preserveAspectRatio="%s"/>`,
		esc(url), box.Px.X, box.Px.Y, box.Px.W, box.Px.H, fit)
}

func writeShape(b *strings.Builder, el *Element, box LayoutBox, r Resolver) {
	fill := "#000000"
	if el.Style != nil && el.Style.Color != "" {
		fill = r.Color(el.Style.Color)
	}
	fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="12" fill="%s"/>`,
		box.Px.X, box.Px.Y, box.Px.W, box.Px.H, esc(fill))
}

func writeText(b *strings.Builder, el *Element, box LayoutBox, r Resolver) {
	st := el.Style
	if st == nil {
		st = &Style{}
	}
	color := r.Color(st.Color)
	if color == "" {
		color = "#111111"
	}
	family := r.Font(st.Font)
	align := st.Align
	if align == "" {
		align = "center"
	}
	scale := st.Scale
	if scale == 0 {
		scale = 1
	}
	// Deterministic font size derived from the element's box, scaled by the
	// style's scale factor. Exact glyph metrics are the render worker's job.
	fontSize := fontSizeBase(box) * scale
	weight := st.Weight
	if weight == "" {
		weight = "700"
	}

	lines := wrapText(el.Text, box.Px.W, fontSize)
	lineH := fontSize * 1.15
	totalH := lineH * float64(len(lines))
	startY := box.Px.Y + (box.Px.H-totalH)/2 + fontSize

	anchor, tx := "middle", box.Px.X+box.Px.W/2
	switch align {
	case "left":
		anchor, tx = "start", box.Px.X
	case "right":
		anchor, tx = "end", box.Px.X+box.Px.W
	}

	// Optional contrast scrim behind the text block (accessibility).
	if st.Scrim {
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="10" fill="rgba(0,0,0,0.35)"/>`,
			box.Px.X, startY-fontSize-8, box.Px.W, totalH+16)
	}

	for i, line := range lines {
		y := startY + float64(i)*lineH
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-family="%s" font-size="%.1f" font-weight="%s" fill="%s" text-anchor="%s">%s</text>`,
			tx, y, esc(family), fontSize, esc(weight), esc(color), anchor, esc(line))
	}
}

// wrapText is a deterministic greedy word-wrap using an average-character-width
// estimate (~0.55em). Exact metrics are the render worker's job; this keeps the
// backend dependency-free and good enough for layout/preview.
func wrapText(text string, boxW, fontSize float64) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	avgChar := fontSize * 0.55
	maxChars := int(boxW / avgChar)
	if maxChars < 4 {
		maxChars = 4
	}
	var lines []string
	cur := ""
	for _, w := range words {
		if cur == "" {
			cur = w
			continue
		}
		if len(cur)+1+len(w) <= maxChars {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func fontSizeBase(box LayoutBox) float64 {
	// Base a text size on the smaller of box height and a fraction of width.
	byH := box.Px.H * 0.9
	byW := box.Px.W * 0.18
	if byH < byW {
		return byH
	}
	return byW
}

func esc(s string) string { return html.EscapeString(s) }
