// Command seed-media-models writes the generative-media catalog into
// `mediaModels`, which is the collection the composer reads to decide what it
// may offer.
//
// It exists because that catalog is DATA, and data does not arrive with a
// deploy. Dev was seeded by hand while the feature was built; production has
// nothing, and without rows the AI-video tab reads "not switched on yet" and no
// clip can be generated.
//
// ⚠️ Every row is written `enabled: false`, and an EXISTING row's `enabled` is
// never touched. That field is a review gate, not a feature flag: a model
// nobody has looked at must not be able to start spending money, and a model
// somebody has deliberately turned on must not be turned off by a re-run.
// Enabling is a separate, deliberate act — a field edit in the console.
//
// Prices are USD per unit, converted from credits measured against the live
// ElevenLabs API on 2026-10-30 at the CREATOR plan rate ($0.00022/credit) —
// the dearest we might be on, so a downgrade cannot turn them into an
// under-charge:
//
//	music   15 credits/s   → $0.0033/s
//	speech  1 credit/char  → $0.00022/char   (measured 0.485 on multilingual_v2;
//	                                          billed full, other models charge it)
//	stt     1 credit/s     → $0.00022/s      (inferred from 2 credits on a 4s clip)
//	video   606 credits/s  → $0.133/s        (measured on veo-3.1-fast)
//
// The per-model video prices below are the PROVIDER's published per-second
// rates, which is what EstUSDPerSecond is for; the measured figure above is the
// floor the code falls back to when a row carries no price at all.
//
// ⚠️ A ROW'S DOCUMENT ID *IS* THE PROVIDER'S model_id. It is what gets sent on
// the wire, so a tidier-looking id — "eleven-music-v1" for what ElevenLabs
// calls "music_v2" — is not a naming choice, it is a 422 on every generation
// that model serves. The display name is the place for anything readable.
//
// Model ids come from the API itself: POST an unknown model_id and the 422
// enumerates every valid tag. That costs nothing, where guessing at a real one
// can cost a generation.
//
// Idempotent. Dry-run by default; set APPLY=1 to write.
//
//	go run ./scripts/seed-media-models           # dry-run
//	APPLY=1 go run ./scripts/seed-media-models   # apply
//
// The database comes from FIRESTORE_DATABASE_ID, as everywhere else — so
// seeding production is that variable, not a flag here.
package main

import (
	"context"
	"log"
	"os"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	_ "github.com/idivarts/backend-sls/pkg/firebase"
)

// videoRow builds a video model row. Everything that differs between models
// is a field rather than a default, because the differences are what the row
// exists to record — and every value below was read off the API, not guessed.
//
// How to read them off it: POST the model_id with deliberately invalid values
// and the 422 enumerates what it will take.
//
//	duration_secs: 0      -> "Input should be greater than or equal to 4"
//	duration_secs: 999    -> "Input should be less than or equal to 15"
//	resolution: "BAD"     -> "Input should be '480p' or '720p'"
//	aspect_ratio: "BAD"   -> the full list
//	generate_audio: "x"   -> "bool_parsing" if it takes it, "extra_forbidden" if not
//	end_frame: "BAD"      -> "should be a valid dictionary" if it takes one
//
// The request is rejected at validation, so a full capability sweep costs
// nothing — where one wrong field costs a generation to discover.
type videoSpec struct {
	id, name               string
	usdPerSecond           float64
	keyframes, nativeAudio bool
	minS, maxS             int
	// durations is the EXACT set, for a model that takes a set rather than a
	// range. Veo refuses 5s with a 422; Seedance takes anything from 4 to its
	// cap, so it leaves this empty and the bounds are the truth.
	durations            []int
	resolutions, aspects []string
}

func videoRow(v videoSpec) trendlymodels.MediaModel {
	return trendlymodels.MediaModel{
		ID:              v.id,
		Provider:        "elevenlabs",
		DisplayName:     v.name,
		Kind:            "video",
		StartFrame:      v.keyframes,
		EndFrame:        v.keyframes,
		ImageRefs:       true,
		NativeAudio:     v.nativeAudio,
		MinDurationS:    v.minS,
		MaxDurationS:    v.maxS,
		DurationsS:      v.durations,
		Resolutions:     v.resolutions,
		AspectRatios:    v.aspects,
		EstUSDPerSecond: v.usdPerSecond,
		MinPlan:         "pro",
		Enabled:         false,
	}
}

// What every Seedance variant takes, which is a wider set than we send.
var seedanceAspects = []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}

