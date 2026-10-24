package ai

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Deterministic checks on an AI-authored design, run before it is persisted.
//
// This is the cheapest quality lever in the design loop, and the only one that
// costs no tokens: a model that gets "scene 2 has no data-duration" back as a
// tool error fixes it on the next step, with nobody rendering anything and
// nobody looking at a picture. It catches the class of mistake that is
// MECHANICAL — a missing attribute, an animation that cannot survive the seek,
// a scene whose motion outlasts the scene — and leaves taste to the render loop.
//
// It is deliberately a GATE, not a linter: a design that fails is not written,
// so a broken revision never reaches the user's screen or the publish path. The
// rules mirror what the generate_design tool description promises, which is the
// only reason the model can be expected to satisfy them.

// designProblem is one failed check, phrased as an instruction the model can act
// on rather than a diagnostic for a developer.
type designProblem string

var (
	// `animation: name 1.2s ease-out 0.3s both` — shorthand carries the times
	// positionally: the FIRST time is duration, the second (if any) is delay.
	reAnimShorthand = regexp.MustCompile(`(?i)animation\s*:\s*([^;}"']+)`)
	reAnimDuration  = regexp.MustCompile(`(?i)animation-duration\s*:\s*([^;}"']+)`)
	reAnimDelay     = regexp.MustCompile(`(?i)animation-delay\s*:\s*([^;}"']+)`)
	// A CSS time: 1.2s / 400ms. Captures the number and its unit.
	reTime = regexp.MustCompile(`(?i)(-?\d*\.?\d+)\s*(ms|s)\b`)
	// `transition: …` / `transition-property: …`, which the seek cannot replay.
	reTransition = regexp.MustCompile(`(?i)\btransition(-[a-z]+)?\s*:`)
	reInfinite   = regexp.MustCompile(`(?i)\banimation(-iteration-count)?\s*:[^;}"']*\binfinite\b`)
	// A declaration of exactly `transform` or `opacity` at the start of a style
	// attribute or after a semicolon. Anchored so `transform-origin` and
	// `-webkit-transform` don't trip it.
	reOwnedTransform = regexp.MustCompile(`(?i)(^|;)\s*transform\s*:`)
	reOwnedOpacity   = regexp.MustCompile(`(?i)(^|;)\s*opacity\s*:`)
	// Scene transitions the player knows how to composite.
	validTransitions = map[string]bool{"cut": true, "fade": true, "push": true, "slide": true}
)

// cssTimeMs parses the first CSS time in s and returns it in milliseconds.
func cssTimeMs(s string) (float64, bool) {
	m := reTime.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	if strings.EqualFold(m[2], "s") {
		return v * 1000, true
	}
	return v, true
}

// animationEndMs is the latest moment any animation declared in `css` is still
// running, in ms. The longhand properties win when both forms appear, matching
// the cascade for a single declaration block.
func animationEndMs(css string) (float64, bool) {
	var duration, delay float64
	var found bool

	// Shorthand: the first time is duration, a second time is delay.
	for _, m := range reAnimShorthand.FindAllStringSubmatch(css, -1) {
		times := reTime.FindAllString(m[1], -1)
		if len(times) > 0 {
			if v, ok := cssTimeMs(times[0]); ok {
				duration, found = maxF(duration, v), true
			}
		}
		if len(times) > 1 {
			if v, ok := cssTimeMs(times[1]); ok {
				delay, found = maxF(delay, v), true
			}
		}
	}
	for _, m := range reAnimDuration.FindAllStringSubmatch(css, -1) {
		if v, ok := cssTimeMs(m[1]); ok {
			duration, found = maxF(duration, v), true
		}
	}
	for _, m := range reAnimDelay.FindAllStringSubmatch(css, -1) {
		if v, ok := cssTimeMs(m[1]); ok {
			delay, found = maxF(delay, v), true
		}
	}
	return duration + delay, found
}

