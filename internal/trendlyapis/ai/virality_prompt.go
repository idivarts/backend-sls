package ai

import (
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// Assembly for the virality knowledge layers.
//
// Three blocks go into the system prompt, in this order and for a reason:
// L0 (the craft contract) states the standard, L2 (the pattern library) gives
// the model something to choose from, and L1 (the platform playbooks) override
// both for the specific surfaces this piece targets. Last wins where they
// disagree, which is why the platform block is appended last and says so.

// moduleHasVirality reports whether a module's output is content that has to
// survive a feed, and so is held to the virality craft contract.
//
// Onboarding is deliberately excluded: it is a setup conversation, and loading
// ~2k tokens of distribution theory into it would crowd out the thing it is
// actually doing. The design module inherits this through moduleContent.
func moduleHasVirality(module string) bool {
	return module == moduleContent || module == moduleCalendar || module == moduleStrategy
}

// viralityBlocks assembles the three layers for a prompt, or returns "" for a
// module that does not author content.
//
// `platforms` is what the caller knows. Script generation knows, because it
// works from the live editor and may be looking at unsaved edits; ordinary
// chat does not, so we fall back to the saved content doc.
func viralityBlocks(module, brandID, contextID string, platforms []trendlymodels.Platform) string {
	if !moduleHasVirality(module) {
		return ""
	}
	if len(platforms) == 0 {
		platforms = platformsForContext(module, brandID, contextID)
	}
	return viralityCraftInstructions + viralityPatternsBlock + platformPlaybookBlock(platforms)
}

// platformList normalises what the live editor sends — a single `platform`, a
// `platforms` list, or neither — into the typed form the playbooks are keyed
// by. The list wins when both are present, matching contentBriefText.
func platformList(single string, many []string) []trendlymodels.Platform {
	vals := many
	if len(vals) == 0 && strings.TrimSpace(single) != "" {
		vals = []string{single}
	}
	out := make([]trendlymodels.Platform, 0, len(vals))
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, trendlymodels.Platform(v))
		}
	}
	return out
}

// targetPlatformList picks the platforms a generation request is actually FOR.
//
// A variation request carries two different platforms: `platform` is the
// piece's generic surface and `targetPlatform` is the variation being written
// right now. enrichLiveBrief already tells the model "you are tailoring this
// for the linkedin variation", so the playbook has to agree with it — loading
// the generic platform's rules there would hand the model Instagram's ranking
// signals while instructing it to write for LinkedIn, which is worse than
// giving it no platform rules at all.
func targetPlatformList(targetPlatform, platform string) []trendlymodels.Platform {
	if strings.TrimSpace(targetPlatform) != "" {
		return platformList(targetPlatform, nil)
	}
	return platformList(platform, nil)
}

// platformsForContext recovers the target platforms from the saved content doc.
//
// This is a second read of a document buildSystemPrompt has usually already
// loaded through loadModuleContext. That is deliberate rather than overlooked:
// threading the doc through would change four signatures to save one small
// Firestore read on a path that then spends seconds inside an LLM call. If a
// request-scoped cache ever lands for these, this is a caller to move onto it.
//
// Returns nil for anything that is not a saved content doc, which makes the
// platform block simply absent — the craft contract still applies.
func platformsForContext(module, brandID, contextID string) []trendlymodels.Platform {
	if module != moduleContent || brandID == "" || contextID == "" {
		return nil
	}
	ct, err := trendlymodels.GetContent(brandID, contextID)
	if err != nil || ct == nil {
		return nil
	}
	if len(ct.Platforms) > 0 {
		return ct.Platforms
	}
	if ct.Platform != "" {
		return []trendlymodels.Platform{trendlymodels.Platform(ct.Platform)}
	}
	return nil
}
