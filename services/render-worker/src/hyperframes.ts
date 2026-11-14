/**
 * Rendering a Design Studio v3 project with the Hyperframes CLI.
 *
 * ── Why a subprocess and not a library ──────────────────────────────────────
 * Hyperframes publishes a programmatic API at `hyperframes/api`, but it
 * exposes only transcribe, removeBackground and offloadHeavyMedia, and says in
 * its own types that it is "for HyperFrames' own apps; it may change between
 * minor versions". Rendering, linting and measuring are CLI-only. So the CLI
 * is the integration surface, and this module is the thin thing around it.
 *
 * ── Three defaults of theirs we deliberately override ───────────────────────
 * Each of these was found by actually running it in Phase 0, not by reading
 * the docs, and each would have failed in production rather than in CI:
 *
 *  1. `hyperframes init` writes a CDN <script> tag for GSAP. Every render
 *     would then depend on jsdelivr being reachable. We vendor GSAP into the
 *     image and copy it into each project, so a render needs no network at
 *     all — proved by rendering under `--network none`.
 *  2. Their published Dockerfile declares `ARG TARGETARCH=amd64`, which
 *     defeats BuildKit's automatic platform value and pulls x86-64 ffmpeg into
 *     an arm64 image. We do not use their Dockerfile; we point the CLI at the
 *     ffmpeg already in ours.
 *  3. Telemetry is on by default, and the bundled skills emit usage beacons.
 *     Both are switched off here, for every invocation.
 *
 * ── The gate ────────────────────────────────────────────────────────────────
 * `check --json` runs BEFORE the render and before the wallet is charged. On a
 * hand-written reel in Phase 0 it caught five real errors; it also audits
 * layout, contrast and craft, and returns a `Fix:` line a model can act on.
 * A design that fails the gate never reaches the encoder.
 */
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { chromium } from "playwright";
import { copyFile, mkdir, readFile, readdir, stat, writeFile } from "node:fs/promises";
import { dirname, isAbsolute, join, resolve } from "node:path";

/**
 * Where the vendored GSAP sits. Copied in by the Dockerfile.
 *
 * Four levels up, not three: tsc emits to dist/services/render-worker/src, so
 * __dirname is that deep inside the package and `vendor/` sits at its root.
 * Overridable by env so a test or a local run can point elsewhere.
 */
const GSAP_VENDOR =
    process.env.TRENDLY_GSAP_VENDOR_PATH ?? resolve(__dirname, "../../../../vendor/gsap-3.15.0.min.js");

/** The path a project references it by, relative as their contract requires. */
const GSAP_PROJECT_PATH = "assets/vendor/gsap-3.15.0.min.js";

export interface HyperframesEnvOptions {
    /** Overrides for tests. */
    ffmpegPath?: string;
    ffprobePath?: string;
}

/**
 * The absolute path to the CLI entry point.
 *
 * Resolved through the module graph rather than left to `npx` or PATH,
 * because every invocation runs with cwd set to the MATERIALISED PROJECT —
 * a temp directory outside this package, with no node_modules of its own.
 * `npx --no-install` would search that directory's (empty) chain and then
 * PATH, find nothing, and fail every render.
 */
function hyperframesEntry(): string {
    if (process.env.TRENDLY_HYPERFRAMES_ENTRY) return process.env.TRENDLY_HYPERFRAMES_ENTRY;
    const pkgJson = require.resolve("hyperframes/package.json");
    return join(dirname(pkgJson), "bin", "hyperframes.mjs");
}

/**
 * The environment every invocation runs with.
 *
 * Built from an ALLOWLIST, not from `...process.env`. Spreading the task
 * environment would hand the subprocess every variable the Lambda has —
 * including HEYGEN_API_KEY once Phase 4 adds it to provider.environment — and
 * a render that can reach a paid API is a render that can spend outside the
 * wallet, with no 402 in front of it. Media generation is a separate, metered
 * step; a key absent from this environment is a capability that cannot be
 * billed by accident.
 *
 * Telemetry is off for the same class of reason: this worker renders other
 * people's brand material, and none of it should leave the container.
 */
