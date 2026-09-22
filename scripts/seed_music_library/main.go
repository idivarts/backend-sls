// seed_music_library generates a curated, commercially-licensed music catalog
// via the ElevenLabs Music API and writes it to the shared `musicLibrary`
// Firestore collection for the Studio soundtrack browser. Run ONCE (re-runnable
// to add more). Requires ELEVENLABS_API_KEY + AWS creds + service-account.json.
//
//	cd backend-sls
//	ELEVENLABS_API_KEY=... ATTACHMENT_S3_BUCKET_NAME=... ATTACHMENT_CF_DISTRIBUTION_URL=... \
//	  go run ./scripts/seed_music_library
package main

import (
	"fmt"
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/media"
	"github.com/idivarts/backend-sls/pkg/elevenlabs"

	_ "github.com/idivarts/backend-sls/pkg/firebase"
)

type moodSpec struct {
	Mood   string
	Prompt string
	Titles []string
}

// Expand these lists to grow the catalog. Each title generates one ~30s track.
var catalog = []moodSpec{
	{"upbeat", "upbeat energetic pop, bright plucky synths, driving four-on-the-floor beat, feel-good, no vocals", []string{"Neon Drive", "Sunrise Rush"}},
	{"cinematic", "cinematic epic build, swelling strings and brass, dramatic percussion, no vocals", []string{"Ascent", "Horizon Line"}},
	{"corporate", "clean corporate motivational, light piano, soft beat, optimistic, no vocals", []string{"Momentum", "Clear Path"}},
	{"chill", "chill lofi, warm mellow keys, relaxed swing, hazy, no vocals", []string{"Afterglow", "Slow Sunday"}},
	{"dramatic", "dark dramatic tension building to a punchy electronic drop, no vocals", []string{"Undercurrent", "The Drop"}},
}

const trackLenMs = 30000

func main() {
	created, failed := 0, 0
	for _, m := range catalog {
		for _, title := range m.Titles {
			audio, err := elevenlabs.GenerateMusic(elevenlabs.MusicRequest{
				Prompt:            m.Prompt,
				LengthMs:          trackLenMs,
				ForceInstrumental: true,
			})
			if err != nil {
				log.Printf("generate %q (%s): %v", title, m.Mood, err)
				failed++
				continue
			}
			url, err := media.UploadAudio(audio, "catalog")
			if err != nil {
				log.Printf("upload %q: %v", title, err)
				failed++
				continue
			}
			id, err := trendlymodels.CreateMusicTrack(&trendlymodels.MusicTrack{
				Title:      title,
				Moods:      []string{m.Mood},
				URL:        url,
				DurationMs: trackLenMs,
				Provider:   "elevenlabs",
			})
			if err != nil {
				log.Printf("write %q: %v", title, err)
				failed++
				continue
			}
			created++
			fmt.Printf("✓ %s [%s] → %s (%s)\n", title, m.Mood, id, url)
		}
	}
	fmt.Printf("\nDone. created=%d failed=%d\n", created, failed)
}
