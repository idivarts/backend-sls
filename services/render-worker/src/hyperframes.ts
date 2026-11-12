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
import { copyFile, mkdir, readFile, readdir, stat, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";

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
 * The environment every invocation runs with.
 *
 * `HYPERFRAMES_NO_TELEMETRY` and `DO_NOT_TRACK` are not optional: this worker
 * renders other people's brand material, and nothing about it should leave the
 * container. No provider keys are passed here either — media generation is a
 * separate, metered step, and a key absent from this environment is a
 * capability that cannot be billed by accident.
 */
export function hyperframesEnv(opts: HyperframesEnvOptions = {}): NodeJS.ProcessEnv {
    return {
        ...process.env,
        HYPERFRAMES_NO_TELEMETRY: "1",
        DO_NOT_TRACK: "1",
        HYPERFRAMES_FFMPEG_PATH: opts.ffmpegPath ?? process.env.HYPERFRAMES_FFMPEG_PATH ?? "/usr/bin/ffmpeg",
        HYPERFRAMES_FFPROBE_PATH: opts.ffprobePath ?? process.env.HYPERFRAMES_FFPROBE_PATH ?? "/usr/bin/ffprobe",
    };
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
        const child = spawn("npx", ["--no-install", "hyperframes", ...args], {
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
 * Put the vendored GSAP into the project and make index.html load it locally.
 *
 * Rewriting any CDN tag rather than only appending one, because a project that
 * came from `hyperframes init` or from a template will carry the jsdelivr tag,
 * and leaving it in place would reintroduce the network dependency we just
 * removed — silently, since it would still render on a machine with network.
 */
export async function vendorGsap(projectDir: string): Promise<void> {
    const dest = join(projectDir, GSAP_PROJECT_PATH);
    await mkdir(dirname(dest), { recursive: true });
    await copyFile(GSAP_VENDOR, dest);

    const indexPath = join(projectDir, "index.html");
    let html = await readFile(indexPath, "utf8");

    const cdnTag = /<script[^>]+src=["']https?:\/\/[^"']*gsap[^"']*["'][^>]*>\s*<\/script>/gi;
    if (cdnTag.test(html)) {
        html = html.replace(cdnTag, `<script src="${GSAP_PROJECT_PATH}"></script>`);
    } else if (!html.includes(GSAP_PROJECT_PATH)) {
        // No GSAP tag at all: a project whose check would fail with "uses GSAP
        // but no GSAP script is loaded". Insert it rather than let the render
        // produce a film where nothing moves.
        html = html.replace(/<\/head>/i, `  <script src="${GSAP_PROJECT_PATH}"></script>\n  </head>`);
    }
    await writeFile(indexPath, html, "utf8");
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

    const start = res.stdout.indexOf("{");
    const end = res.stdout.lastIndexOf("}");
    if (start === -1 || end === -1) {
        return {
            ok: false,
            errors: [],
            warnings: [],
            problem: "The design could not be checked before rendering.",
        };
    }

    let parsed: unknown;
    try {
        parsed = JSON.parse(res.stdout.slice(start, end + 1));
    } catch {
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
