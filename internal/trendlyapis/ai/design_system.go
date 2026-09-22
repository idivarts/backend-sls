package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// The Brand Design System is a brand's single, declared design/brand standard.
// It reaches the AI in three complementary ways, all built from the one document:
//
//	designSystemPromptBlock  → compact prose in the system prompt (identity,
//	                           voice & tone, content rules, imagery, platform tone)
//	brandKitCSS              → :root CSS tokens for the design tools (colors/fonts/logo)
//	designSystemImagery…     → mood keywords + negatives folded into generate_image
//
// Every section is optional; only populated parts are rendered, so a brand with a
// half-filled Design System still gets exactly the context it has defined.

// maxDesignSystemChars caps the rendered design-system block so it never crowds
// out brand memory / module context in the system prompt.
const maxDesignSystemChars = 4000

// loadDesignSystem reads the brand's Design System, returning nil when the brand
// has none or on error (the AI simply runs without it, as it does today).
func loadDesignSystem(brandID string) *trendlymodels.BrandDesignSystem {
	if brandID == "" {
		return nil
	}
	ds, err := trendlymodels.GetDesignSystem(context.Background(), brandID)
	if err != nil {
		return nil
	}
	return ds
}

// designSystemPromptBlock renders the design system into a compact, model-friendly
// block for the system prompt. Returns "" when nothing is populated.
func designSystemPromptBlock(ds *trendlymodels.BrandDesignSystem) string {
	if ds == nil {
		return ""
	}
	var parts []string

	// Identity & positioning.
	if id := ds.Identity; id != nil {
		var lines []string
		add := func(label, val string) {
			if strings.TrimSpace(val) != "" {
				lines = append(lines, label+": "+strings.TrimSpace(val))
			}
		}
		addList := func(label string, vals []string) {
			if v := joinNonEmpty(vals, ", "); v != "" {
				lines = append(lines, label+": "+v)
			}
		}
		add("Tagline", id.Tagline)
		add("Mission", id.Mission)
		add("Target audience", id.Audience)
		addList("Personality", id.Personality)
		addList("Value props", id.ValueProps)
		addList("Competitors", id.Competitors)
		if len(lines) > 0 {
			parts = append(parts, "Identity:\n"+strings.Join(lines, "\n"))
		}
	}

	// Voice & tone (supersedes the legacy Brand.AIVoice string).
	if v := ds.Voice; v != nil {
		var lines []string
		if s := joinNonEmpty(v.Adjectives, ", "); s != "" {
			lines = append(lines, "Adjectives: "+s)
		}
		if tone := toneWords(v.Tone); tone != "" {
			lines = append(lines, "Tone: "+tone)
		}
		if s := strings.TrimSpace(v.POV); s != "" {
			lines = append(lines, "Point of view: "+povWords(s))
		}
		if s := strings.TrimSpace(v.EmojiPolicy); s != "" {
			lines = append(lines, "Emoji: "+emojiWords(s))
		}
		if s := strings.TrimSpace(v.ReadingLevel); s != "" {
			lines = append(lines, "Reading level: "+s)
		}
		if s := joinNonEmpty(v.SamplePhrases, " / "); s != "" {
			lines = append(lines, "Sample phrases: "+s)
		}
		if s := strings.TrimSpace(v.Guidelines); s != "" {
			lines = append(lines, "Notes: "+s)
		}
		if len(lines) > 0 {
			parts = append(parts, "Voice & tone (write everything in this voice):\n"+strings.Join(lines, "\n"))
		}
	}

	// Content rules & guardrails.
	if r := ds.Rules; r != nil {
		var lines []string
		addList := func(label string, vals []string) {
			if v := joinNonEmpty(vals, ", "); v != "" {
				lines = append(lines, label+": "+v)
			}
		}
		if s := joinNonEmpty(r.BannedWords, ", "); s != "" {
			lines = append(lines, "NEVER use these words/phrases: "+s)
		}
		addList("Prefer these terms", r.PreferredTerms)
		addList("Approved claims (only these)", r.ApprovedClaims)
		addList("Always include disclaimers", r.Disclaimers)
		addList("Always mention", r.RequiredMentions)
		if h := r.Hashtag; h != nil {
			if s := joinNonEmpty(h.Branded, " "); s != "" {
				lines = append(lines, "Branded hashtags: "+s)
			}
			if s := joinNonEmpty(h.Banned, " "); s != "" {
				lines = append(lines, "Never use hashtags: "+s)
			}
			if h.MaxCount != nil {
				lines = append(lines, fmt.Sprintf("Max hashtags: %d", *h.MaxCount))
			}
		}
		if s := strings.TrimSpace(r.CTAStyle); s != "" {
			lines = append(lines, "CTA style: "+s)
		}
		if s := strings.TrimSpace(r.Grammar); s != "" {
			lines = append(lines, "Grammar/style: "+s)
		}
		addList("Do", r.Dos)
		addList("Don't", r.Donts)
		if len(lines) > 0 {
			parts = append(parts, "Content rules (must follow):\n"+strings.Join(lines, "\n"))
		}
	}

	// Imagery / photography style (text side; the tool side is handled separately).
	if img := ds.Imagery; img != nil {
		var lines []string
		if s := joinNonEmpty(img.MoodKeywords, ", "); s != "" {
			lines = append(lines, "Mood: "+s)
		}
		if s := strings.TrimSpace(img.StyleType); s != "" {
			lines = append(lines, "Style: "+s)
		}
		if s := strings.TrimSpace(img.ColorTreatment); s != "" {
			lines = append(lines, "Color treatment: "+s)
		}
		if s := strings.TrimSpace(img.Composition); s != "" {
			lines = append(lines, "Composition: "+s)
		}
		if s := strings.TrimSpace(img.SubjectMatter); s != "" {
			lines = append(lines, "Subjects: "+s)
		}
		if s := joinNonEmpty(img.Avoid, ", "); s != "" {
			lines = append(lines, "Avoid in visuals: "+s)
		}
		if len(lines) > 0 {
			parts = append(parts, "Imagery style:\n"+strings.Join(lines, "\n"))
		}
	}

	// Color & type identity — a one-line summary so the AI can name them even in
	// non-design modules (the full tokens ride on brandKitCSS in the content module).
	if s := paletteSummary(ds.Palette); s != "" {
		parts = append(parts, "Brand colors: "+s)
	}
	if s := fontSummary(ds.Fonts); s != "" {
		parts = append(parts, "Brand fonts: "+s)
	}

	// Per-platform overrides.
	if len(ds.PlatformOverrides) > 0 {
		var lines []string
		for platform, ov := range ds.PlatformOverrides {
			seg := platformOverrideLine(ov)
			if seg == "" {
				continue
			}
			lines = append(lines, "- "+platform+": "+seg)
		}
		if len(lines) > 0 {
			parts = append(parts, "Per-platform overrides (apply when producing for that platform):\n"+strings.Join(lines, "\n"))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	out := "Brand Design System (the brand's declared standard — every piece you create must follow it):\n" +
		strings.Join(parts, "\n\n")
	if len(out) > maxDesignSystemChars {
		out = out[:maxDesignSystemChars] + "\n…(truncated)"
	}
	return out
}

// designSystemVoiceText returns just the voice guidance/adjectives as a single
// line, used to decide whether the design system supersedes the legacy AIVoice.
func designSystemHasVoice(ds *trendlymodels.BrandDesignSystem) bool {
	if ds == nil || ds.Voice == nil {
		return false
	}
	v := ds.Voice
	return len(v.Adjectives) > 0 || strings.TrimSpace(v.Guidelines) != "" ||
		strings.TrimSpace(v.POV) != "" || strings.TrimSpace(v.EmojiPolicy) != "" ||
		len(v.SamplePhrases) > 0 || v.Tone != nil
}

// brandKitCSS builds a :root CSS variable block from the brand's Design System
// (colors + fonts + logo), for the design tools to reference. Falls back to the
// brand's avatar image for the logo when the Design System has no logo yet.
// Returns "" only when there is nothing at all to expose.
func brandKitCSS(ds *trendlymodels.BrandDesignSystem, fallbackLogo string) string {
	var vars []string

	logo := fallbackLogo
	if ds != nil {
		for _, l := range ds.Logos {
			if strings.TrimSpace(l.URL) != "" {
				// Prefer the primary variant; otherwise take the first with a URL.
				logo = l.URL
				if l.Variant == "primary" {
					break
				}
			}
		}
	}
	if strings.TrimSpace(logo) != "" {
		vars = append(vars, fmt.Sprintf("--brand-logo:url(%q)", logo))
	}

	if ds != nil {
		// Colors by role, plus a numbered token for every color so nothing is lost.
		roleSeen := map[string]bool{}
		for i, c := range ds.Palette {
			hex := strings.TrimSpace(c.Hex)
			if hex == "" {
				continue
			}
			if role := strings.TrimSpace(c.Role); role != "" && !roleSeen[role] {
				vars = append(vars, fmt.Sprintf("--brand-%s:%s", role, hex))
				roleSeen[role] = true
			}
			vars = append(vars, fmt.Sprintf("--brand-color-%d:%s", i+1, hex))
		}
		// Fonts by role.
		for _, f := range ds.Fonts {
			role := strings.TrimSpace(f.Role)
			family := strings.TrimSpace(f.Family)
			if role == "" || family == "" {
				continue
			}
			stack := family
			if fb := strings.TrimSpace(f.Fallback); fb != "" {
				stack = fmt.Sprintf("'%s',%s", family, fb)
			} else {
				stack = fmt.Sprintf("'%s',sans-serif", family)
			}
			vars = append(vars, fmt.Sprintf("--brand-font-%s:%s", role, stack))
		}
	}

	if len(vars) == 0 {
		return ""
	}
	return ":root{" + strings.Join(vars, ";") + "}"
}

// designSystemImageryGuidance folds the brand's imagery style into an image
// prompt: a positive prefix (mood/style/treatment) and a negative-prompt suffix
// (avoid list). Returns the prompt unchanged when no imagery style is defined.
func designSystemImageryGuidance(ds *trendlymodels.BrandDesignSystem, prompt string) string {
	if ds == nil || ds.Imagery == nil {
		return prompt
	}
	img := ds.Imagery
	var pos []string
	if s := joinNonEmpty(img.MoodKeywords, ", "); s != "" {
		pos = append(pos, s)
	}
	if s := strings.TrimSpace(img.StyleType); s != "" {
		pos = append(pos, s+" style")
	}
	if s := strings.TrimSpace(img.ColorTreatment); s != "" {
		pos = append(pos, s)
	}
	if s := strings.TrimSpace(img.Composition); s != "" {
		pos = append(pos, s)
	}
	out := prompt
	if len(pos) > 0 {
		out = strings.TrimSpace(out) + ". Brand visual style: " + strings.Join(pos, ", ") + "."
	}
	if s := joinNonEmpty(img.Avoid, ", "); s != "" {
		out += " Avoid: " + s + "."
	}
	// Fold brand colors in so generated imagery stays on-palette.
	if s := paletteSummary(ds.Palette); s != "" {
		out += " Use the brand palette (" + s + ") where appropriate."
	}
	return out
}

// ── small rendering helpers ──

func joinNonEmpty(vals []string, sep string) string {
	var out []string
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, sep)
}

func paletteSummary(palette []trendlymodels.DSColor) string {
	var out []string
	for _, c := range palette {
		hex := strings.TrimSpace(c.Hex)
		if hex == "" {
			continue
		}
		label := hex
		if name := strings.TrimSpace(c.Name); name != "" {
			label = name + " " + hex
		} else if role := strings.TrimSpace(c.Role); role != "" {
			label = role + " " + hex
		}
		out = append(out, label)
	}
	return strings.Join(out, ", ")
}

func fontSummary(fonts []trendlymodels.DSFont) string {
	var out []string
	for _, f := range fonts {
		family := strings.TrimSpace(f.Family)
		if family == "" {
			continue
		}
		if role := strings.TrimSpace(f.Role); role != "" {
			out = append(out, role+" "+family)
		} else {
			out = append(out, family)
		}
	}
	return strings.Join(out, ", ")
}

func platformOverrideLine(ov trendlymodels.DSPlatformOverride) string {
	var seg []string
	if s := strings.TrimSpace(ov.ToneOverride); s != "" {
		seg = append(seg, "tone: "+s)
	}
	if s := strings.TrimSpace(ov.CaptionLength); s != "" {
		seg = append(seg, "caption length: "+s)
	}
	if s := strings.TrimSpace(ov.HashtagStrategy); s != "" {
		seg = append(seg, "hashtags: "+s)
	}
	if s := joinNonEmpty(ov.ContentPillars, ", "); s != "" {
		seg = append(seg, "pillars: "+s)
	}
	if s := strings.TrimSpace(ov.Notes); s != "" {
		seg = append(seg, s)
	}
	return strings.Join(seg, "; ")
}

// toneWords renders the tone sliders into plain words, skipping any dial left
// near the neutral center so the prompt only states meaningful preferences.
func toneWords(t *trendlymodels.DSToneSliders) string {
	if t == nil {
		return ""
	}
	var out []string
	add := func(v *int, low, high string) {
		if v == nil {
			return
		}
		if w := poleWord(*v, low, high); w != "" {
			out = append(out, w)
		}
	}
	add(t.Formality, "casual", "formal")
	add(t.Playfulness, "serious", "playful")
	add(t.Warmth, "neutral", "warm")
	add(t.Boldness, "understated", "bold")
	add(t.Luxury, "accessible", "premium")
	return strings.Join(out, ", ")
}

// poleWord maps a 0..100 slider to "very/quite <pole>" or "" near the center.
func poleWord(v int, low, high string) string {
	switch {
	case v <= 20:
		return "very " + low
	case v <= 40:
		return "fairly " + low
	case v >= 80:
		return "very " + high
	case v >= 60:
		return "fairly " + high
	default:
		return ""
	}
}

func povWords(pov string) string {
	switch strings.ToLower(strings.TrimSpace(pov)) {
	case "we":
		return "first person (\"we\")"
	case "you":
		return "second person (address the reader as \"you\")"
	case "brand":
		return "third person (the brand name)"
	default:
		return pov
	}
}

func emojiWords(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "none":
		return "never use emoji"
	case "minimal":
		return "use emoji sparingly"
	case "liberal":
		return "emoji are welcome"
	default:
		return policy
	}
}
