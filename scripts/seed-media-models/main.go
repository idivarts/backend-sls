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

func videoRow(id, name string, usdPerSecond float64, keyframes bool) trendlymodels.MediaModel {
	m := trendlymodels.MediaModel{
		ID:          id,
		Provider:    "elevenlabs",
		DisplayName: name,
		Kind:        "video",
		StartFrame:  keyframes,
		EndFrame:    keyframes,
		ImageRefs:   true,
		NativeAudio: true,
		// Veo refuses 5 seconds with a 422 — it takes a SET, which min/max
		// cannot express, so both are carried and the set wins.
		MinDurationS:    4,
		MaxDurationS:    8,
		DurationsS:      []int{4, 6, 8},
		Resolutions:     []string{"720p", "1080p"},
		AspectRatios:    []string{"16:9", "9:16"},
		EstUSDPerSecond: usdPerSecond,
		MinPlan:         "pro",
		Enabled:         false,
	}
	return m
}

func catalog() []trendlymodels.MediaModel {
	return []trendlymodels.MediaModel{
		videoRow("veo-3.1-fast-generate-001", "Veo 3.1 Fast", 0.15, true),
		videoRow("veo-3.1-generate-001", "Veo 3.1", 0.40, true),
		videoRow("bytedance-seedance-v2", "Seedance v2", 0.40, false),
		videoRow("bytedance-seedance-v2-fast", "Seedance v2 Fast", 0.15, false),
		videoRow("bytedance-seedance-v2-mini", "Seedance v2 Mini", 0.08, false),
		videoRow("bytedance-seedance-v2.5", "Seedance v2.5", 0.40, false),
		videoRow("creatify-aurora", "Creatify Aurora", 0.20, false),

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
