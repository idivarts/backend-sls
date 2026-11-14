// Package hfedit changes ONE element of a film's HTML, in place.
//
// ── The decision the plan said to make deliberately ───────────────────────
// Phase 13's note: "Editing HTML by string surgery is where this turns
// fragile. The honest options are a real parser or making authoring emit
// every editable element with a stable, addressable wrapper. Pick one
// deliberately. Regex over HTML will work for a month and then quietly
// corrupt someone's design."
//
// This is the parser, and it is the same shape as the render worker's
// html-edit.ts, deliberately: PARSE TO LOCATE, SPLICE BY BYTE OFFSET, NEVER
// SERIALISE. Two reasons that matter more than they sound:
//
//  1. A film is a hand-written document with a GSAP timeline in a <script>,
//     CSS with quotes and angle brackets in it, and attribute order the
//     author chose. Re-serialising a parse tree rewrites all of that —
//     every quote style, every bit of whitespace, the lot — so a one-word
//     text change would produce a diff across the whole file and a revision
//     nobody can read.
//  2. x/net/html is a CONFORMING parser: it inserts <html>, <head> and
//     <tbody>, moves stray content, and fixes up what it thinks is wrong.
//     Printing that back is a different document from the one the model
//     wrote and the one `check` passed.
//
// So the tokenizer walks the source, byte ranges are recorded, and the only
// thing ever written is the original bytes with a few spans replaced.
//
// ── Why not the worker, which already has this in TypeScript ──────────────
// The worker is an AWS Batch task. Changing a hex value must not cost a
// Fargate cold start, and the whole point of Phase 13 is that these edits
// are instant. So the same approach exists twice, in the two languages that
// need it, rather than one of them calling the other.
package hfedit

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// Element is one element located in the source.
type Element struct {
	// TagName, lowercased.
	TagName string
	// Attrs as parsed, keyed by lowercased name.
	Attrs map[string]string

	// Start is the byte offset of "<" in the start tag.
	Start int
	// TagEnd is the byte offset just past ">" of the start tag — also the
	// start of the children.
	TagEnd int
	// End is the byte offset just past the element: past "</tag>" for a
	// normal element, equal to TagEnd for a void or self-closed one.
	End int
}

// HasChildren is false for a void or self-closed element, where there is no
// text to replace.
func (e Element) HasChildren() bool { return e.End > e.TagEnd }

// Edit is a span of the source to replace.
type Edit struct {
	Start int
	End   int
	Text  string
}

// FindByID locates the element with this id.
//
// Returns false rather than an error when there is none: an id that is no
// longer in the film is an ordinary outcome — the user has the element
// selected from a revision that has since been rewritten — and the caller
// says so in those terms.
func FindByID(source, id string) (Element, bool) {
	if id == "" {
		return Element{}, false
	}
	for _, el := range findAll(source, func(e Element) bool { return e.Attrs["id"] == id }) {
		return el, true
	}
	return Element{}, false
}

// findAll walks the document and returns every element the predicate
// accepts, in document order.
//
// ── Why a tokenizer and not a tree ────────────────────────────────────────
// html.Parse builds a tree with no byte offsets on it, so there would be
// nothing to splice against. The tokenizer hands back the RAW bytes of each
// token, which is exactly the offset bookkeeping this needs.
func findAll(source string, accept func(Element) bool) []Element {
	z := html.NewTokenizer(strings.NewReader(source))
	var found []Element

	// Elements whose start tag has been seen and whose end tag has not.
	type open struct {
		el  Element
		idx int // index into `found`, or -1 when this element was not wanted
	}
	var stack []open
	offset := 0

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		raw := z.Raw()
		start := offset
		offset += len(raw)

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			attrs := map[string]string{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attrs[strings.ToLower(string(k))] = string(v)
			}
			el := Element{TagName: tag, Attrs: attrs, Start: start, TagEnd: offset, End: offset}

			// A void element — <img>, <br> — has no end tag to wait for, and
			// neither does one the author self-closed. Either way it is
			// complete here.
			if tt == html.SelfClosingTagToken || isVoid(tag) {
				if accept(el) {
					found = append(found, el)
				}
				continue
			}

			idx := -1
			if accept(el) {
				found = append(found, el)
				idx = len(found) - 1
			}
			stack = append(stack, open{el: el, idx: idx})

		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			// Close the nearest matching start tag, discarding anything
			// opened inside it that was never closed. A film is written by
			// a model; an unclosed <div> must not make every element after
			// it unaddressable.
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].el.TagName != tag {
					continue
				}
				if stack[i].idx >= 0 {
					found[stack[i].idx].End = offset
				}
				stack = stack[:i]
				break
			}
		}
	}
	return found
}