func maxF(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

func attr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

func hasAttr(n *html.Node, name string) bool {
	_, ok := attr(n, name)
	return ok
}

// slideNodes returns every [data-slide] element, in document order.
func slideNodes(root *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && hasAttr(n, "data-slide") {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

// styleTextIn concatenates every inline style attribute and <style> block
// inside n. The seek-safety and timing checks run against this.
func styleTextIn(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode {
			if s, ok := attr(x, "style"); ok {
				sb.WriteString(s)
				sb.WriteString(";")
			}
			if x.Data == "style" {
				for c := x.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						sb.WriteString(c.Data)
					}
				}
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// hasMeaningfulText reports whether n holds text of its own (not just in a
// descendant that is itself addressable).
func hasMeaningfulText(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
			return true
		}
	}
	return false
}

// validateDesignHTML checks a design against the contract generate_design states.
//
// docType is "image" or "video"; wantSlides is the slide count the model said it
// was producing (0 when it did not say). An empty result means the design is
// structurally sound — it says nothing about whether it looks good, which is
// what render_design_preview is for.
func validateDesignHTML(htmlDoc, docType string, wantSlides int) []designProblem {
	root, err := html.Parse(strings.NewReader(htmlDoc))
	if err != nil {
		return []designProblem{designProblem("the html could not be parsed: " + err.Error())}
	}

	var problems []designProblem
	add := func(format string, args ...any) {
		problems = append(problems, designProblem(fmt.Sprintf(format, args...)))
	}

	slides := slideNodes(root)
	if len(slides) == 0 {
		add("no [data-slide] section found — every design must wrap its slides in " +
			"[data-carousel] with at least one <section data-slide=\"0\"> inside it")
		return problems
	}
	if wantSlides > 0 && len(slides) != wantSlides {
		add("you said slides=%d but the html contains %d [data-slide] sections — "+
			"make them agree", wantSlides, len(slides))
	}

	// Anything addressable must carry a data-el, or the user cannot tap it to
	// pin a comment and the text edit path has nothing to anchor on.
	if countTextElements(root, false) > 0 && countTextElements(root, true) == 0 {
		add("no element carries a data-el attribute — give every editable text " +
			"element a unique data-el=\"<id>\" so the user can select and comment on it")
	}

	for i, slide := range slides {
		css := styleTextIn(slide)

		// The player writes transform and opacity on the slide element itself to
		// composite transitions, and clears them on the next seek. A design that
		// sets either inline does not merely lose it — it loses it unpredictably,
		// depending on whether the frame fell inside a transition.
		if own, ok := attr(slide, "style"); ok {
			if reOwnedTransform.MatchString(own) {
				add("scene %d sets `transform` inline on the slide element. The player "+
					"owns that property to composite transitions and will overwrite it — "+
					"move the transform onto an element inside the scene", i)
			}
			if reOwnedOpacity.MatchString(own) {
				add("scene %d sets `opacity` inline on the slide element. The player owns "+
					"that property to cross-fade scenes and will overwrite it — put the "+
					"opacity on an element inside the scene", i)
			}
		}

		// A misspelled transition is silently treated as a cut by the player, so
		// the design would just quietly lose the join it asked for.
		if kind, ok := attr(slide, "data-transition"); ok {
			k := strings.ToLower(strings.TrimSpace(kind))
			if !validTransitions[k] {
				add("scene %d has data-transition=%q, which is not a transition the player "+
					"knows. Use cut, fade, push or slide", i, kind)
			}
			if i == 0 && k != "cut" && k != "" {
				add("scene 0 has a transition, but the first scene has nothing to enter " +
					"from — it is always a cut. Remove data-transition from it")
			}
		}

		// The seek pauses animations and sets currentTime; a transition has no
		// timeline to seek, so it renders as its end state or not at all.
		if loc := reTransition.FindString(css); loc != "" {
			add("scene %d uses a CSS %s — transitions do not render, because every "+
				"frame is produced by seeking paused animations. Express it as an "+
				"@keyframes animation instead", i, strings.TrimSuffix(strings.TrimSpace(loc), ":"))
		}
		if reInfinite.MatchString(css) {
			add("scene %d has an infinite animation — there is no stable frame to "+
				"capture. Give it a finite duration that ends inside the scene", i)
		}

		if docType != "video" {
			continue
		}

		raw, ok := attr(slide, "data-duration")
		if !ok || strings.TrimSpace(raw) == "" {
			add("scene %d has no data-duration — it will silently fall back to "+
				"3000ms and desynchronise the video. Set it explicitly", i)
			continue
		}
		dur, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || dur <= 0 {
			add("scene %d has data-duration=%q, which is not a positive number of "+
				"milliseconds", i, raw)
			continue
		}
		if end, found := animationEndMs(css); found && end > float64(dur) {
			add("scene %d: its animation runs to %.0fms (delay + duration) but the "+
				"scene is only %dms long, so the motion is cut off mid-flight. Either "+
				"shorten the animation or lengthen data-duration", i, end, dur)
		}
	}
	return problems
}

// countTextElements counts elements holding their own text. withDataEl narrows
// it to those that are addressable.
func countTextElements(root *html.Node, withDataEl bool) int {
	n := 0
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode && x.Data != "style" && x.Data != "script" && hasMeaningfulText(x) {
			if !withDataEl || hasAttr(x, "data-el") {
				n++
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return n
}

// designProblemsResult renders failed checks as the tool's answer. It is an
// ok:false so the model treats it as work to redo, and it lists every problem
// at once rather than one per round-trip.
func designProblemsResult(problems []designProblem) string {
	list := make([]string, 0, len(problems))
	for _, p := range problems {
		list = append(list, string(p))
	}
	return jsonResult(map[string]any{
		"ok":       false,
		"error":    "the design was not saved — it breaks the rules below",
		"problems": list,
		"note": "fix every problem listed and call the same tool again with the " +
			"corrected html. Nothing was shown to the user.",
	})
}
