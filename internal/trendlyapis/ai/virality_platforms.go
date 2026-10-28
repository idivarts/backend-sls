package ai

import (
	"sort"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// playbookPriority decides which playbooks survive the cap.
//
// Sorting the keys alphabetically would be deterministic, which is what the
// cap needs, and also wrong: facebook/instagram/linkedin/reddit would win on
// spelling and a piece targeting YouTube would get Reddit's rules instead of
// the ones about its own ranking gate. Ordering is therefore explicit, roughly
// by how much the platform's own documented behaviour differs from the generic
// craft contract — a surface whose rules are already covered by L0 loses least
// by being dropped.
var playbookPriority = map[trendlymodels.Platform]int{
	trendlymodels.PlatformInstagram: 0,
	trendlymodels.PlatformYouTube:   1,
	trendlymodels.PlatformLinkedIn:  2,
	trendlymodels.PlatformTwitter:   3,
	trendlymodels.PlatformReddit:    4,
	trendlymodels.PlatformFacebook:  5, // closest to Instagram's, so cheapest to lose
}

// Per-platform distribution playbooks — the L1 layer.
//
// The craft contract (virality_craft.go) is platform-agnostic on purpose. This
// file is where a surface's own rules live: what it ranks on, which ask maps to
// the signal it actually rewards, and the format facts that decide whether a
// piece is eligible for distribution at all.
//
// Injected ONLY for the platforms a piece actually targets. That is not a
// micro-optimisation: the whole set runs to well over a thousand tokens and it
// rides every message in the content/calendar/strategy modules, so sending
// Reddit's rules to someone writing a Reel costs real money for nothing.
//
// SOURCING, per the rule in virality_craft.go: a line that states what a
// platform ranks on is from that platform's own documentation unless it is
// marked "reported, unverified". Twitter and LinkedIn publish nothing
// comparable, so their blocks lean on format facts, which are checkable, and
// flag the rest. When refreshing this file, keep that distinction — it is the
// only thing stopping marketing-blog folklore from becoming a rule the model
// treats as fact. Last reviewed against the sources: 2026-10-29.
var platformPlaybooks = map[trendlymodels.Platform]string{
	trendlymodels.PlatformInstagram: "INSTAGRAM\n" +
		"- Reels are ranked on four predictions, in Instagram's own stated order: " +
		"reshare, watch-through, like, tap the audio. Write for the reshare.\n" +
		"- Explore ranks on likes, SAVES and shares, and raw popularity counts for much " +
		"more there than in Feed. Reference-style pieces earn Explore; jokes do not.\n" +
		"- Feed is a different surface again: it predicts a few seconds of dwell, then " +
		"comment, like, share, profile tap.\n" +
		"- Best ask: 'send this to someone who…'. Second best: 'save this for…'. Not " +
		"'follow for more'.\n" +
		"- Six things that make a Reel LESS visible: low resolution, a visible " +
		"watermark, no audio, borders or letterboxing, mostly text, or already posted. " +
		"A re-upload of something that already ran here is explicitly demoted.\n" +
		"- The caption's first line is the only part shown before 'more'. Extra hashtags " +
		"belong in the first comment, not the caption.\n",

	trendlymodels.PlatformFacebook: "FACEBOOK\n" +
		"- Reels behave as they do on Instagram — same company, same mechanics. Feed " +
		"predicts dwell, comment, like and share.\n" +
		"- Engagement bait ('like if', 'tag someone who', 'comment YES') has been " +
		"demoted since 2017, with harsher treatment for repeat offenders. Never use it.\n" +
		"- Best ask: a share, or a genuine question that a real person would answer " +
		"without being prompted to.\n" +
		"- Native video only. Link-outs in the post body suppress reach (reported, " +
		"unverified).\n",

	trendlymodels.PlatformYouTube: "YOUTUBE (Shorts and video)\n" +
		"- Studio's own Shorts metric is 'Viewed (vs. swiped away)' — the swipe decision " +
		"in the opening seconds IS the ranking gate, named by the platform.\n" +
		"- Since March 2025 a Shorts view counts on any start or replay, with no minimum " +
		"watch time. View count is therefore not a quality signal; retention is. Do not " +
		"optimise for views.\n" +
		"- The TITLE does real work here, unlike on Instagram: it is searchable and it " +
		"shows in the feed. Write it as a hook, never as a label. 'How we cut onboarding " +
		"to 4 minutes' beats 'Onboarding update'.\n" +
		"- Best ask: a specific next watch, or a comment that genuinely continues the " +
		"topic. Subscribe prompts mid-video cost retention.\n" +
		"- Shorts are vertical and under 3 minutes. The description and tags aid " +
		"retrieval, not ranking.\n",

	trendlymodels.PlatformLinkedIn: "LINKEDIN\n" +
		"- Dwell time is the signal most consistently reported for video, and early " +
		"engagement in the first hour gates wider distribution (reported, unverified — " +
		"LinkedIn publishes no ranking documentation).\n" +
		"- The first line before 'see more' is the whole hook. Nothing else is visible " +
		"until someone chooses to expand.\n" +
		"- A specific outcome, a real number, or a genuinely contrarian claim " +
		"outperforms general professional advice, which the feed is saturated with.\n" +
		"- Engagement-pod prompts and 'comment YES if you agree' are actively suppressed " +
		"as of 2026 (reported). Ask for a view or an opinion, not a reaction.\n" +
		"- Native upload. External links in the post body reduce reach (reported, " +
		"unverified) — put the link in the first comment.\n" +
		"- 30-90 seconds for reaching beyond followers; longer only for an audience that " +
		"already knows the brand.\n",

	trendlymodels.PlatformTwitter: "TWITTER / X\n" +
		"- The first post carries everything: no setup, no 'a thread 🧵' preamble before " +
		"the point. Front-load.\n" +
		"- In a thread every entry has to stand alone, because entries are seen out of " +
		"order and quoted individually.\n" +
		"- Native video autoplays MUTED and most viewers never unmute. Burned-in " +
		"captions are not optional here.\n" +
		"- The share mechanic is the quote-post: write a line someone can add their own " +
		"take to. That is a different target from a line people merely agree with.\n" +
		"- Short. A post that needs 'see more' has already lost the scroll.\n",

	trendlymodels.PlatformReddit: "REDDIT\n" +
		"- The TITLE is the entire post for ranking purposes, and it cannot be edited " +
		"after submission. It carries the whole hook.\n" +
		"- Subreddit norms outrank every general rule here. Anything that reads as " +
		"promotional is removed by moderators regardless of quality.\n" +
		"- Value first, brand second or not at all. A brand voice that works on " +
		"Instagram is actively penalised on Reddit.\n" +
		"- No engagement bait of any kind, and no asking for upvotes — both are against " +
		"sitewide rules, not merely ineffective.\n",
}

// platformPlaybookBlock returns the L1 block for the platforms a piece targets,
// or "" when none of them is a platform we have a playbook for.
//
// Unknown values are skipped rather than guessed at: a playbook invented for a
// surface nobody verified is worse than no playbook, because the model cannot
// tell the difference. linkedin_page shares LinkedIn's rules, so it folds into
// the same entry instead of duplicating it.
//
// The cap exists because a piece targeting every platform at once would
// otherwise push ~1,500 tokens of playbook into every message of the
// conversation. Beyond four surfaces the per-platform advice is mostly
// redundant with the craft contract anyway.
const maxPlatformPlaybooks = 4

func platformPlaybookBlock(platforms []trendlymodels.Platform) string {
	if len(platforms) == 0 {
		return ""
	}

	seen := make(map[trendlymodels.Platform]bool, len(platforms))
	var keys []trendlymodels.Platform
	for _, p := range platforms {
		key := trendlymodels.Platform(strings.ToLower(strings.TrimSpace(string(p))))
		if key == trendlymodels.PlatformLinkedInPage {
			key = trendlymodels.PlatformLinkedIn
		}
		if _, ok := platformPlaybooks[key]; !ok || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ""
	}
	// Stable order so the same content produces the same prompt every time —
	// an unstable system prompt would make the Phase 2 scorer's run-to-run
	// variance impossible to attribute — and priority order so the cap below
	// drops the playbook that costs least, not the one latest in the alphabet.
	sort.Slice(keys, func(i, j int) bool {
		return playbookPriority[keys[i]] < playbookPriority[keys[j]]
	})
	if len(keys) > maxPlatformPlaybooks {
		keys = keys[:maxPlatformPlaybooks]
	}

	var sb strings.Builder
	sb.WriteString("\n--- THIS PIECE'S PLATFORMS ---\n")
	sb.WriteString("Where a platform below contradicts the general rules above, the " +
		"platform wins for that platform's version of the piece.\n\n")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(platformPlaybooks[k])
	}
	return sb.String()
}