const ENV_ALLOWLIST = ["PATH", "HOME", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL", "NODE_ENV", "CONTAINER"];

export function hyperframesEnv(opts: HyperframesEnvOptions = {}): NodeJS.ProcessEnv {
    const env: NodeJS.ProcessEnv = {};
    for (const key of ENV_ALLOWLIST) {
        const value = process.env[key];
        if (value !== undefined) env[key] = value;
    }
    env.HYPERFRAMES_NO_TELEMETRY = "1";
    env.DO_NOT_TRACK = "1";
    // ffmpeg is pointed at only when we actually know where it is.
    //
    // An earlier version defaulted to /usr/bin/ffmpeg, which is right for our
    // image (apt) and wrong for theirs (/usr/local/bin, a pinned static
    // build). Setting the variable to a path that does not exist is worse
    // than not setting it: the CLI trusts it, fails with "Configured path
    // does not exist", and tells the user to apt-get install an ffmpeg that
    // is already there. Unset, it resolves from PATH, which is correct in
    // both images. Found by running this module against the real CLI; no
    // unit test would have noticed.
    const ffmpeg = opts.ffmpegPath ?? process.env.HYPERFRAMES_FFMPEG_PATH;
    const ffprobe = opts.ffprobePath ?? process.env.HYPERFRAMES_FFPROBE_PATH;
    if (ffmpeg && existsSync(ffmpeg)) env.HYPERFRAMES_FFMPEG_PATH = ffmpeg;
    if (ffprobe && existsSync(ffprobe)) env.HYPERFRAMES_FFPROBE_PATH = ffprobe;

    // Where whisper-cli is, for `transcribe`.
    //
    // The image sets this and also puts the binary on PATH, so either of the
    // CLI's two discovery routes finds it. Passed through on the same terms
    // as ffmpeg — only when it exists — because a path that is set and wrong
    // is worse than one that is unset: the CLI trusts it instead of looking.
    const whisper = process.env.HYPERFRAMES_WHISPER_PATH;
    if (whisper && existsSync(whisper)) env.HYPERFRAMES_WHISPER_PATH = whisper;
    // The model cache, when it has been moved off $HOME.
    if (process.env.HYPERFRAMES_CACHE_DIR) env.HYPERFRAMES_CACHE_DIR = process.env.HYPERFRAMES_CACHE_DIR;
    if (process.env.PRODUCER_HEADLESS_SHELL_PATH) {
        env.PRODUCER_HEADLESS_SHELL_PATH = process.env.PRODUCER_HEADLESS_SHELL_PATH;
    }

    // Tell the CLI where a browser is, or it will not find one on arm64.
    //
    // Hyperframes downloads chrome-headless-shell from chrome-for-testing,
    // which publishes no linux-arm64 build — so on our Graviton worker it
    // gives up with "Chrome Headless Shell is not available for Linux ARM64"
    // and suggests apt-get installing chromium-browser. Their own Dockerfile
    // sidesteps this by shipping Playwright's pinned arm64 headless shell.
    //
    // We already have exactly that: the image runs `playwright install
    // chromium`, and the worker launches it for every v1/v2 render. So the
    // browser is resolved from the Playwright package rather than installed
    // a second time — one browser in the image, one version to reason about.
    const browser = process.env.HYPERFRAMES_BROWSER_PATH ?? resolvePlaywrightChromium();
    if (browser) env.HYPERFRAMES_BROWSER_PATH = browser;

    return env;
}

/**
 * The Chromium that Playwright installed into this image, or undefined when
 * it cannot be resolved — in which case the CLI is left to its own discovery
 * and will say so clearly if it fails.
 */
function resolvePlaywrightChromium(): string | undefined {
    try {
        const path = chromium.executablePath();
        return path && existsSync(path) ? path : undefined;
    } catch {
        return undefined;
    }
}

export interface CliResult {
    code: number | null;
    stdout: string;
    stderr: string;
}

/**
 * Run the CLI in a project directory.
 *
 * Output is captured rather than streamed because two of their lines are
 * structured and worth parsing: `@hf-result` on success and `@hf-diagnostic`
 * on an audio failure. Scraping the human-readable progress bar instead would
 * break the first time they restyle it.
 */
export function runHyperframes(
    projectDir: string,
    args: string[],
    opts: { timeoutMs?: number; env?: NodeJS.ProcessEnv } = {}
): Promise<CliResult> {
    const timeoutMs = opts.timeoutMs ?? 15 * 60_000;
    return new Promise((resolvePromise, reject) => {
        const child = spawn(process.execPath, [hyperframesEntry(), ...args], {
            cwd: projectDir,
            env: opts.env ?? hyperframesEnv(),
        });
        let stdout = "";
        let stderr = "";
        const timer = setTimeout(() => {
            child.kill("SIGKILL");
            reject(new Error(`hyperframes ${args[0]} timed out after ${timeoutMs}ms`));
        }, timeoutMs);

        child.stdout.on("data", (d) => (stdout += d.toString()));
        child.stderr.on("data", (d) => (stderr += d.toString()));
        child.on("error", (err) => {
            clearTimeout(timer);
            reject(err);
        });
        child.on("close", (code) => {
            clearTimeout(timer);
            resolvePromise({ code, stdout, stderr });
        });
    });
}

/**
 * Put the vendored GSAP into the project and make EVERY document load it
 * locally.
 *
 * Rewriting any CDN tag rather than only appending one, because a project that
 * came from `hyperframes init` or from a template will carry the jsdelivr tag,
 * and leaving it in place would reintroduce the network dependency we just
 * removed — silently, since it would still render on a machine with network.
 *
 * ── Why every document, not just index.html ────────────────────────────────
 * Each composition under `compositions/` is a SEPARATE document that the
 * Hyperframes runtime mounts in its own context, so a script tag in
 * index.html does not reach it. index.html alone was the first version of
 * this, and it let a real render fail on dev with
 *
 *     check failed: Composition uses GSAP but no GSAP script is loaded.
 *
 * on a project whose three compositions each called `gsap.timeline()` with no
 * tag anywhere. The run before it had passed — because that one's model output
 * happened to include a CDN tag. That is the shape of bug worth removing
 * rather than retrying: the project is only renderable when the model
 * remembers something it is not reliable about, and the failure is charged to
 * the user.
 *
 * So GSAP is injected wherever it is used and not loaded, with a path relative
 * to the file being patched — `compositions/hook.html` needs `../assets/...`,
 * not `assets/...`.
 */
/**
 * Take the player's runtime tag out before the CLI opens the project.
 *
 * index.html carries it so the app can play the film with no second copy of
 * the document (pkg/hfproject/player.go explains why there used to be two).
 * It points at CloudFront, and a render must work with no network at all —
 * proved under `--network none`, and the whole reason GSAP is vendored.
 *
 * The CLI injects its own runtime anyway, so removing this costs the render
 * nothing: measured, the frames are byte-identical either way.
 */
const playerRuntimeTag = /[ \t]*<script[^>]+src=["'][^"']*\/design-player\/[^"']*runtime\.js["'][^>]*>\s*<\/script>\s*\n?/gi;

export function stripPlayerRuntime(html: string): string {
    return html.replace(playerRuntimeTag, "");
}

/**
 * Put the player's script tags BACK, so the document that returns to the
 * bucket is the one the app can play.
 *
 * The exact inverse of `stripPlayerRuntime` + `vendorGsap`, and it exists
 * because those two are not symmetric with what gets stored. The worker
 * downloads the stored film, mutates it for the CLI (runtime out, GSAP
 * local) and uploads it again. An earlier ordering uploaded the mutated
 * copy, and every design authored in that window is still sitting in the
 * bucket with no runtime tag and a `assets/vendor/gsap-*.js` src that was
 * never uploaded — a 403, no animation library, and a black rectangle.
 * Those designs cannot heal on their own: their media is already resolved,
 * so the resolver has nothing to do and the upload never fires.
 *
 * Running this on every preparation makes re-preparing a design repair it.
 *
 * The CDN base is read out of the project's own player.html rather than
 * configured here. That file is written by the same Go code that wrote the
 * script tags (pkg/hfproject/player.go), so it names the exact asset
 * version this project was built against — and pinning the version is the
 * whole reason those assets are addressed by version. Guessing a host here
 * would eventually serve a project a runtime it was not written for.
 *
 * Unparseable or missing player.html → the html is returned untouched. A
 * document we cannot heal confidently is better left as it is: the render
 * does not need these tags at all.
 */
export function restorePlayerShape(html: string, playerHTML: string | undefined): string {
    if (!playerHTML) return html;
    const base = playerHTML.match(/(https?:\/\/[^"']*\/design-player\/[^"']*\/)player\.js/i)?.[1];
    if (!base) return html;

    let out = html;

    // A local GSAP src is this worker's own doing — put it back on the CDN.
    // The basename carries the version, so it is reused rather than restated.
    out = out.replace(
        /<script([^>]*)\ssrc=["'](?:\.\.\/)*assets\/vendor\/(gsap-[^"'/]+)["']([^>]*)>/gi,
        (_m, pre, file, post) => `<script${pre} src="${base}${file}"${post}>`
    );

    // Already carries a runtime tag — whatever wrote it knew the version.
    if (/<script[^>]+src=["'][^"']*\/design-player\/[^"']*runtime\.js["']/i.test(out)) return out;

    const tag = `<script src="${base}runtime.js"></script>`;
    // Same placement as the Go writer: immediately inside <head>, before the
    // composition script that calls into the runtime.
    // Function replacements throughout: a literal one would read `$&` and
    // friends out of the URL.
    if (/<head[^>]*>/i.test(out)) return out.replace(/<head[^>]*>/i, (m) => `${m}\n    ${tag}`);
    if (/<body[^>]*>/i.test(out)) return out.replace(/<body[^>]*>/i, (m) => `${m}\n    ${tag}`);
    return `${tag}\n${out}`;
}

export async function vendorGsap(projectDir: string): Promise<void> {
    const dest = join(projectDir, GSAP_PROJECT_PATH);
    await mkdir(dirname(dest), { recursive: true });
    await copyFile(GSAP_VENDOR, dest);

    for (const rel of await listProjectFiles(projectDir)) {
        if (!/\.html?$/i.test(rel)) continue;
        const abs = join(projectDir, rel);
        const before = await readFile(abs, "utf8");
        const after = patchGsapTag(before, rel);
        if (after !== before) await writeFile(abs, after, "utf8");
    }
}

/**
 * Rewrite or inject the GSAP tag in one document.
 *
 * Exported for tests: the path arithmetic and the insertion fallbacks are the
 * parts that break quietly, and a render is far too slow a way to find out.
 */
export function patchGsapTag(html: string, relPath: string): string {
    // How many directories deep this document sits, so the vendored file can
    // be reached from it. "compositions/hook.html" → "../".
    const depth = relPath.split("/").length - 1;
    const src = "../".repeat(depth) + GSAP_PROJECT_PATH;

    const cdnTag = /<script[^>]+src=["']https?:\/\/[^"']*gsap[^"']*["'][^>]*>\s*<\/script>/gi;
    if (cdnTag.test(html)) {
        return html.replace(cdnTag, `<script src="${src}"></script>`);
    }

    // Already local, however it got there.
    if (/<script[^>]+src=["'][^"']*gsap[^"']*["']/i.test(html)) return html;

    // Only inject where GSAP is actually used. A static composition that never
    // mentions gsap does not need the 72 KB, and the check does not ask for it.
    if (!/\bgsap\s*\./.test(html) && !/\bgsap\s*\(/.test(html)) return html;

    const tag = `<script src="${src}"></script>`;

    // In order of preference, so the tag lands before the code that uses it.
    if (/<\/head>/i.test(html)) return html.replace(/<\/head>/i, `  ${tag}\n</head>`);
    if (/<body[^>]*>/i.test(html)) return html.replace(/<body[^>]*>/i, (m) => `${m}\n  ${tag}`);

    // ⚠️ A BARE <template> FRAGMENT IS LEFT ALONE, ON PURPOSE.
    //
    // A scene file is `<template>…</template>`, and the authoring contract
    // states that in a scene "styles and scripts outside it are DROPPED". So
    // a tag prepended here never runs. It does, however, make the CLI's own
    // `check` stop complaining — it scans the file's text and sees a gsap
    // script — and that is far worse than doing nothing:
    //
    //   before: `check` failed loudly, "Composition uses GSAP but no GSAP
    //           script is loaded", and nothing was rendered or charged.
    //   after:  `check` passed, `render` ran, the user was charged, and the
    //           film came out unstyled and static because the composition
    //           scripts still threw.
    //
    // Proved by rendering the same project three ways in the image: pristine
    // → render BLOCKS with sub_timeline_script_failure; prepended outside the
    // template → renders garbage; inside the template → renders the same
    // garbage, because a cloned template's scripts do not execute either.
    //
    // Getting GSAP into a composition's own context is a real fix and not
    // this function's to make. Until then the honest behaviour is to leave
    // the file untouched and let the gate refuse the project.
    return html;
}

export interface CheckFinding {
    severity: string;
    message: string;
    sourceFile?: string;
}

export interface CheckOutcome {
    ok: boolean;
    errors: CheckFinding[];
    warnings: CheckFinding[];
    /** A short, user-facing reason when ok is false. */
    problem?: string;
}

function collectFindings(node: unknown, out: CheckFinding[]): void {
    if (Array.isArray(node)) {
        for (const item of node) collectFindings(item, out);
        return;
    }
    if (node && typeof node === "object") {
        const obj = node as Record<string, unknown>;
        if (typeof obj.severity === "string" && typeof obj.message === "string") {
            out.push({
                severity: obj.severity,
                message: obj.message,
                sourceFile: typeof obj.sourceFile === "string" ? obj.sourceFile : undefined,
            });
        }
        for (const value of Object.values(obj)) collectFindings(value, out);
    }
}

/**
 * Run the pre-render gate.
 *
 * Findings are nested at different depths under lint / runtime / layout /
 * contrast / craft / audio, and the shape has changed between their releases,
 * so this walks the tree for anything carrying a severity and a message rather
 * than reading fixed paths. A gate that silently finds nothing because a key
 * moved is worse than no gate.
 */
export async function checkProject(projectDir: string): Promise<CheckOutcome> {
    const res = await runHyperframes(projectDir, ["check", "--json"], { timeoutMs: 5 * 60_000 });

    const parsed = await readCheckReport(projectDir, res.stdout);
    if (!parsed) {
        return {
            ok: false,
            errors: [],
            warnings: [],
            problem: "The design could not be checked before rendering.",
        };
    }

    const all: CheckFinding[] = [];
    collectFindings(parsed, all);
    const errors = all.filter((f) => f.severity === "error");
    const warnings = all.filter((f) => f.severity === "warning");
    const ok = (parsed as Record<string, unknown>).ok === true && errors.length === 0;

    return {
        ok,
        errors,
        warnings,
        problem: ok ? undefined : errors[0]?.message ?? "This design has a problem that would stop it rendering.",
    };
}

/**
 * Get the check report, preferring the file the CLI writes over its stdout.
 *
 * Slicing stdout between the first '{' and the last '}' looked fine until you
 * notice the CLI already prints `@hf-progress {…}` lines on the render path
 * and a trailing `report saved to …` line after check. One brace-bearing
 * notice ahead of the payload and the slice starts in the wrong place, the
 * parse throws, and the gate fails CLOSED for every design — with a message
 * blaming the design rather than the parser.
 *
 * Every run writes an authoritative report to .hyperframes/artifacts/checks,
 * so that is read first; stdout is only the fallback for a version that stops
 * writing one.
 */
async function readCheckReport(projectDir: string, stdout: string): Promise<unknown | null> {
    const dir = join(projectDir, ".hyperframes", "artifacts", "checks");
    try {
        const names = (await readdir(dir)).filter((n) => n.endsWith(".json"));
        if (names.length > 0) {
            const withTimes = await Promise.all(
                names.map(async (n) => ({ n, mtime: (await stat(join(dir, n))).mtimeMs }))
            );
            withTimes.sort((a, b) => b.mtime - a.mtime);
            return JSON.parse(await readFile(join(dir, withTimes[0].n), "utf8"));
        }
    } catch {
        /* fall through to stdout */
    }

    const start = stdout.indexOf("{");
    const end = stdout.lastIndexOf("}");
    if (start === -1 || end === -1 || end <= start) return null;
    try {
        return JSON.parse(stdout.slice(start, end + 1));
    } catch {
        return null;
    }
}

export interface HyperframesRenderResult {
    outputPath: string;
    /** Parsed from their `@hf-result` line; absent on older versions. */
    referenceBackground?: unknown;
    stdout: string;
}

/**
 * Render the project to an MP4.
 *
 * `--quality` is a real cost lever: `draft` while iterating, `delivery` for the
 * file a user publishes. The caller decides, because only it knows whether
 * anyone is waiting for this.
 */
export async function renderProject(
    projectDir: string,
    outputPath: string,
    opts: { quality?: "draft" | "looks" | "delivery"; timeoutMs?: number } = {}
): Promise<HyperframesRenderResult> {
    const quality = opts.quality ?? "delivery";
    const res = await runHyperframes(
        projectDir,
        ["render", "--quality", quality, "-o", outputPath],
        { timeoutMs: opts.timeoutMs ?? 20 * 60_000 }
    );

    if (res.code !== 0) {
        const detail = (res.stderr || res.stdout).trim().slice(-600);
        throw new Error(`hyperframes render failed (exit ${res.code}): ${detail}`);
    }

    const info = await stat(outputPath).catch(() => null);
    if (!info || info.size === 0) {
        // The CLI exited 0 but produced nothing usable. Treated as a failure
        // here rather than uploaded, because an empty MP4 reaching a content
        // document looks to the user exactly like a successful render.
        throw new Error("hyperframes render produced no output file");
    }

    let referenceBackground: unknown;
    const resultLine = res.stdout.split("\n").find((l) => l.startsWith("@hf-result "));
    if (resultLine) {
        try {
            referenceBackground = JSON.parse(resultLine.slice("@hf-result ".length)).reference_background;
        } catch {
            /* the line is advisory; a parse failure must not fail the render */
        }
    }

    return { outputPath, referenceBackground, stdout: res.stdout };
}

export interface SnapshotFrame {
    /** Absolute path of the PNG on disk. */
    path: string;
    /** Where in the film this frame is, in milliseconds. */
    timeMs: number;
    /** The CLI's own word for what this frame is, when it gives one. */
    role?: string;
}

/**
 * Still frames across the film, for a preview the user approves BEFORE paying
 * to render.
 *
 * ── Why stills and not a video ─────────────────────────────────────────────
 * The Hyperframes skill's own loop puts `snapshot` at step 3 and render at
 * step 5, and says of the snapshots: "that is how you see the video". Seeking
 * six frames costs a fraction of seeking three hundred, which is what makes it
 * honest to show this before charging anything.
 *
 * It is deliberately NOT their scrubbing player. That would be a truer
 * preview, and it is where this should end up, but it needs the project
 * serving its own runtime and a WebView on native that nothing here has
 * tested. Stills reuse the render path exactly — same download, same GSAP
 * vendoring, same `check` — so a frame the user approves was produced by the
 * same machinery that will render it.
 */
export async function snapshotProject(
    projectDir: string,
    outputDir: string,
    opts: { frames?: number; timeoutMs?: number } = {}
): Promise<SnapshotFrame[]> {
    const frames = Math.min(Math.max(opts.frames ?? 6, 1), 24);
    const res = await runHyperframes(
        projectDir,
        ["snapshot", "--frames", String(frames), "--output", outputDir, "--json"],
        { timeoutMs: opts.timeoutMs ?? 5 * 60_000 }
    );

    if (res.code !== 0) {
        const detail = (res.stderr || res.stdout).trim().slice(-600);
        throw new Error(`hyperframes snapshot failed (exit ${res.code}): ${detail}`);
    }

    const parsed = parseSnapshotJson(res.stdout);
    if (parsed.length === 0) {
        // Exited 0 with nothing to show. Treated as a failure rather than an
        // empty filmstrip, which the user would read as "my design is blank".
        throw new Error("hyperframes snapshot produced no frames");
    }
    // Resolve against the project dir, since the CLI reports paths relative to
    // where it ran.
    return parsed.map((f) => ({ ...f, path: isAbsolute(f.path) ? f.path : join(projectDir, f.path) }));
}

/**
 * Pull the frame list out of `snapshot --json`.
 *
 * Exported for tests. The CLI prints human lines around its JSON, so the
 * object is located rather than assumed to be the whole of stdout — the same
 * lesson `checkProject` learned when brace-slicing stdout went wrong.
 */
export function parseSnapshotJson(stdout: string): SnapshotFrame[] {
    const start = stdout.indexOf("{");
    const end = stdout.lastIndexOf("}");
    if (start < 0 || end <= start) return [];

    let doc: unknown;
    try {
        doc = JSON.parse(stdout.slice(start, end + 1));
    } catch {
        return [];
    }
    const list = (doc as { frames?: unknown }).frames;
    if (!Array.isArray(list)) return [];

    const out: SnapshotFrame[] = [];
    for (const item of list) {
        if (!item || typeof item !== "object") continue;
        const f = item as Record<string, unknown>;
        if (typeof f.path !== "string" || !f.path) continue;
        // `time` is seconds in their JSON. Milliseconds here, because every
        // other time on a design document is milliseconds and a mixed unit is
        // how a filmstrip ends up in the wrong order.
        const seconds = typeof f.time === "number" && Number.isFinite(f.time) ? f.time : 0;
        out.push({
            path: f.path,
            timeMs: Math.max(0, Math.round(seconds * 1000)),
            role: typeof f.role === "string" ? f.role : undefined,
        });
    }
    // Document order is not guaranteed to be time order, and a filmstrip is
    // only legible in time order.
    return out.sort((a, b) => a.timeMs - b.timeMs);
}

/**
 * Every file in a materialised project, relative to its root. Used to verify a
 * download landed before the CLI is pointed at it — a half-materialised
 * project renders as a film with missing scenes rather than as an error.
 */
export async function listProjectFiles(projectDir: string): Promise<string[]> {
    const out: string[] = [];
    async function walk(dir: string, prefix: string): Promise<void> {
        const entries = await readdir(dir, { withFileTypes: true });
        for (const entry of entries) {
            if (entry.name === ".hyperframes" || entry.name === "node_modules") continue;
            const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
            if (entry.isDirectory()) await walk(join(dir, entry.name), rel);
            else out.push(rel);
        }
    }
    await walk(projectDir, "");
    return out.sort();
}
