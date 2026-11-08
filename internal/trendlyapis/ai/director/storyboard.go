package director

import (
	"fmt"
	"sort"
	"strings"
)

// Storyboard renders a plan the way a person reviewing it needs to see it:
// the shape first, then per scene what it is for and what it costs, then the
// money. This is the terminal stand-in for the storyboard UI, and writing it
// first is useful on its own — if a plan does not read well here it will not
// read well as cards either.
func Storyboard(p Plan, m Manifest, problems []Problem) string {
	var b strings.Builder
	line := strings.Repeat("─", 78)

	fmt.Fprintf(&b, "%s\n", line)
	fmt.Fprintf(&b, "%s · %s\n", strings.ToUpper(m.Content.Format), m.Content.Title)
	fmt.Fprintf(&b, "%s\n", line)
	fmt.Fprintf(&b, "%s\n\n", wrap(p.Summary, 78))

	isCarousel := m.Content.Format == "carousel" || m.Content.Format == "post"
	if isCarousel {
		fmt.Fprintf(&b, "%d slides · %dx%d\n", len(p.Scenes), p.Canvas.Width, p.Canvas.Height)
	} else {
		length := fmt.Sprintf("%.1fs", float64(p.EffectiveTotalMs())/1000)
		// Say both numbers when they differ: a timeline whose scenes follow
		// their narration is longer than its planned durations suggest, and
		// the planned figure is the one that misleads.
		if p.EffectiveTotalMs() != p.TotalMs() {
			length = fmt.Sprintf("%.1fs once narrated (%.1fs planned)",
				float64(p.EffectiveTotalMs())/1000, float64(p.TotalMs())/1000)
		}
		fmt.Fprintf(&b, "%d scenes · %s · %dx%d @%dfps · %s transitions\n",
			len(p.Scenes), length, p.Canvas.Width, p.Canvas.Height,
			p.Canvas.Fps, p.TransitionPreset)
	}
	if p.Audio.MusicApproach != "" && p.Audio.MusicApproach != "none" {
		fmt.Fprintf(&b, "music: %s", p.Audio.MusicApproach)
		if p.Audio.MusicPrompt != "" {
			fmt.Fprintf(&b, " — %s", p.Audio.MusicPrompt)
		}
		b.WriteString("\n")
	}
	if p.Audio.VoiceModel != "" {
		fmt.Fprintf(&b, "voice: %s (duck music: %v)\n", voiceLine(p.Audio), p.Audio.DuckMusic)
	}
	b.WriteString("\n")

	// The cast is printed before the scenes because the anchor still is the
	// thing a person approves FIRST — a face is far easier to judge as one
	// still than as four clips, and it is the cheapest quality gate here.
	if len(p.Cast) > 0 || len(p.Locations) > 0 {
		b.WriteString("CAST AND PLACES\n")
		for _, c := range p.Cast {
			anchor := "NO ANCHOR"
			switch {
			case c.AnchorMediaID != "":
				anchor = "anchor " + c.AnchorMediaID
			case c.AnchorAssetID != "":
				anchor = "anchor " + c.AnchorAssetID + " (to be made)"
			}
			fmt.Fprintf(&b, "  %-12s %-34s %s\n", c.ID, shorten(c.Description, 34), anchor)
			if c.Look != "" {
				fmt.Fprintf(&b, "  %-12s look: %s\n", "", shorten(c.Look, 56))
			}
		}
		for _, l := range p.Locations {
			anchor := ""
			switch {
			case l.AnchorMediaID != "":
				anchor = "anchor " + l.AnchorMediaID
			case l.AnchorAssetID != "":
				anchor = "anchor " + l.AnchorAssetID + " (to be made)"
			}
			fmt.Fprintf(&b, "  %-12s %-34s %s\n", l.ID, shorten(l.Description, 34), anchor)
		}
		b.WriteString("\n")
	}

	reqByID := map[string]AssetRequest{}
	for _, ar := range p.AssetRequests {
		reqByID[ar.ID] = ar
	}
	// Prices come from the registry, not from the plan. Showing a figure the
	// planner made up would be worse than showing none.
	price, totalPrice := p.DerivedCosts(m)

	for i, s := range p.Scenes {
		head := fmt.Sprintf("%d. %s", i+1, s.Beat)
		if !isCarousel {
			head = fmt.Sprintf("%d. [%.1fs] %s", i+1, float64(s.DurationMs)/1000, s.Beat)
		}
		fmt.Fprintf(&b, "%s\n", head)
		meta := "   " + s.Approach
		if !isCarousel && i > 0 {
			meta += fmt.Sprintf(" · enters %s", s.TransitionIn.Kind)
			if s.TransitionIn.Ms > 0 {
				meta += fmt.Sprintf(" %dms", s.TransitionIn.Ms)
			}
		}
		if s.FitToNarration {
			meta += " · fits narration"
		}
		fmt.Fprintf(&b, "%s\n", meta)

		for _, l := range s.Layers {
			switch {
			case l.ReuseMediaID != "":
				desc := l.ReuseMediaID
				if it, ok := m.MediaByID(l.ReuseMediaID); ok {
					desc = fmt.Sprintf("%s (%s)", it.Describes, it.ID)
				}
				fmt.Fprintf(&b, "   · %-11s reuse  %s\n", l.Slot, desc)
			case l.AssetRequestID != "":
				ar := reqByID[l.AssetRequestID]
				tag := ar.Kind
				if ar.DurationS > 0 {
					tag = fmt.Sprintf("%s %ds", ar.Kind, ar.DurationS)
				}
				chain := ""
				if ar.StartFrameFromAssetID != "" {
					chain = fmt.Sprintf(" ← continues %s", ar.StartFrameFromAssetID)
				}
				// Naming the mechanism on the line is the point of this
				// section: a reader sees at a glance whether a face is held
				// by an anchor, by a chain, or by nothing at all.
				if l.TrimInMs != 0 || l.TrimOutMs != 0 {
					chain += fmt.Sprintf(" · trim %.1f-%.1fs of one clip", float64(l.TrimInMs)/1000, float64(l.TrimOutMs)/1000)
				}
				for _, cid := range ar.CastIDs {
					for _, c := range p.Cast {
						if c.ID != cid {
							continue
						}
						if hasRef(ar, c) {
							chain += " · anchored on " + cid
						} else {
							chain += " · " + cid + " UNANCHORED"
						}
					}
				}
				if ar.NativeAudio {
					chain += " · native audio ON"
				}
				fmt.Fprintf(&b, "   · %-11s %-6s %s$%.2f%s\n", l.Slot, tag,
					shorten(ar.Intent, 42)+"  ", price[ar.ID], chain)
			case l.HTMLBrief != "":
				fmt.Fprintf(&b, "   · %-11s type   %s\n", l.Slot, shorten(l.HTMLBrief, 54))
			}
		}
		if s.Narration != "" {
			tag := ""
			// Show which request speaks this line, so a single narration blob
			// is obvious on sight rather than only in validation.
			for _, ar := range p.AssetRequests {
				if ar.Kind == "voice" && ar.SceneID == s.ID {
					tag = fmt.Sprintf("  [%s]", ar.ID)
					break
				}
			}
			// How long the line takes to say, where that is not simply the
			// scene's length: a voice overrunning its picture is the
			// commonest way a render comes out wrong.
			if need := NarrationMs(s.Narration); need > s.DurationMs && s.DurationMs > 0 {
				fit := "OVERRUNS"
				if s.FitToNarration {
					fit = "scene stretches to fit"
				}
				tag += fmt.Sprintf("  ~%.1fs to say, %s", float64(need)/1000, fit)
			}
			fmt.Fprintf(&b, "   “%s”%s\n", shorten(s.Narration, 56), tag)
		}
		if s.Why != "" {
			fmt.Fprintf(&b, "   why: %s\n", wrapIndent(s.Why, 70, "        "))
		}
		b.WriteString("\n")
	}

	// What the run will actually do, in dependency order, because "what runs
	// in parallel" is the question a person asks next.
	par, seq := lanes(p)
	fmt.Fprintf(&b, "%s\n", line)
	fmt.Fprintf(&b, "RUN · %d requests · %d can start at once, %d wait on a chain\n", len(p.AssetRequests), par, seq)
	for _, ar := range p.AssetRequests {
		dep := "parallel"
		if ar.StartFrameFromAssetID != "" {
			dep = "after " + ar.StartFrameFromAssetID
		}
		if ar.SceneID != "" {
			dep = "scene " + ar.SceneID
		}
		fmt.Fprintf(&b, "  %-5s %-6s %-9s $%-6.2f %s\n", ar.ID, ar.Kind, dep, price[ar.ID], shorten(ar.Intent, 40))
	}

	fmt.Fprintf(&b, "%s\n", line)
	fmt.Fprintf(&b, "COST  $%.2f of a $%.2f cap (wallet $%.2f, mode %s) · priced from the registry\n",
		totalPrice, m.Budget.RunCapUSD, m.Budget.WalletUSD, m.Budget.Mode)

	if len(p.Notes) > 0 {
		b.WriteString("\nNOTES\n")
		for _, n := range p.Notes {
			fmt.Fprintf(&b, "  · %s\n", wrapIndent(n, 74, "    "))
		}
	}

	if len(problems) > 0 {
		fmt.Fprintf(&b, "\n%s\nVALIDATION\n", line)
		for _, pr := range problems {
			fmt.Fprintf(&b, "  %s\n", pr)
		}
		if Fatal(problems) {
			b.WriteString("\n  → would NOT run. Nothing is charged for a plan that fails here.\n")
		} else {
			b.WriteString("\n  → would run. Warnings are shown on the storyboard, not blocking.\n")
		}
	} else {
		fmt.Fprintf(&b, "\nVALIDATION  clean — this plan would run as written.\n")
	}
	return b.String()
}

