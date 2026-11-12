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

# Brand fonts, if any are dropped in. Kept last so adding one doesn't rebuild
# Chromium. Quicksand belongs here once the design prompt is allowed to ask for
# it by name (Phase 3).
COPY services/render-worker/fonts/ /usr/share/fonts/truetype/trendly/
RUN fc-cache -f || true

WORKDIR /app/services/render-worker
ENTRYPOINT ["node", "dist/services/render-worker/src/cli.js"]