func catalog() []trendlymodels.MediaModel {
	return []trendlymodels.MediaModel{
		// Veo takes a SET of lengths — 5s is a 422 — and tops out at 1080p.
		videoRow(videoSpec{
			id: "veo-3.1-fast-generate-001", name: "Veo 3.1 Fast", usdPerSecond: 0.15,
			keyframes: true, nativeAudio: true,
			minS: 4, maxS: 8, durations: []int{4, 6, 8},
			resolutions: []string{"720p", "1080p"}, aspects: []string{"16:9", "9:16"},
		}),
		videoRow(videoSpec{
			id: "veo-3.1-generate-001", name: "Veo 3.1", usdPerSecond: 0.40,
			keyframes: true, nativeAudio: true,
			minS: 4, maxS: 8, durations: []int{4, 6, 8},
			resolutions: []string{"720p", "1080p"}, aspects: []string{"16:9", "9:16"},
		}),

		// Seedance takes a RANGE, goes further than Veo, and does take start
		// and end frames — all three of which the first pass of this file got
		// wrong, by copying Veo's shape instead of asking.
		videoRow(videoSpec{
			id: "bytedance-seedance-v2", name: "Seedance v2", usdPerSecond: 0.40,
			keyframes: true, nativeAudio: true, minS: 4, maxS: 15,
			resolutions: []string{"480p", "720p", "1080p", "4k"}, aspects: seedanceAspects,
		}),
		videoRow(videoSpec{
			id: "bytedance-seedance-v2-fast", name: "Seedance v2 Fast", usdPerSecond: 0.15,
			keyframes: true, nativeAudio: true, minS: 4, maxS: 15,
			// 1080p is a 422 on the fast and mini variants. The row said
			// 720p/1080p, so every 1080p request would have been refused.
			resolutions: []string{"480p", "720p"}, aspects: seedanceAspects,
		}),
		videoRow(videoSpec{
			id: "bytedance-seedance-v2-mini", name: "Seedance v2 Mini", usdPerSecond: 0.08,
			keyframes: true, nativeAudio: true, minS: 4, maxS: 15,
			resolutions: []string{"480p", "720p"}, aspects: seedanceAspects,
		}),
		videoRow(videoSpec{
			id: "bytedance-seedance-v2.5", name: "Seedance v2.5", usdPerSecond: 0.40,
			keyframes: true, nativeAudio: true, minS: 4, maxS: 30,
			resolutions: []string{"480p", "720p", "1080p"}, aspects: seedanceAspects,
		}),

		// ⛔ creatify-aurora is NOT here, and should not be added back.
		//
		// It is not a text-to-video model at all. Its API requires `image` and
		// `audio` and FORBIDS `prompt`, `duration_secs` and `aspect_ratio` —
		// it animates a still from a soundtrack, which is a different product
		// wearing the same endpoint. Nothing in mediagen.Spec can drive it, so
		// a row for it is an invitation to enable a model that 422s on every
		// request. Supporting it means a driver, not a row.

		{
			ID: "music_v2", Provider: "elevenlabs",
			DisplayName: "ElevenLabs Music", Kind: "music",
			MinDurationS: 3, MaxDurationS: 600,
			EstUSDPerSecond: 0.0033, Enabled: false,
		},
		{
			ID: "eleven_multilingual_v2", Provider: "elevenlabs",
			DisplayName: "Multilingual v2", Kind: "voice",
			EstUSDPerCall: 0.00022, Enabled: false,
		},
		{
			ID: "scribe_v2", Provider: "elevenlabs",
			DisplayName: "Scribe v2", Kind: "stt",
			EstUSDPerSecond: 0.00022, Enabled: false,
		},
	}
}

func main() {
	ctx := context.Background()
	apply := os.Getenv("APPLY") == "1"
	if !apply {
		log.Println("DRY RUN — set APPLY=1 to write")
	}

	for _, m := range catalog() {
		model := m
		existing, err := trendlymodels.GetMediaModel(ctx, model.ID)
		if err != nil {
			log.Printf("  ! %s: could not read: %v", model.ID, err)
			continue
		}
		if existing != nil {
			// Never clobber a review decision, in either direction.
			model.Enabled = existing.Enabled
		}
		state := "disabled"
		if model.Enabled {
			state = "ENABLED (kept)"
		}
		verb := "create"
		if existing != nil {
			verb = "update"
		}
		log.Printf("  %s %-30s %-8s %s", verb, model.ID, model.Kind, state)
		if !apply {
			continue
		}
		if err := trendlymodels.UpsertMediaModel(ctx, &model); err != nil {
			log.Printf("  ! %s: %v", model.ID, err)
		}
	}

	if apply {
		trendlymodels.InvalidateMediaModels()
		log.Println("done — enable rows deliberately, one at a time")
	}
}
