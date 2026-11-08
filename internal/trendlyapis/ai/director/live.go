package director

// Building the manifest from real data.
//
// ── Why this file is separate from fixture.go ──────────────────────────────
// The fixture is for the lab: a hand-written brand so a plan can be read and
// argued about without touching a database. This is the same shape assembled
// from what a brand actually has. Keeping them apart means the lab cannot
// silently drift from production — they meet at `Manifest`, and anything the
// planner relies on has to exist in both.
//
// ⚠️ ONLY ENABLED MODELS GO IN. `enabled` is a REVIEW GATE, not a feature flag:
// a model nobody has looked at must not be able to start spending money. So an
// empty model list is the NORMAL state of a workspace where nothing has been
// turned on, and the planner is told to read it as "not available to you"
// rather than as a fault. A planner that invented a model id would produce a
// plan that cannot run.
//
// ⚠️ THE LIBRARY IS TRUNCATED, deliberately. A brand with two thousand assets
// cannot have all of them in a prompt, and the planner's job with the library
// is to notice "we already have a shot of the product on a desk" — which the
// newest few dozen rows answer about as well as all of them, and the
// `search_media` tool answers better.

import (
	"context"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// How many library rows the planner sees.
//
// Forty. Enough that "we already have something like this" is answerable, few
// enough that the list does not crowd out the brief. Anything beyond it is
// reachable through the search tool, which is the better instrument anyway.
const libraryWindow = 40

// RunCapFraction is how much of a wallet one run may spend by default.
//
// A fifth. Nobody wants one reel to cost a month's allowance, and a planner
// given the whole wallet as its ceiling will spend a surprising amount of it on
// a first attempt nobody has approved yet.
const RunCapFraction = 0.2

// BuildManifest assembles everything the planner sees, from live data.
func BuildManifest(
	ctx context.Context,
	brandID string,
	content *trendlymodels.Content,
	studio *trendlymodels.Studio,
	mode string,
	targetSeconds int,
) (Manifest, error) {
	if content == nil || studio == nil {
		return Manifest{}, fmt.Errorf("director: a manifest needs a content and a studio")
	}

	m := Manifest{
		Capabilities: DefaultCapabilities(),
		Tools:        ToolInventory(),
	}

	// ── The post ──
	aspect := "9:16"
	if studio.Width > studio.Height {
		aspect = "16:9"
	}
	m.Content = ContentFacts{
		ContentID:     content.ID,
		Title:         content.Title,
		Format:        string(content.ContentFormat),
		Aspect:        aspect,
		TargetSeconds: targetSeconds,
		Script:        content.Script,
		Caption:       content.Caption,
	}
	for _, p := range content.Platforms {
		m.Content.Platforms = append(m.Content.Platforms, string(p))
	}
	if len(content.ContentPillars) > 0 {
		m.Content.Pillar = content.ContentPillars[0]
	}

	// ── The brand ──
	m.Brand = BrandFacts{BrandID: brandID}
	var brand trendlymodels.Brand
	if err := brand.Get(brandID); err == nil {
		m.Brand.Name = brand.Name
	}
	if ds, err := trendlymodels.GetDesignSystem(ctx, brandID); err == nil && ds != nil {
		m.Brand.Voice = designSystemVoice(ds)
		for _, c := range ds.Palette {
			if c.Hex != "" {
				m.Brand.Palette = append(m.Brand.Palette, c.Hex)
			}
		}
		for _, f := range ds.Fonts {
			if f.Family != "" {
				m.Brand.Fonts = append(m.Brand.Fonts, f.Family)
			}
		}
		m.Brand.Rules = designSystemRules(ds)
	}

	// ── The models ──
	//
	// Every kind the planner can ask for, in one list. Video, image, music and
	// voice are all plannable, and a planner that could see only video would
	// quietly decide every scene has to be filmed.
	for _, kind := range []string{"video", "image", "music", "voice"} {
		rows, err := trendlymodels.ListEnabledMediaModels(ctx, kind)
		if err != nil {
			continue
		}
		for _, r := range rows {
			m.Models = append(m.Models, MediaModel{
				ID:              r.ID,
				DisplayName:     r.DisplayName,
				Kind:            r.Kind,
				StartFrame:      r.StartFrame,
				EndFrame:        r.EndFrame,
				ImageRefs:       r.ImageRefs,
				NativeAudio:     r.NativeAudio,
				MinDurationS:    r.MinDurationS,
				MaxDurationS:    r.MaxDurationS,
				DurationsS:      r.DurationsS,
				Resolutions:     r.Resolutions,
				AspectRatios:    r.AspectRatios,
				EstUSDPerSecond: r.EstUSDPerSecond,
				EstUSDPerCall:   r.EstUSDPerCall,
			})
		}
	}

	// ── The library ──
	if rows, err := trendlymodels.ListLibraryAssets(ctx, brandID, "", "", libraryWindow); err == nil {
		for _, a := range rows {
			m.Library = append(m.Library, MediaItem{
				ID:         a.ID,
				Kind:       a.Kind,
				Describes:  describeAsset(a),
				DurationMs: a.DurationMs,
				Aspect:     aspectOf(a.Width, a.Height),
				Origin:     originOf(a),
				NoAIInput:  a.NoAIInput,
			})
		}
	}

	// ── The budget ──
	//
	// In DOLLARS, because that is what the registry prices things in and what
	// the planner reasons about. The wallet is in tokens, so it is converted
	// here — once, in the one place that knows both units.
	m.Budget = Budget{Mode: normaliseMode(mode)}
	if orgID, ok := trendlymodels.OrgIDForBrand(brandID); ok && orgID != "" {
		if w, err := trendlymodels.GetTokenWallet(orgID); err == nil && w != nil {
			// The monthly allotment PLUS top-ups, because both are spendable —
			// top-ups are spent after the allotment but they are still money
			// the brand has. Counting only the allotment would make a
			// topped-up wallet plan as if it were empty.
			remaining := w.Balance + w.TopupBalance
			if remaining < 0 {
				remaining = 0
			}
			m.Budget.WalletUSD = float64(remaining) * trendlymodels.WalletTokenValueUSD
			m.Budget.RunCapUSD = m.Budget.WalletUSD * RunCapFraction
		}
		var org trendlymodels.Organization
		if err := org.Get(orgID); err == nil && org.PlanKey != nil {
			m.Budget.Plan = *org.PlanKey
		}
	}

	return m, nil
}

// normaliseMode keeps the planner's three intents and nothing else.
//
// The user picks an intent, never a model. "Balanced" is the default because
// the two extremes are both surprising: economy refuses generated video
// outright, and cinematic will spend the cap.
func normaliseMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "economy":
		return "economy"
	case "cinematic":
		return "cinematic"
	default:
		return "balanced"
	}
}

