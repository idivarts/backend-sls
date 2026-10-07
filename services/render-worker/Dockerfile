# Render worker — headless Chromium + FFmpeg.
#
# ONE image serves both front doors:
#   • AWS Batch (video)  → default ENTRYPOINT runs cli.js and the task exits,
#                          which is what keeps Fargate costing nothing when idle
#   • Lambda (images)    → the function's ImageConfig overrides ENTRYPOINT to the
#                          AWS Runtime Interface Client and CMD to the handler
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

# aws-lambda-ric compiles from source, so the toolchain is needed here and
# thrown away with this stage.
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 make g++ ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY services/render-worker/package.json services/render-worker/package-lock.json ./services/render-worker/
RUN cd services/render-worker && npm ci --ignore-scripts

COPY shared-libs/design ./shared-libs/design
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
    && npm i --no-save aws-lambda-ric \
    && npx playwright install --with-deps chromium \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /app/services/render-worker/dist ./services/render-worker/dist

# Brand fonts, if any are dropped in. Kept last so adding one doesn't rebuild
# Chromium. Quicksand belongs here once the design prompt is allowed to ask for
# it by name (Phase 3).
COPY services/render-worker/fonts/ /usr/share/fonts/truetype/trendly/
RUN fc-cache -f || true

WORKDIR /app/services/render-worker
ENTRYPOINT ["node", "dist/services/render-worker/src/cli.js"]
