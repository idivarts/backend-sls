#!/usr/bin/env bash
#
# Rebuild vendor/whisper-cli-aarch64.
#
# Run this only when Hyperframes bumps the whisper.cpp commit it pins
# (grep WHISPER_COMMIT in node_modules/hyperframes/dist). See README.md for
# why the binary is vendored instead of built in CI.
#
# Needs Docker with an arm64 builder — native on Apple Silicon.
set -euo pipefail
cd "$(dirname "$0")"

# The commit Hyperframes 0.9.39 pins (v1.9.5).
COMMIT="${WHISPER_COMMIT:-d1be6fde11ac6e0407606b4e42fe72d34add8037}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

cat > "$WORK/Dockerfile" <<'DOCKER'
FROM node:22-bookworm-slim AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
      git cmake build-essential ca-certificates && rm -rf /var/lib/apt/lists/*
ARG COMMIT
RUN git clone https://github.com/ggml-org/whisper.cpp /src && cd /src && git checkout "$COMMIT"
# GGML_NATIVE=OFF so the binary is not tuned to whatever built it; the arch
# baseline is Graviton2's (Neoverse-N1). +fp16 is required, not an
# optimisation: ggml's NEON fp16 intrinsics fail to compile without it
# ("inlining failed in call to always_inline vfmaq_f16: target specific
# option mismatch").
RUN cmake -S /src -B /build -DCMAKE_BUILD_TYPE=Release \
      -DBUILD_SHARED_LIBS=OFF -DGGML_OPENMP=OFF \
      -DGGML_NATIVE=OFF -DGGML_CPU_ARM_ARCH=armv8.2-a+fp16+dotprod \
      -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_SERVER=OFF \
 && cmake --build /build --config Release -j"$(nproc)" --target whisper-cli \
 && ldd /build/bin/whisper-cli

FROM scratch
COPY --from=build /build/bin/whisper-cli /whisper-cli
DOCKER

docker buildx build --platform linux/arm64 \
  --build-arg "COMMIT=$COMMIT" \
  --output "type=local,dest=$WORK/out" "$WORK"

install -m 0755 "$WORK/out/whisper-cli" ./whisper-cli-aarch64
echo "built from $COMMIT"
shasum -a 256 ./whisper-cli-aarch64
echo "→ update the sha256 in README.md if it changed"
