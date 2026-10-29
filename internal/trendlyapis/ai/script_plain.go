package ai

import (
	"regexp"
	"strings"
)

// The script field is rich text. The brand app edits it in a RichTextEditor,
// so what reaches the validator and the judge is HTML — `<strong>VO:</strong>`
// where every check in script_validate.go expects `**VO:**`.
//
// This was invisible for the whole of Phase 1 because the corpus in
// testdata/devscripts came from the model's streamed markdown, before any
// editor touched it. A script the user has typed in or revised arrives
// tag-wrapped, and every structural signal the validator reads — the label on
// a line, the bullet, the heading — is spelled differently.
//
// So HTML is mapped back to the markdown dialect the validator already
// normalises, rather than teaching each check a second spelling. The mapping
// is deliberately lossy in one direction only: it never invents structure that
// was not in the markup.
//
// plainScript is IDEMPOTENT on plain markdown, which is the property that keeps
// the Phase 1 corpus meaningful — script_plain_test.go asserts it over every
// file in that corpus.

// Anything that looks like a tag or an entity. Cheap pre-check: a plain
// markdown script matches neither, and returns untouched.
var reLooksLikeHTML = regexp.MustCompile(`</?[a-zA-Z][^>]*>|&[a-zA-Z#][a-zA-Z0-9]{1,8};`)

var (
	// Block boundaries become a blank line: in markdown that is what separates
	// one beat from the next, and the section split depends on it.
	reBlockEnd = regexp.MustCompile(`(?i)</(p|div|h[1-6]|blockquote|section|article|figure|table|tr|ul|ol)\s*>`)
	// A line break inside a block is one newline, not a new block. `</li>` is
	// deliberately absent: the next `<li>` already opens its own line, and
	// closing one here too would put a blank line between every bullet.
	reLineBreak = regexp.MustCompile(`(?i)<br\s*/?>`)
	// Headings carry their level across, so reStructuralLine still sees them.
	reHeadingOpen = regexp.MustCompile(`(?i)<h([1-6])[^>]*>`)
	reListItem    = regexp.MustCompile(`(?i)<li[^>]*>`)
	reBoldTag     = regexp.MustCompile(`(?i)</?(strong|b)\s*>`)
	reItalicTag   = regexp.MustCompile(`(?i)</?(em|i)\s*>`)
	// Everything else: comments, spans, attributes, anchors, images.
	reAnyTag = regexp.MustCompile(`(?s)<!--.*?-->|</?[a-zA-Z][^>]*>`)
	// Three or more newlines read as one blank line.
	reBlankRun    = regexp.MustCompile(`\n{3,}`)
	reTrailingWS  = regexp.MustCompile(`[ \t]+\n`)
	reSpaceRun    = regexp.MustCompile(`[ \t]{2,}`)
	reEmptyMarker = regexp.MustCompile(`(?m)^\s*(\*\*|\*)\s*$`)
)

// Entities are decoded LAST, so text that legitimately contains `&lt;p&gt;`
// is never mistaken for a tag and stripped.
var entities = strings.NewReplacer(
	"&nbsp;", " ",
	"&#160;", " ",
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", `"`,
	"&#34;", `"`,
	"&apos;", "'",
	"&#39;", "'",
	"&rsquo;", "’",
	"&lsquo;", "‘",
	"&ldquo;", "“",
	"&rdquo;", "”",
	"&mdash;", "—",
	"&ndash;", "–",
	"&hellip;", "…",
	// Last: an &amp; that encoded one of the above must not decode twice.
	"&amp;", "&",
)

// plainScript renders a rich-text script as the markdown the validator reads.
// Plain input is returned unchanged.
func plainScript(s string) string {
	if s == "" || !reLooksLikeHTML.MatchString(s) {
		return s
	}

	out := reBlockEnd.ReplaceAllString(s, "\n\n")
	out = reLineBreak.ReplaceAllString(out, "\n")
	out = reHeadingOpen.ReplaceAllStringFunc(out, func(m string) string {
		level := reHeadingOpen.FindStringSubmatch(m)[1]
		n := int(level[0] - '0')
		return "\n\n" + strings.Repeat("#", n) + " "
	})
	out = reListItem.ReplaceAllString(out, "\n- ")
	out = reBoldTag.ReplaceAllString(out, "**")
	out = reItalicTag.ReplaceAllString(out, "*")
	out = reAnyTag.ReplaceAllString(out, "")
	out = entities.Replace(out)

	// An empty block leaves a bare `**` or `*` on its own line — a marker with
	// nothing marked, which reEmphasis would otherwise read as a label.
	out = reEmptyMarker.ReplaceAllString(out, "")
	out = reSpaceRun.ReplaceAllString(out, " ")
	out = reTrailingWS.ReplaceAllString(out, "\n")
	out = reBlankRun.ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}
