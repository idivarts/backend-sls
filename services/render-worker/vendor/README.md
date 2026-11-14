# vendor/

Third-party artefacts baked into the render image rather than fetched at run
time. A render must work with no network — that is what makes it reproducible,
and what stops a brand's material reaching a CDN on the way to a film.

## `gsap-3.15.0.min.js`

`hyperframes init` writes a jsdelivr `<script>` tag. Copied into each project
instead, so a render needs no network at all. Pinned to the version
Hyperframes 0.9.39 expects.

## `whisper-cli-aarch64`

`hyperframes transcribe` — which is what turns a voiceover into the word-level
timestamps a caption track is built from — shells out to `whisper-cli` from
[whisper.cpp]. When it cannot find one it **clones and compiles whisper.cpp**
into its cache on first use. In a Fargate task that is a fresh clone and a
full C++ build inside the render, every render, which would be slow enough to
look like a hang and is exactly the kind of work an image is for.

So the binary is built once, here, and copied into the image.

- **Source**: <https://github.com/ggml-org/whisper.cpp> at commit
  `d1be6fde11ac6e0407606b4e42fe72d34add8037` — the commit Hyperframes 0.9.39
  pins (`WHISPER_TAG = "v1.9.5"`, `WHISPER_COMMIT = d1be6fd…`), so this is the
  same binary it would have built for itself.
- **Target**: `linux/arm64`, matching the image. `armv8.2-a+fp16+dotprod` is
  the baseline — Graviton2 (Neoverse-N1) and later all have it, and ggml's
  fp16 NEON intrinsics do not compile without `+fp16` declared.
- **Statically linked** against libwhisper and ggml (`BUILD_SHARED_LIBS=OFF`),
  OpenMP off, so the only dynamic dependencies are libstdc++, libm, libgcc_s
  and libc — all already in `node:22-bookworm-slim`.
- **sha256**: `62a46ad1ccb13ec0b5f3ffb951a5f2cc1bda1a5ed0b604f9e7f9813bfc426002`

### Why a binary in git, and not a build stage in the Dockerfile

The worker image is built in CI on `ubuntu-latest` — an x86-64 runner
targeting `linux/arm64` through QEMU, with no registry layer cache. Compiling
whisper.cpp there would mean an emulated C++ build on **every** image build,
for a binary that changes when Hyperframes bumps its pin and at no other time.
Built natively on arm64 it takes about half a minute; emulated it is minutes,
repeatedly, for nothing.

The build is one command and the exact flags are below, so this is checkable
rather than magic: rebuild it and compare.

```sh
# From this directory. Requires Docker with an arm64 builder (native on
# Apple Silicon; `docker buildx create --platform linux/arm64` elsewhere).
./build-whisper-cli.sh
```

The whisper MODEL is not vendored — it is 148 MB, and a download pinned by
digest is as reproducible as a file in git without the weight. The Dockerfile
fetches `ggml-base.en.bin` from the same revision of Hyperframes' own model
repository and verifies its sha256.

[whisper.cpp]: https://github.com/ggml-org/whisper.cpp
