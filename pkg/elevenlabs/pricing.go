package elevenlabs

// ── What ElevenLabs actually charges, and what that costs us ────────────────
//
// ElevenLabs bills CREDITS, not dollars, and the credits per unit of work are
// nothing like each other: a character of speech costs half a credit, a second
// of music fifteen, and a second of generated video six hundred. Pricing any of
// it from a guessed dollar figure is how a product ends up selling video at a
// loss while over-charging for a voiceover.
//
// So the costs here are expressed in CREDITS, measured against the live API,
// and converted once. Every figure below was taken the same way, on
// 2026-10-30, and can be re-taken the same way in ten minutes:
//
//	GET /v1/user/subscription                  → character_count before/after
//	GET /v1/usage/character-stats?breakdown_type=product_type
//
// ⚠️ Each is ONE measurement, not a curve. They are rounded UP, and the
// conversion below uses the dearest plan rate rather than our own, because an
// over-charge is a product decision we can take back and an under-charge is
// revenue that never existed.
const (
	// USDPerCredit converts a credit to money.
	//
	// Deliberately the CREATOR plan's rate ($22 / 100,000), not the Pro rate we
	// are actually on ($99 / 500,000 = $0.000198). The plan can change under
	// us — downgrades included — and the cheaper rate would silently turn every
	// price here into an under-charge the day it did.
	USDPerCredit = 0.00022

	// MusicCreditsPerSecond — measured: 45 credits for a 3.0s clip (15.0/s),
	// consistent with 238 and 947 credits across the days either side.
	MusicCreditsPerSecond = 16.0

	// TTSCreditsPerChar — measured: 32 credits for 66 characters (0.485/char)
	// on eleven_multilingual_v2.
	//
	// Billed at the FULL rate rather than the measured half, because the half
	// is a property of that model: eleven_v3 and the older multilingual models
	// bill a credit per character, and the model is a per-request parameter
	// that nothing here controls. Measuring one model and charging as if every
	// model were that cheap is how a model switch becomes a margin cut.
	TTSCreditsPerChar = 1.0

	// STTCreditsPerSecond — inferred, not measured cleanly: a 2-credit charge
	// appeared against a 4-second clip (0.5/s). Rounded up to 1.0 and flagged:
	// this is the weakest number on the page, and the one to re-measure first
	// against a long file.
	STTCreditsPerSecond = 1.0

	// VideoCreditsPerSecond — measured: 2,424 credits for a 4.0s clip on
	// veo-3.1-fast-generate-001 (606/s), i.e. about $0.13 a second.
	//
	// Used ONLY as the floor for a registry row that carries no price of its
	// own. A real row's EstUSDPerSecond wins, because the per-second cost is a
	// property of the model and the catalog changes weekly.
	VideoCreditsPerSecond = 606.0
)

// CreditsUSD converts credits to dollars.
func CreditsUSD(credits float64) float64 {
	if credits <= 0 {
		return 0
	}
	return credits * USDPerCredit
}

// MusicCostUSD prices a generated music bed by its length.
func MusicCostUSD(lengthMs int) float64 {
	secs := float64(lengthMs) / 1000
	if secs <= 0 {
		secs = 30
	}
	return CreditsUSD(MusicCreditsPerSecond * secs)
}

// TTSCostUSD prices a voiceover by its text.
//
// Counted in RUNES, not bytes: ElevenLabs bills characters, and a Devanagari or
// emoji character is several bytes. Billing by byte would over-charge non-Latin
// scripts by a factor of three, which is a worse failure than under-charging.
func TTSCostUSD(text string) float64 {
	return CreditsUSD(TTSCreditsPerChar * float64(len([]rune(text))))
}

// STTCostUSD prices a transcription by the length of the audio.
func STTCostUSD(durationMs int) float64 {
	secs := float64(durationMs) / 1000
	if secs <= 0 {
		return 0
	}
	return CreditsUSD(STTCreditsPerSecond * secs)
}

// VideoFloorCostUSD prices a second of generated video when the registry row
// has no price of its own.
func VideoFloorCostUSD(seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return CreditsUSD(VideoCreditsPerSecond * seconds)
}
