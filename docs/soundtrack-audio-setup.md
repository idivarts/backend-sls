# Soundtrack (video audio) setup

The video Studio's audio module was rebuilt from a "generate music" box into a
**soundtrack tool**: search/preview/select a licensed music library, pick a voice
(with sample previews) for AI voiceover, hear the mix on preview, and have the
audio muxed into the exported MP4.

This doc covers the external/dashboard setup you need to action. **No new vendor
or dashboard is required** — it reuses the existing ElevenLabs key and S3.

## 1. Library source (decision)

The music library is a **curated preset catalog we generate once via the
ElevenLabs Music API** and store ourselves:
- Legally clean: ElevenLabs Music (paid plans) is cleared for commercial
  social-media use — no third-party stock-licensing deals needed.
- No per-use cost for the user: browsing/selecting a catalog track is a **plan
  entitlement (free)**; only *generating* a new track/voice meters the wallet.
- Real audition-first UX: users search by mood/keyword, preview inline, and pick.

Brand-specific AI-generated tracks (the old "generate") still work and show up
under "Your tracks" (the existing `generatedAudio` collection).

## 2. ElevenLabs — what you need to confirm

You already set `ELEVENLABS_API_KEY` (see `elevenlabs-setup.md`). Additionally:

1. **Plan tier** — confirm the plan includes the **Music API** and enough monthly
   credits to (a) generate the initial catalog and (b) serve ongoing per-brand
   generation + voiceovers. Seeding the catalog is a **one-time** credit spend
   (~N tracks × moods × ~30s each).
2. **Commercial-use tier** — confirm Eleven Music commercial use (social / paid
   marketing) is enabled on the plan. Only film/TV/large-studio needs Enterprise
   (not us).
3. **Voices** — no new permission. The Studio's voice picker uses
   `GET /v1/voices`, which already returns each voice's `preview_url` for the
   in-app "hear this voice" samples. No cloning required for v1.

There is **no new API key, scope, or dashboard** to create — it's the same
ElevenLabs account/key.

## 3. Seed the music catalog (one-time, you run it)

A seeding script generates the preset catalog and writes it to the shared
`musicLibrary` Firestore collection.

- Script: `backend-sls/scripts/seed_music_library/main.go`
- What it does: for each mood in a fixed list (upbeat, cinematic, corporate,
  chill, dramatic, …) it generates a few ~30s instrumental tracks via ElevenLabs
  Music, uploads each to S3 (`audio/` prefix → CloudFront), and writes a
  `musicLibrary/{trackId}` doc `{title, moods, url, durationMs, source}`.
- Run it once (locally or as a one-off job) with the ElevenLabs key + AWS creds +
  `service-account.json` present:
  ```bash
  cd backend-sls
  ELEVENLABS_API_KEY=... ATTACHMENT_S3_BUCKET_NAME=... ATTACHMENT_CF_DISTRIBUTION_URL=... \
    go run ./scripts/seed_music_library
  ```
- Re-runnable to add more tracks; safe to expand the mood/track lists.

## 4. Firestore

- New shared collection **`musicLibrary`** — the preset catalog. Rules: **read by
  any authenticated user, write backend-only** (Admin SDK / seed script). Added to
  `firestore.rules`.
- No composite index needed for v1 (mood is a single-field filter; text search is
  client-side over the small catalog). If the catalog grows large and needs
  `where(mood) + orderBy(createdAt)`, add that composite index to both index files.

## 5. Rendered-MP4 audio (web)

On "Save render" (web WebCodecs path), the selected music + voiceover are
**decoded, mixed (music auto-ducked under the voice), AAC-encoded, and muxed into
the MP4** alongside the video track. This needs no server or extra vendor — it's
Web Audio + WebCodecs + mp4-muxer in the browser. Caveat: same CORS rule — the
audio URLs (CloudFront) must send `Access-Control-Allow-Origin` so the browser can
`fetch()` + `decodeAudioData()` them; otherwise the encode fails. (Native video
render remains the server-side holdout.)

## Checklist for you
- [ ] Confirm ElevenLabs plan includes Music API + commercial use + enough credits.
- [ ] Run `scripts/seed_music_library` once to populate the catalog.
- [ ] Ensure CORS (`Access-Control-Allow-Origin`) on the attachment/audio
      CloudFront + S3 so both html2canvas (images) and audio decode work.
- [ ] Deploy `firestore.rules` (adds the `musicLibrary` read rule).