func lanes(p Plan) (parallel, sequential int) {
	for _, ar := range p.AssetRequests {
		if ar.StartFrameFromAssetID == "" {
			parallel++
		} else {
			sequential++
		}
	}
	return
}

// ToolGaps lists what the plan relies on that is not built yet, grouped by
// phase. The lab is also a way of finding out what to build next.
func ToolGaps(inv map[string][]Tool) string {
	byPhase := map[string][]string{}
	for group, ts := range inv {
		for _, t := range ts {
			if t.Status == ToolExists {
				continue
			}
			ph := t.Phase
			if ph == "" {
				ph = "?"
			}
			byPhase[ph] = append(byPhase[ph], fmt.Sprintf("%s (%s, %s)", t.Name, t.Status, group))
		}
	}
	phases := make([]string, 0, len(byPhase))
	for p := range byPhase {
		phases = append(phases, p)
	}
	sort.Strings(phases)

	var b strings.Builder
	for _, p := range phases {
		fmt.Fprintf(&b, "  phase %s\n", p)
		sort.Strings(byPhase[p])
		for _, t := range byPhase[p] {
			fmt.Fprintf(&b, "    · %s\n", t)
		}
	}
	return b.String()
}

func shorten(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) <= n {
		return s
	}
	if n < 4 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func wrap(s string, n int) string { return wrapIndent(s, n, "") }

func wrapIndent(s string, n int, indent string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > n {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	lines = append(lines, cur)
	return strings.Join(lines, "\n"+indent)
}

// voiceLine names the model AND the voice id, because the id is the thing that
// actually keeps the voice the same from line to line.
func voiceLine(a AudioPlan) string {
	if a.VoiceID == "" {
		return a.VoiceModel + " (no voice id — every line a different voice)"
	}
	return a.VoiceModel + " / " + a.VoiceID
}
