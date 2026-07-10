# ElevenLabs setup — AI music & voiceovers

Backend for the "Add AI voiceovers & background music" ticket. The ElevenLabs
key is **server-side only** — the RN app never calls ElevenLabs directly.

## 1. Account & plan

1. Create/confirm an ElevenLabs account at <https://elevenlabs.io>.
2. Choose a **paid plan** — Eleven Music commercial use (social / paid-marketing)
   is cleared on paid plans; the free tier is **not** licensed for commercial
   use. Music v2 got an API price cut (up to 50% off); confirm current credit
   pricing for our expected volume.
3. Both **Music** and **Text-to-Speech** are covered by the same account/key.

## 2. API key

1. ElevenLabs dashboard → Profile → **API Keys** → create a key scoped to
   Music + Text-to-Speech.
2. Store it as the GitHub Actions secret **`ELEVENLABS_API_KEY`** (repo or org
   level, same place as `OPENAI_API_KEY`). The deploy workflow injects it; the
   serverless config reads `${env:ELEVENLABS_API_KEY}` into every function.
3. For local dev, export `ELEVENLABS_API_KEY` in your shell before `sls offline`
   or set it in your local env.

## 3. What the backend exposes (already implemented)

Lambda `trendly_studio_apis`, routes under `/api/media` (manager session):

| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/api/media/voices` | List available voices for the picker |
| `POST` | `/api/media/brands/:brandId/audio/music` | Generate a music bed (`{prompt, lengthMs, instrumental}`) |
| `POST` | `/api/media/brands/:brandId/audio/voiceover` | Generate a voiceover (`{text, voiceId, language, model}`) |
| `GET`  | `/api/media/brands/:brandId/audio` | List a brand's generated audio (`?kind=music\|voiceover`) |

The AI chat can also generate audio via the `generate_music` / `generate_voiceover`
tools (content module).

- `pkg/elevenlabs` — the client (Music `POST /v1/music`, TTS
  `POST /v1/text-to-speech/{voice_id}`, `GET /v1/voices`).
- Audio bytes are stored in the attachment S3 bucket (`audio/` prefix) → served
  from CloudFront, same as image/video uploads.
- Each generation is recorded in Firestore `brands/{brandId}/generatedAudio/{id}`
  (backend-write-only; brand members read to preview).

## 4. Metering (token wallet)

Generation is metered against the org token wallet on **both** sides:
- Backend gate: `402 {error:"upgrade_required", reason:"tokens_exhausted"}` when
  the wallet is empty; debit after success via `MeterUSD`.
- Cost model is **provisional** (`MusicCostUSD` / `TTSCostUSD` in
  `internal/trendlyapis/media/audio.go`) — margin-safe estimates mapping
  ElevenLabs credit cost → wallet tokens. **Re-tune** once real per-request
  credit costs are observed at volume (Phase-0 spike in the ticket).

## 5. Render / mux

Generated music + voiceover are **baked into the video server-side** at render
time (music bed + ducked voiceover → single MP4) — see `render-worker.md`. This
is what makes scheduled posts auto-publish with audio intact (Meta does not strip
baked-in audio, unlike API-attached IG/FB music).

## 6. Licensing note (surface in-product)

Eleven Music v2 is trained on licensed stems and cleared for broad commercial use
on paid plans (social / paid-marketing). Only film/TV/large-studio-games need an
Enterprise license — not our use case. Document this in the audio panel so brands
know the music is safe to publish.

## Checklist for you

- [ ] Paid ElevenLabs plan confirmed for commercial use + expected volume.
- [ ] `ELEVENLABS_API_KEY` added as a GitHub Actions **secret**.
- [ ] Validate Music + TTS quality on 3–4 real content samples (ticket Phase 0).
- [ ] Confirm/adjust the provisional cost constants after the spike.
