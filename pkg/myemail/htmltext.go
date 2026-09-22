package myemail

import (
	"html"
	"regexp"
	"strings"
)

// Mail clients and spam filters both penalise HTML-only messages, so every send
// carries a text/plain alternative derived from the rendered template. This is a
// pragmatic converter for the templates in templates/*.html — not a general
// purpose HTML renderer.
//
// Line breaks are carried through the pipeline as a NUL sentinel so that the
// newlines the source HTML happens to be indented with can be collapsed away
// without losing the breaks that <br> and block elements actually imply.
const lineSentinel = "\x00"

var (
	htmlCommentRe = regexp.MustCompile(`(?is)<!--.*?-->`)
	// One regex per tag: RE2 has no backreferences, so `<(script|style)>…</\1>`
	// is not expressible.
	htmlDropBlockRes = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script\b[^>]*>.*?</\s*script\s*>`),
		regexp.MustCompile(`(?is)<style\b[^>]*>.*?</\s*style\s*>`),
		regexp.MustCompile(`(?is)<head\b[^>]*>.*?</\s*head\s*>`),
		regexp.MustCompile(`(?is)<title\b[^>]*>.*?</\s*title\s*>`),
	}
	htmlAnchorRe    = regexp.MustCompile(`(?is)<a\b[^>]*?href\s*=\s*["']([^"']*)["'][^>]*>(.*?)</\s*a\s*>`)
	htmlLineBreakRe = regexp.MustCompile(`(?i)<\s*br\b[^>]*>`)
	htmlBlockRe     = regexp.MustCompile(`(?i)<\s*/?\s*(p|div|tr|li|h[1-6]|table|ul|ol|blockquote|section|header|footer)\b[^>]*>`)
	htmlTagRe       = regexp.MustCompile(`(?s)<[^>]*>`)
	whitespaceRe    = regexp.MustCompile(`[ \t\r\n\f\v]+`)
	sentinelRunRe   = regexp.MustCompile(`[ \t]*\x00[ \t\x00]*`)
)

func htmlToText(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	out := htmlCommentRe.ReplaceAllString(input, "")
	for _, re := range htmlDropBlockRes {
		out = re.ReplaceAllString(out, "")
	}

	// Keep CTA destinations — the templates lean heavily on button links, which
	// would otherwise vanish entirely from the plain-text part.
	out = htmlAnchorRe.ReplaceAllStringFunc(out, func(match string) string {
		groups := htmlAnchorRe.FindStringSubmatch(match)
		if len(groups) != 3 {
			return match
		}
		href := strings.TrimSpace(groups[1])
		label := strings.TrimSpace(html.UnescapeString(htmlTagRe.ReplaceAllString(groups[2], "")))
		label = strings.TrimSpace(whitespaceRe.ReplaceAllString(label, " "))

		switch {
		case href == "" || strings.HasPrefix(href, "#"):
			return label
		case label == "" || label == href:
			return href
		default:
			return label + " (" + href + ")"
		}
	})

	// A <br> is a single break; a block boundary is a paragraph break.
	out = htmlLineBreakRe.ReplaceAllString(out, lineSentinel)
	out = htmlBlockRe.ReplaceAllString(out, lineSentinel+lineSentinel)
	out = htmlTagRe.ReplaceAllString(out, "")
	out = html.UnescapeString(out)
	out = strings.ReplaceAll(out, " ", " ")

	// Collapse the source's own indentation and wrapping, then restore only the
	// breaks the markup asked for.
	out = whitespaceRe.ReplaceAllString(out, " ")
	out = sentinelRunRe.ReplaceAllStringFunc(out, func(run string) string {
		if strings.Count(run, lineSentinel) > 1 {
			return "\n\n"
		}
		return "\n"
	})

	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
