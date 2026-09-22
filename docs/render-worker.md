# Scene render worker — infra to provision

The AI-Studio scene graph is rendered to final media by an **out-of-band
container worker**, not a Lambda: deterministic video rendering (CanvasKit
frames + FFmpeg encode + audio mux) exceeds the Lambda envelope (15 min / 10 GB /
no GPU). Images can be rendered on the client (see below), but the device-
independent path (scheduled posts, mobile, bulk/AI content, re-renders) needs the
server worker.

## What already exists (backend)

- **Queue**: `SceneRenderQueue-<stage>` (SQS) — declared in `serverless.trendly.yml`.
  IAM `sqs:SendMessage` is granted to the API functions.
- **Enqueue**: `internal/trendlyapis/ai/render_enqueue.go` sends a `RenderJob`
  `{brandId, contentId, revisionId, docType}` to `RENDER_QUEUE_URL` (best-effort;
  no-op if unset, so nothing breaks before the worker exists).
- **Scene source of truth**: `brands/{brandId}/contents/{contentId}/scenes/{revisionId}`
  holds the `scenegraph.Document`. `pkg/scenegraph.RenderSVG` already serializes a
  scene to SVG; `ComputeLayout` gives element boxes.
- **Write-back target**: the content's `sceneRef.renderUrl` (image) / `sceneRef.proxyUrl`
  (video preview) — update via the `scenes`/content model.

## What you provision

1. **A container render worker** (ECS/Fargate task or a small always-on service),
   triggered by `SceneRenderQueue`:
   - Read the `RenderJob`, load the scene revision from Firestore.
   - **Image**: `RenderSVG` → rasterize to PNG via **resvg** (or CanvasKit).
     Upload to the image bucket; write `sceneRef.renderUrl`; also set the content
     `attachments[0].imageUrl` so publishing picks it up.
   - **Video**: expand each scene's animation presets → per-frame CanvasKit draws
     → FFmpeg encode; **mux `content.audio`** (music bed + ducked voiceover);
     burn captions if enabled. Upload MP4; write `sceneRef.renderUrl` and set the
     content video attachment. Also emit a fast low-res **proxy MP4** →
     `sceneRef.proxyUrl` for in-app preview.
   - Use the **Firebase Admin SDK** (service account) for the Firestore writes —
     the `scenes` collection is `allow write: if false` for clients.
2. **Set `RENDER_QUEUE_URL`** (GitHub Actions var) to the queue URL so the API
   starts enqueuing.
3. **Fonts**: bundle **Quicksand** (the brand font) in the worker image so text
   metrics match the SVG/preview (pin fonts for deterministic output).

## Fonts / pipeline references

- Web (later, client-side export): WebCodecs `VideoEncoder` + `mp4-muxer`.
- Server/worker: CanvasKit (headless, Node) for frames + **FFmpeg** for encode/mux.
- Static images: **resvg** (SVG→PNG) is the lightest path for Module 1.
- Avoid: `ffmpeg-kit` (retired), `react-native-skia-video` (beta, no audio),
  Remotion (free only ≤3-person teams).

## Image v1 without the worker (interim)

Until the worker exists, images still work end-to-end: the app renders the scene
with `react-native-svg`, captures it to a PNG (`react-native-view-shot`), uploads
it, and sets both `attachments[0].imageUrl` and `sceneRef.renderUrl`. This is
WYSIWYG (preview == captured export) and needs no server rasterizer. **Video
always needs the worker** (device-independent MP4 for the scheduler).

## Audio mux command (reference)

Music bed under a ducked voiceover, then muxed onto the silent video:

```
ffmpeg -i video.mp4 -i music.mp3 -i voice.mp3 \
  -filter_complex "[1:a]volume=0.6[m];[2:a]volume=1.0[v]; \
     [m][v]sidechaincompress=threshold=0.05:ratio=8[mixed]" \
  -map 0:v -map "[mixed]" -c:v copy -shortest out.mp4
```

## Checklist for you

- [ ] Stand up the Fargate render worker consuming `SceneRenderQueue`.
- [ ] Bundle Quicksand + resvg/CanvasKit + FFmpeg in the worker image.
- [ ] Give the worker a Firebase service account for Firestore write-back.
- [ ] Set `RENDER_QUEUE_URL` (GitHub Actions var) once the worker is live.
