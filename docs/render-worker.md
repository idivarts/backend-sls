# Render worker

Design renders run on our infrastructure, not on the user's device. A design is
a self-contained HTML document; the worker loads it in headless Chromium and
captures it — one PNG per slide, or an MP4 with the soundtrack muxed in.

> **Replaces the CanvasKit scene-graph plan.** That approach (and its
> `SceneRenderQueue`) was deleted in `60a4449`. This document described it until
> 2026-10; everything below is the shipped design.

## Why the server

The client renderer used **html2canvas**, a JavaScript reimplementation of CSS
painting. It is not the browser's compositor, so soft shadows, blurs, blend
modes and gradient text were wrong in exports while correct in the preview —
and the AI had to be told to avoid half of CSS to work around it. It also cost
~1s per frame regardless of resolution, could not run on native at all, and
needed a browser tab held open.

Chromium paints the preview; now Chromium paints the export. Same engine, same
pixels.

## Shape

| Lane | Path | Why |
|---|---|---|
| images, carousels | `POST /render` → `ImageRenderQueue` (SQS) → `render_image` Lambda | Short jobs. Fargate's one-minute minimum and billed image pull would cost ~5x and add ~35s before rendering starts |
| video | `POST /render` → `batch:SubmitJob` → Fargate Spot task | No 15-minute ceiling, ~70% cheaper on Spot, and Batch already is the queue |

One container image serves both; only the entrypoint differs. The task exits
when the job is done, so **idle cost is zero** — there is no NAT gateway and no
always-on service.

Source: `services/render-worker/`. Infrastructure: `serverless.trendly.yml`
(`ImageRenderQueue`, `RenderVpc`, `RenderJobQueue`, `RenderJobDefinition`).

## Write-back

The worker writes the same fields the client used to, so nothing downstream
changed:

- revision: `renderUrl`, `renderStatus`, `renderProgress`, `renderError`
- content: `attachments`, `designRef.renderUrl`

The app watches the revision document it already subscribes to.

## Determinism

Same input, same pixels, on every host:

- `--font-render-hinting=none --disable-lcd-text --force-color-profile=srgb`
- fonts baked into the image (text metrics decide line breaks)
- every animation **paused** with `currentTime` set explicitly per frame, so a
  frame depends only on the time asked for
- the seek logic is `shared-libs/design/design-runtime.ts`, imported by both the
  worker and the app's preview frame — one implementation, so preview and export
  cannot drift

## Audio

Muxed by the same FFmpeg invocation that encodes the video, so there is never a
silent intermediate. A failed audio fetch **fails the job** rather than quietly
shipping a mute video.

⚠️ The graph that used to be documented here was wrong: `sidechaincompress`
outputs only its first input, so mapping it straight to the output drops the
voiceover entirely. The voice has to be split — one copy drives the ducking, the
other is mixed back in:

```
[1:a]volume=0.7[m];
[2:a]volume=1.0[v];
[v]asplit=2[vkey][vout];
[m][vkey]sidechaincompress=threshold=0.05:ratio=8:release=250[mduck];
[mduck][vout]amix=inputs=2:duration=first:dropout_transition=0[aout]
```

See `services/render-worker/src/audio.ts`.

## Metering

A render costs the org's token wallet, priced per slide and per second of video
— delivery (CDN egress), not compute, is what a render actually costs. Charged
once per revision at submission: a retry after a failure is free, a forced
re-render pays again. Numbers and their derivation:
`internal/models/trendlymodels/render_cost.go`.