// designSystemVoice flattens the brand's voice into the sentence a planner can
// act on. The structured sliders are for the writing surfaces; a director needs
// "how should this sound" in words.
func designSystemVoice(ds *trendlymodels.BrandDesignSystem) string {
	if ds.Voice == nil {
		return ""
	}
	parts := []string{}
	if ds.Voice.Guidelines != "" {
		parts = append(parts, ds.Voice.Guidelines)
	}
	if len(ds.Voice.Adjectives) > 0 {
		parts = append(parts, "Sounds: "+strings.Join(ds.Voice.Adjectives, ", "))
	}
	if ds.Voice.POV != "" {
		parts = append(parts, "Written as: "+ds.Voice.POV)
	}
	if len(ds.Voice.SamplePhrases) > 0 {
		// Sample phrases do more for a planner than any slider: they are the
		// brand in its own words, which is what a narrated line has to sound
		// like.
		parts = append(parts, "Phrases it uses: "+strings.Join(ds.Voice.SamplePhrases, "; "))
	}
	return strings.Join(parts, " ")
}

// designSystemRules is the brand's own rules, which OUTRANK the craft floor.
//
// Ordered with the hardest first where we can tell, because a planner reading a
// long list weights the top of it — and the rule that matters most is almost
// always the one about what may be claimed.
func designSystemRules(ds *trendlymodels.BrandDesignSystem) []string {
	if ds.Rules == nil {
		return nil
	}
	out := []string{}
	// Approved claims FIRST, because the rule that matters most is the one
	// about what may be said — and a planner reading a long list weights the
	// top of it. This is also what the script-provenance check is for: a line
	// claiming something not in here has nothing behind it.
	for _, r := range ds.Rules.ApprovedClaims {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, "May claim: "+t)
		}
	}
	for _, r := range ds.Rules.Dos {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, t)
		}
	}
	for _, r := range ds.Rules.Donts {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, "Never: "+t)
		}
	}
	for _, r := range ds.Rules.BannedWords {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, "Never use the word: "+t)
		}
	}
	return out
}

// describeAsset is what the planner reads instead of looking at the picture.
//
// The PROMPT first where there is one: a generated clip has no filename anybody
// chose, so the words it was asked to be are the only description it has.
func describeAsset(a trendlymodels.Asset) string {
	if a.Origin != nil && a.Origin.Prompt != "" {
		return a.Origin.Prompt
	}
	for _, s := range []string{a.Title, a.Alt} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	if a.Ingest != nil {
		if len(a.Ingest.Shots) > 0 {
			return a.Ingest.Shots[0].Description
		}
		if a.Ingest.Text != "" {
			return shorten(a.Ingest.Text, 120)
		}
	}
	// An honest blank beats a guess. A planner told "image" will not reuse it,
	// which is the right outcome for a row nobody described.
	return a.Kind
}

func originOf(a trendlymodels.Asset) string {
	if a.Origin == nil {
		return a.Source
	}
	switch a.Origin.Kind {
	case trendlymodels.AssetOriginFrame:
		return "a frame taken from another clip"
	case trendlymodels.AssetOriginGenerated:
		if a.Origin.Model != "" {
			return "generated by " + a.Origin.Model
		}
		return "generated"
	case trendlymodels.AssetOriginRender:
		return "a render of an earlier post"
	default:
		return a.Origin.Kind
	}
}

func aspectOf(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	if h > w {
		return "9:16"
	}
	if w > h {
		return "16:9"
	}
	return "1:1"
}
