# AI Studio — setup index

The AI-Native Image & Video Studio + AI audio features span three external
setups. Details in the linked docs; this is the one-page env matrix.

- **ElevenLabs** (music + voiceover) → [elevenlabs-setup.md](./elevenlabs-setup.md)
- **Canva Connect** (deep-edit bridge) → [canva-setup.md](./canva-setup.md)
- **Scene render worker** (Fargate) → [render-worker.md](./render-worker.md)

## Env / secret matrix (GitHub Actions → serverless)

| Name | Kind | Used by | Notes |
|---|---|---|---|
| `ELEVENLABS_API_KEY` | secret | `trendly_studio`, `trendly_ai` | Music + TTS |
| `CANVA_CLIENT_ID` | var | `trendly_studio` | Public integration id |
| `CANVA_CLIENT_SECRET` | secret | `trendly_studio` | Token exchange (Basic auth) |
| `CANVA_REDIRECT_URI` | var | `trendly_studio` | Must match the portal redirect URL |
| `RENDER_QUEUE_URL` | var | `trendly_ai`, `trendly_studio` | Set once the render worker exists |

All are wired in `serverless.trendly.yml` (`provider.environment`) and
`.github/workflows/deploy-trendly.yaml`. Secrets go under `secrets.*`, non-secret
ids/URLs under `vars.*`.

## New AWS resources

- `SceneRenderQueue-<stage>` (SQS) — added; the Fargate worker consumes it.

## New Firestore collections (rules + indexes already updated)

| Path | Access | Model |
|---|---|---|
| `brands/{b}/contents/{c}/scenes/{rev}` | read: members/public-share · write: backend | `content_scene.go` |
| `brands/{b}/generatedAudio/{id}` | read: members · write: backend | `generated_audio.go` |
| `canvaConnections/{managerId}` | fully backend-only | `canva_connection.go` |
| `canvaOauthStates/{state}` | fully backend-only | `canva_oauth_state.go` |

Composite index added (both `firestore.indexes.json` + `.enterprise.json`):
`generatedAudio` on `kind` ASC + `createdAt` DESC.

## New plan entitlement

`OrgEntitlements.canvaBridge` (Pro+). Audio + scene edits are token-wallet metered
(not a plan gate).
