# Render worker — headless Chromium + FFmpeg.
#
# ONE image serves both front doors:
#   • AWS Batch (video)  → default ENTRYPOINT runs cli.js and the task exits,
#                          which is what keeps Fargate costing nothing when idle
#   • Lambda (images)    → the function's ImageConfig overrides ENTRYPOINT to
#                          lambda-bootstrap.js, which long-polls the Lambda
#                          Runtime API itself (see src/lambda-bootstrap.ts for
#                          why aws-lambda-ric is not used)
#
# Built for linux/arm64 to match the rest of the stack (provider.architecture is
# already arm64) and to take the ~20% Graviton discount on both Lambda and
# Fargate. Playwright publishes an arm64 Linux Chromium, so the browser is the
# same build on every architecture — unlike Google's chrome-headless-shell,
# which is x86_64-only and was the reason this decision needed checking at all.
#
# Build from the REPO ROOT so shared-libs is in context:
#   docker build -f services/render-worker/Dockerfile -t render-worker .

FROM node:22-bookworm-slim AS build
WORKDIR /app

# Nothing here compiles: tsc only, and every dependency with a native component
# (sharp) ships a prebuilt linux-arm64 binary, which is why --ignore-scripts is
# safe on both stages.
COPY services/render-worker/package.json services/render-worker/package-lock.json ./services/render-worker/
RUN cd services/render-worker && npm ci --ignore-scripts

COPY shared-libs/design ./shared-libs/design
# ⚠️ design/ is NOT self-contained any more. studio-runtime.ts imports the
# studio document contract from firestore/trendly-pro/models, which is the
# point of that file — the app and this worker must measure a studio the same
# way, and a second copy of the types in design/ would be the two-sources-of-
# truth bug the studio exists to remove. So the models travel with it.
#
# Copied as a directory rather than the two files the chain needs today
# (studio.ts -> asset.ts), because tsc only type-checks what is actually
# imported: adding a third import later must not break the image build, which
# is a failure only CI sees, ten minutes in.
COPY shared-libs/firestore/trendly-pro/models ./shared-libs/firestore/trendly-pro/models
COPY services/render-worker/tsconfig.json ./services/render-worker/
COPY services/render-worker/src ./services/render-worker/src
RUN cd services/render-worker && npm run build


FROM node:22-bookworm-slim AS runtime
WORKDIR /app

ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    NODE_ENV=production

# FFmpeg does every encode and the whole audio mix. The font set is the other
# half of determinism: text metrics decide line breaks, so a design must find
# the same families here that the preview found in the user's browser.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    fonts-liberation fonts-dejavu-core fonts-noto-core fonts-noto-color-emoji \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY services/render-worker/package.json services/render-worker/package-lock.json ./services/render-worker/
RUN cd services/render-worker && npm ci --omit=dev --ignore-scripts \
    && npx playwright install --with-deps chromium \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /app/services/render-worker/dist ./services/render-worker/dist

# GSAP, vendored rather than fetched.
#
# `hyperframes init` writes a jsdelivr <script> tag, which would make every
# render depend on a CDN being reachable. Baking it into the image and copying
# it into each project is what lets a render run with no network at all — and
# a render that cannot reach the internet cannot leak a brand's material to
# one either. Pinned to the version Hyperframes 0.9.39 expects.
COPY services/render-worker/vendor/ /app/services/render-worker/vendor/

# whisper, for the captions half of a voiceover.
#
# `hyperframes transcribe` is what turns a voiceover into word-level
# timestamps, and it shells out to whisper-cli. Given none, it CLONES AND
# COMPILES whisper.cpp into its cache on first use — inside the render, in a
# Fargate task that is thrown away afterwards, every time. So the binary is
# vendored (built once on arm64; see vendor/README.md for the exact build and
# why it is not a stage here) and the model is fetched by digest.
#
# base.en rather than the CLI's small.en default: a third of the size for
# captions that are a few seconds of clean studio TTS, and the model is named
# explicitly at the call site so the cached one is the one used. Fetched from
# the same model repository and revision Hyperframes itself pins, and
# sha256-checked, because an unverified 148 MB download into an image is a
# supply chain nobody is watching.
COPY services/render-worker/vendor/whisper-cli-aarch64 /usr/local/bin/whisper-cli
RUN chmod 0755 /usr/local/bin/whisper-cli \
    && apt-get update && apt-get install -y --no-install-recommends curl \
    && mkdir -p /root/.cache/hyperframes/whisper/models \
    && curl -fsSL -o /root/.cache/hyperframes/whisper/models/ggml-base.en.bin \
         https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-base.en.bin \
    && echo "a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002  /root/.cache/hyperframes/whisper/models/ggml-base.en.bin" \
         | sha256sum -c - \
    && rm -rf /var/lib/apt/lists/*
# Fatal if the binary cannot run here — a wrong architecture or a missing
# libstdc++ is a thing to learn at build time, not from a render that quietly
# produces a film with no captions.
RUN whisper-cli --help > /dev/null
ENV HYPERFRAMES_WHISPER_PATH=/usr/local/bin/whisper-cli

# Brand fonts, if any are dropped in. Kept last so adding one doesn't rebuild
# Chromium. Quicksand belongs here once the design prompt is allowed to ask for
# it by name (Phase 3).
COPY services/render-worker/fonts/ /usr/share/fonts/truetype/trendly/
RUN fc-cache -f || true

# The HeyGen CLI, which `media-use` shells out to.
#
# Not optional and not a second integration: `hyperframes media-use resolve`
# delegates bgm, image, voice and avatar-video to it, and without it every
# declaration fails in 200ms with "heygen CLI not found — it's the free path
# for bgm/image/voice/avatar-video". It is HeyGen's own single binary, it
# supports linux/arm64, and it authenticates from HEYGEN_API_KEY, which the
# Batch job definition already passes — so nothing interactive happens here
# and no credential is baked into a layer.
#
# `|| true` deliberately: a film that declares no media must still render if
# HeyGen's installer is briefly unreachable, and the media step already
# reports a missing CLI as a warning rather than a failure. The version is
# echoed so the build log records what the image actually got.
# curl is not in node:22-bookworm-slim, and the installer needs bash — it uses
# `set -o pipefail`, which dash refuses. The first attempt at this shipped a
# green deploy with a dead media path because `curl: not found` was swallowed.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && curl -fsSL https://static.heygen.ai/cli/install.sh | bash \
    && rm -rf /var/lib/apt/lists/*
ENV PATH="/root/.local/bin:${PATH}"
# Fatal on purpose. A render image whose media path is silently dead is worse
# than a deploy that waits: the failure then surfaces six minutes and one
# preview later, in a log nobody is watching. If HeyGen's installer is down,
# this blocks the deploy — which is the right way round, because the
# alternative shipped exactly that silent breakage once already.
RUN heygen --version

WORKDIR /app/services/render-worker
ENTRYPOINT ["node", "dist/services/render-worker/src/cli.js"]
