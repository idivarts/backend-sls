package director

// ── A manifest without the infrastructure ───────────────────────────────────
//
// The real manifest is assembled from Firestore: the content doc, the brand's
// design system, the ENABLED rows of mediaModels, a digest of the brand's
// media library, and the org's wallet. None of that is needed to judge whether
// the planner plans well, so this file stands in for all of it.
//
// The brand here is Trendly itself, which makes the lab useful twice: it tests
// the planner, and what it plans is a post we might actually want. The brand
// facts are not invented — the voice rules, the proof rule and the competitor
// list are the ones documented in trendly-website/CLAUDE.md, and the palette is
// the one in branding/. The MEDIA LIBRARY is the only made-up part: those rows
// stand for product captures we would really have, and their ids are not real.
//
// Every model below is copied from scripts/seed-media-models — the real
// catalogue, with the real capabilities that were read off the provider's 422s
// rather than guessed. The Seedance family is deliberately absent: ElevenLabs
// gates ByteDance behind per-workspace approval and refuses an otherwise valid
// request with 403 model_access_denied, so those rows exist in the registry
// but cannot be enabled, and a manifest that offered them would have the
// planner reaching for a 15- or 30-second clip it cannot have.

// FixtureManifest is a plausible brand, content and wallet, for the lab.
func FixtureManifest(format string, targetSeconds int, mode string, runCap float64) Manifest {
	return Manifest{
		Content: ContentFacts{
			ContentID:     "ct_fixture_01",
			Title:         "The Sunday night problem",
			Format:        format,
			Platforms:     []string{"instagram", "linkedin"},
			Aspect:        "9:16",
			TargetSeconds: targetSeconds,
			Hook:          "Sunday nights used to be the worst part of my week.",
			Script: `Sunday nights used to be the worst part of my week.
Me, an empty content calendar, and nine tabs of AI open.
Scheduling was never the hard part. Deciding what to post was.
Trendly does the deciding with you — it builds the strategy, fills the month, writes in your voice, and schedules it.
One workspace instead of nine tabs.
Free to start, no card.`,
			Caption: "Sunday night used to mean staring at an empty calendar.",
			Pillar:  "Education",
		},
		// Voice, proof rule and competitor handling are lifted from
		// trendly-website/CLAUDE.md, which is the documented source of truth
		// for tone; the palette is branding/'s.
		Brand: BrandFacts{
			BrandID: "br_fixture_01",
			Name:    "Trendly — AI Social Planner",
			Voice:   "Confident, punchy, no fluff. Short sentences. Name the pain, then the relief. Second person, founder-to-founder. Specific over generic. Never 'boost productivity'.",
			Palette: []string{"#054463", "#02537D", "#16242E", "#FFFFFF"},
			Fonts:   []string{"Quicksand", "Inter"},
			Rules: []string{
				"NO FABRICATED PROOF. Never invent a testimonial, a named customer, a company logo, a review badge, a user count or a growth number. US FTC fake-review rules carry per-violation penalties. Proof comes only from things we can stand behind.",
				"Never stock photography. Visuals are real product UI, product mocks, or nothing.",
				"A competitor may be named (Buffer, Later, SocialPilot, Hootsuite) and credited where it is genuinely strong, but never show its UI, logo or branding.",
				"USD only, never INR.",
				"Pair any call to action with the risk reversal: free forever, no credit card, cancel anytime.",
				"Treat product capabilities as claims — only say what is actually built.",
			},
			WhatWorked: []string{
				"Audience is solo founders and in-house teams of two or three, plus small agencies. Speak to one person doing the work, never to a department.",
				"The wedge against Buffer and Later is that they schedule what you have already decided; Trendly decides with you. Lead with the deciding, not the posting.",
				"Posts that show the actual product screen outperform posts that describe it.",
				"Naming the specific pain in the first line holds attention; a feature list in the first line loses it.",
			},
			LogoMediaID: "md_logo_mark",
		},
		// Only ENABLED rows ever reach a manifest. Two Veo video tiers, two
		// image models, a music model and a voice model.
		Models: []MediaModel{
			{
				ID: "veo-3.1-fast-generate-001", DisplayName: "Veo 3.1 Fast", Kind: "video",
				StartFrame: true, EndFrame: true, ImageRefs: true, NativeAudio: true,
				MinDurationS: 4, MaxDurationS: 8, DurationsS: []int{4, 6, 8},
				Resolutions: []string{"720p", "1080p"}, AspectRatios: []string{"16:9", "9:16"},
				EstUSDPerSecond: 0.15,
			},
			{
				ID: "veo-3.1-generate-001", DisplayName: "Veo 3.1", Kind: "video",
				StartFrame: true, EndFrame: true, ImageRefs: true, NativeAudio: true,
				MinDurationS: 4, MaxDurationS: 8, DurationsS: []int{4, 6, 8},
				Resolutions: []string{"720p", "1080p"}, AspectRatios: []string{"16:9", "9:16"},
				EstUSDPerSecond: 0.40,
			},
			{
				ID: "google/gemini-3.1-flash-image-preview", DisplayName: "Gemini 3.1 Flash Image",
				Kind: "image", AspectRatios: []string{"9:16", "1:1", "4:5", "16:9"},
				EstUSDPerCall: 0.04,
			},
			{
				ID: "google/gemini-3-pro-image-preview", DisplayName: "Gemini 3 Pro Image",
				Kind: "image", AspectRatios: []string{"9:16", "1:1", "4:5", "16:9"},
				EstUSDPerCall: 0.14,
			},
			{
				ID: "music_v2", DisplayName: "ElevenLabs Music", Kind: "music",
				MinDurationS: 3, MaxDurationS: 600, EstUSDPerSecond: 0.0033,
			},
			{
				ID: "eleven_multilingual_v2", DisplayName: "Multilingual v2", Kind: "voice",
				EstUSDPerCall: 0.00022,
			},
		},
		// Stand-ins for the product captures a real workspace would hold. The
		// last row keeps the licence trap: it may be shown but never fed to a
		// model, which is also what the brand's no-stock rule says about it.
		Library: []MediaItem{
			{ID: "md_logo_mark", Kind: "image", Describes: "Trendly app icon — navy rounded square with the paper plane and two dots, transparent", Aspect: "1:1", Origin: "branding/"},
			{ID: "md_wordmark_white", Kind: "image", Describes: "Trendly wordmark in white, transparent, for dark backgrounds", Aspect: "16:9", Origin: "branding/"},
			{ID: "md_calendar_fill", Kind: "video", Describes: "Screen capture: a month of the content calendar filling itself post by post, 7 seconds", DurationMs: 7000, Aspect: "9:16", Origin: "product capture"},
			{ID: "md_chat_models", Kind: "image", Describes: "Product still: the AI chat with the model switcher open, GPT, Claude and Gemini visible", Aspect: "9:16", Origin: "product capture"},
			{ID: "md_strategy_doc", Kind: "image", Describes: "Product still: a generated content strategy document with its pillars", Aspect: "9:16", Origin: "product capture"},
			{ID: "md_approval_bar", Kind: "image", Describes: "Product still: the approval bar — Approve, Request changes, AI edit", Aspect: "9:16", Origin: "product capture"},
			{ID: "md_inbox_unified", Kind: "image", Describes: "Product still: the unified inbox, comments and DMs from every platform in one list", Aspect: "9:16", Origin: "product capture"},
			{ID: "md_tabs_mess", Kind: "image", Describes: "Screen still: nine browser tabs of different AI tools open at once, deliberately ugly", Aspect: "9:16", Origin: "product capture"},
			{ID: "md_founder_desk", Kind: "video", Describes: "Over-the-shoulder of someone at a laptop late at night, face not visible, 5 seconds", DurationMs: 5200, Aspect: "9:16", Origin: "own shoot"},
			{ID: "md_stock_desk", Kind: "image", Describes: "Tidy desk with a laptop and a plant (stock, Unsplash)", Aspect: "16:9", Origin: "unsplash", NoAIInput: true},
		},
		Budget: Budget{
			WalletUSD: 42.00,
			RunCapUSD: runCap,
			Plan:      "pro",
			Mode:      mode,
		},
		Capabilities: DefaultCapabilities(),
		Tools:        ToolInventory(),
	}
}
