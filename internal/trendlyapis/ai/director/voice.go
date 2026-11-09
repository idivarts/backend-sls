package director

// Choosing the voice the narration speaks in.
//
// ── Why this exists ─────────────────────────────────────────────────────────
// A plan that narrates needs one voice id for the whole document — identity is
// the easy half of voice consistency, and without a fixed id every line is
// generated in a different voice.
//
// The planner was told that and never shown the voices. So it left the field
// empty, and validation refused the plan as FATAL:
//
//	[FATAL] audio: no voiceId — narration generated without a fixed voice is
//	        a different voice per line
//
// That is a dead end rather than a problem: the ids live in a provider account,
// not anywhere the user can read, so "fix it and plan again" asks for something
// nobody holds. A plan must never be blocked on this. The catalogue goes into
// the manifest so the planner can choose the voice that fits, and this file is
// the guarantee that something sensible is chosen even when it does not.

import (
	"os"
	"sort"
	"strings"
)

// FallbackVoiceEnv names a voice to use when nothing better is known.
//
// An env var rather than a hardcoded id because stock voice ids belong to the
// provider and can be retired; a constant would turn a provider's spring
// cleaning into silent narration. Empty is fine — it just means the last
// resort is the first catalogue entry instead.
const FallbackVoiceEnv = "ELEVENLABS_FALLBACK_VOICE_ID"

// narrationHints are the labels that mark a voice as built for being listened
// to over a video, in rough order of how much we want them. Matched against
// the provider's own label text, lowercased.
var narrationHints = []string{"narration", "narrative", "conversational", "social media", "storytelling"}

// EnsureVoice fills in the document voice when the plan did not choose one,
// and reports what it did.
//
// Returns the voice now in force and whether this call supplied it. A plan
// that already named a voice is left exactly as it is — the planner saw the
// catalogue and the content, and is the better judge of fit than any rule here.
//
// Order, and the reasoning for it:
//
//  1. THE STUDIO'S EXISTING VOICE. A studio that has been narrated before
//     should go on sounding like itself. Changing voices between runs of the
//     same video is a worse outcome than any catalogue pick.
//  2. A CATALOGUE VOICE BUILT FOR NARRATION, by the provider's own labels.
//  3. ANY CATALOGUE VOICE, deterministically — sorted by name, so two plans
//     for the same brand do not drift apart for no reason.
//  4. THE CONFIGURED FALLBACK, if the catalogue could not be read at all.
//
// Only if every one of those is empty does it give up, and even then the
// caller does not refuse — see Validate, where this is a note and not a fatal.
func (p *Plan) EnsureVoice(m Manifest) (voiceID string, supplied bool) {
	if p == nil {
		return "", false
	}
	if p.Audio.VoiceID != "" {
		return p.Audio.VoiceID, false
	}
	// Nothing is narrated, so there is nothing to keep consistent.
	if p.Audio.VoiceModel == "" {
		return "", false
	}

	if chosen := pickVoice(m); chosen != "" {
		p.Audio.VoiceID = chosen
		return chosen, true
	}
	return "", false
}

// pickVoice is the ranking in EnsureVoice's doc comment, in order.
func pickVoice(m Manifest) string {
	if m.PreferredVoiceID != "" {
		return m.PreferredVoiceID
	}

	// Sorted first so every tier below is deterministic. Two plans for the
	// same brand picking different voices because a provider reordered its
	// listing is a drift nobody could explain.
	voices := append([]VoiceOption(nil), m.Voices...)
	sort.Slice(voices, func(i, j int) bool { return voices[i].Name < voices[j].Name })

	for _, hint := range narrationHints {
		for _, v := range voices {
			if v.ID != "" && strings.Contains(strings.ToLower(v.Labels), hint) {
				return v.ID
			}
		}
	}
	for _, v := range voices {
		if v.ID != "" {
			return v.ID
		}
	}
	return strings.TrimSpace(os.Getenv(FallbackVoiceEnv))
}

// FlattenVoiceLabels turns a provider's label map into the one line a planner
// can read — "american · calm · narration".
//
// Sorted by key so the same voice reads the same way every time: an unsorted
// map would reshuffle the manifest on every call, which makes two otherwise
// identical plans differ and makes a prompt impossible to cache.
func FlattenVoiceLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if v := strings.TrimSpace(labels[k]); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " · ")
}