// SetAttribute produces the edit that gives an element this attribute.
//
// Rewrites the value in place when the attribute is there — so attribute
// order, which is the author's, survives — and inserts it just after the
// tag name when it is not.
func SetAttribute(el Element, source, name, value string) (Edit, error) {
	tag := source[el.Start:el.TagEnd]
	lower := strings.ToLower(name)
	if _, has := el.Attrs[lower]; has {
		s, e, ok := attrValueRange(tag, lower)
		if !ok {
			return Edit{}, fmt.Errorf("hfedit: %s is on <%s> but could not be located", name, el.TagName)
		}
		return Edit{Start: el.Start + s, End: el.Start + e, Text: escapeAttr(value)}, nil
	}
	// After "<tag", before whatever follows.
	at := el.Start + 1 + len(el.TagName)
	return Edit{Start: at, End: at, Text: fmt.Sprintf(` %s="%s"`, lower, escapeAttr(value))}, nil
}

// SetText produces the edit that replaces an element's children with text.
//
// The text is escaped, so a headline containing "<3" or "Tom & Jerry" is
// words and not markup. An element with no children — a void one — cannot
// hold text and is refused rather than silently ignored.
func SetText(el Element, text string) (Edit, error) {
	if !el.HasChildren() {
		return Edit{}, fmt.Errorf("hfedit: <%s> cannot hold text", el.TagName)
	}
	closing := len("</" + el.TagName + ">")
	end := el.End - closing
	if end < el.TagEnd {
		return Edit{}, fmt.Errorf("hfedit: <%s> has no body to replace", el.TagName)
	}
	return Edit{Start: el.TagEnd, End: end, Text: escapeText(text)}, nil
}

// Apply splices the edits into the source.
//
// Back to front, so each edit's offsets still refer to the string it was
// measured against. Overlapping edits are refused rather than resolved:
// two instructions about the same bytes mean the caller built them from
// different reads, and guessing which wins is how a design gets quietly
// corrupted.
func Apply(source string, edits []Edit) (string, error) {
	if len(edits) == 0 {
		return source, nil
	}
	ordered := make([]Edit, len(edits))
	copy(ordered, edits)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	for i := 1; i < len(ordered); i++ {
		if ordered[i].Start < ordered[i-1].End {
			return "", fmt.Errorf("hfedit: two edits overlap at byte %d", ordered[i].Start)
		}
	}
	var b strings.Builder
	prev := 0
	for _, e := range ordered {
		if e.Start < prev || e.End > len(source) || e.Start > e.End {
			return "", fmt.Errorf("hfedit: an edit points outside the document")
		}
		b.WriteString(source[prev:e.Start])
		b.WriteString(e.Text)
		prev = e.End
	}
	b.WriteString(source[prev:])
	return b.String(), nil
}

// attrValueRange finds where an attribute's VALUE sits inside a start tag,
// as offsets relative to the tag.
//
// Hand-walked rather than regexped for the reason the whole package exists:
// an attribute value can contain anything, including the name of another
// attribute, and a pattern that matches `style="…"` will eventually match
// the word style inside a CSS string.
func attrValueRange(tag, name string) (int, int, bool) {
	i := 1 // past "<"
	// Skip the tag name.
	for i < len(tag) && !isSpace(tag[i]) && tag[i] != '>' && tag[i] != '/' {
		i++
	}
	for i < len(tag) {
		for i < len(tag) && isSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] == '>' || tag[i] == '/' {
			return 0, 0, false
		}
		nameStart := i
		for i < len(tag) && !isSpace(tag[i]) && tag[i] != '=' && tag[i] != '>' && tag[i] != '/' {
			i++
		}
		got := strings.ToLower(tag[nameStart:i])
		for i < len(tag) && isSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			// A bare attribute, like `hidden`. It has no value range.
			if got == name {
				return 0, 0, false
			}
			continue
		}
		i++ // past "="
		for i < len(tag) && isSpace(tag[i]) {
			i++
		}
		if i >= len(tag) {
			return 0, 0, false
		}
		switch q := tag[i]; q {
		case '"', '\'':
			i++
			valStart := i
			for i < len(tag) && tag[i] != q {
				i++
			}
			if got == name {
				return valStart, i, true
			}
			if i < len(tag) {
				i++ // past the closing quote
			}
		default:
			valStart := i
			for i < len(tag) && !isSpace(tag[i]) && tag[i] != '>' {
				i++
			}
			if got == name {
				return valStart, i, true
			}
		}
	}
	return 0, 0, false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// isVoid reports whether a tag has no end tag. HTML's own list.
func isVoid(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// escapeAttr escapes what would end a double-quoted attribute value.
func escapeAttr(v string) string {
	return strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace(v)
}

// escapeText escapes what would start a tag or an entity.
func escapeText(v string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(v)
}
