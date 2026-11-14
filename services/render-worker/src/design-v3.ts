/**
 * Render one Design Studio v3 design.
 *
 * The shape of the job, end to end:
 *
 *   read the design  →  materialise its project from S3  →  vendor GSAP
 *   →  check (the gate)  →  render  →  upload  →  write the result back
 *
 * ── Why the gate runs here and not only in the API ─────────────────────────
 * The API refuses an obviously unrenderable design before charging, the same
 * way StudioRenderable does today. But only the worker has the project files
 * in front of it, so only the worker can run Hyperframes' own `check` — the
 * one that knows about safe areas, contrast, layout and determinism. A design
 * that fails here never reaches the encoder, and the reason it failed is a
 * sentence from their `Fix:` line rather than a stack trace.
 *
 * ── Why progress is written to the document ────────────────────────────────
 * Not returned to a caller: nobody is waiting on this process. The app reads
 * progress off the design document, which is what lets a render survive a
 * page reload, a locked phone, or the user walking away.
 */
import { mkdtemp, rm, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
    checkProject,
    listProjectFiles,
    renderProject,
    snapshotProject,
    stripPlayerRuntime,
    vendorGsap,
} from "./hyperframes";
import { resolveProjectMedia } from "./media";
import { downloadPrefix, upload, uploadProjectFiles } from "./s3";

export interface DesignV3Job {
    brandId: string;
    designId: string;
    /** Where the project directory lives in the render bucket. */
    projectPrefix: string;
    /**
     * `draft` while someone iterates, `delivery` for the file they publish.
     * A real cost lever, so the caller chooses rather than this defaulting to
     * the expensive one.
     */
    quality?: "draft" | "looks" | "delivery";
    force?: boolean;
    /**
     * What this design may spend resolving media, in HeyGen credits.
     *
     * Written into the project as Hyperframes' own `.hyperframes/
     * credit-limit.json`, so the CLI refuses a call that would exceed it
     * rather than us noticing afterwards.
     */
    creditCeiling?: number;
}

export interface DesignV3Hooks {
    onProgress?: (fraction: number) => Promise<void>;
    onCheckFailed?: (problem: string) => Promise<void>;
}

export interface DesignV3Result {
    url: string;
    posterUrl?: string;
    /** Encoded size, reported so the cost meter has a real number. */
    bytes: number;
    /** What `check` said, kept so a warning can be surfaced without blocking. */
    warnings: string[];
}

/**
 * The smallest project that can possibly render. Anything under this arrived
 * broken, and rendering it would produce a film with missing scenes rather
 * than an error — the failure mode that sends a debugger to the wrong place.
 */
const MIN_PROJECT_FILES = 1;

/**
 * Everything both the render and the preview need before they diverge:
 * the project on disk, GSAP vendored into it, and `check` passed.
 *
 * Shared deliberately rather than copied. A preview the user approves has to
 * have come through the SAME gate as the render that follows it — if the
 * preview skipped `check`, it could show a film that then refuses to render,
 * which is worse than no preview at all.
 */
async function materialiseAndGate(
    job: DesignV3Job,
    workDir: string,
    hooks: DesignV3Hooks
): Promise<string[]> {
    const warnings: string[] = [];
    const fileCount = await downloadPrefix(job.projectPrefix, workDir);
    if (fileCount < MIN_PROJECT_FILES) {
        throw new Error(`no project files found under ${job.projectPrefix}`);
    }

    const files = await listProjectFiles(workDir);
    if (!files.includes("index.html")) {
        // Hyperframes renders the project directory, whose root composition
        // is index.html. Without it the CLI has nothing to enter through,
        // and its own error is considerably less clear than this one.
        throw new Error(`project at ${job.projectPrefix} has no index.html (found: ${files.slice(0, 5).join(", ")})`);
    }

    // Before the gate, not after: a project carrying a CDN script tag
    // would otherwise pass `check` on a machine with network and fail on
    // one without, which is the worst kind of intermittent.
    await vendorGsap(workDir);

    // The player's runtime tag points at CloudFront, and a render must work
    // with no network. The CLI injects its own runtime, so taking ours out
    // costs the render nothing — frames are byte-identical either way.
    {
        const indexPath = join(workDir, "index.html");
        const before = await readFile(indexPath, "utf8");
        const after = stripPlayerRuntime(before);
        if (after !== before) await writeFile(indexPath, after, "utf8");
    }

    // Media the film declared but does not have yet. Before the gate for
    // the same reason: an <img> with no src is a finding, and a film whose
    // picture arrives only at render time is one the preview lied about.
    //
    // Non-fatal by contract. A brief that mentions a song it cannot find
    // must still be previewable, so a failure here is reported as a warning
    // and the film goes on without that asset.
    try {
        const media = await resolveProjectMedia(workDir, {
            heygenApiKey: process.env.HEYGEN_API_KEY,
            creditCeiling: job.creditCeiling,
        });
        for (const p of media.problems) warnings.push(`media: ${p}`);
        if (media.changed) {
            // Back to the bucket, so the render that follows this preview
            // reuses the same files rather than resolving — and paying —
            // for them a second time.
            const n = await uploadProjectFiles(job.projectPrefix, workDir, ["index.html", ...media.files]);
            console.log(`[media] uploaded ${n} file(s) back to ${job.projectPrefix}`);
        }
    } catch (err) {
        // Logged as well as warned. The first version only warned, and a
        // denied upload left the preview looking perfect while the stored
        // project kept pointing at media that was never there.
        console.log(`[media] FAILED: ${(err as Error).message}`);
        warnings.push(`media: ${(err as Error).message}`);
    }

    const outcome = await checkProject(workDir);
    if (!outcome.ok) {
        const problem = outcome.problem ?? "This design has a problem that would stop it rendering.";
        await hooks.onCheckFailed?.(problem);
        throw new Error(`check failed: ${problem}`);
    }
    return [...warnings, ...outcome.warnings.map((w) => w.message)];
}

export async function renderDesignV3(
    job: DesignV3Job,
    hooks: DesignV3Hooks = {}
): Promise<DesignV3Result> {
    if (!job.brandId || !job.designId) throw new Error("renderDesignV3: brandId and designId are required");
    if (!job.projectPrefix) throw new Error("renderDesignV3: the design has no project to render");

    const workDir = await mkdtemp(join(tmpdir(), `hf-${job.designId}-`));
    try {
        const warnings = await materialiseAndGate(job, workDir, hooks);

        await hooks.onProgress?.(0.1);

        const outputPath = join(workDir, "out.mp4");
        await renderProject(workDir, outputPath, { quality: job.quality ?? "delivery" });

        await hooks.onProgress?.(0.9);

        const mp4 = await readFile(outputPath);
        const url = await upload(mp4, "mp4", "video/mp4");

        return { url, bytes: mp4.byteLength, warnings };
    } finally {
        // Batch tasks exit anyway, but a warm Lambda keeps /tmp between
        // invocations and a project with media in it is not small.
        await rm(workDir, { recursive: true, force: true }).catch(() => undefined);
    }
}

export interface DesignV3PreviewFrame {
    url: string;
    timeMs: number;
}

export interface DesignV3PreviewResult {
    frames: DesignV3PreviewFrame[];
    warnings: string[];
}

/**
 * Still frames across the film, so the user can see it before paying to
 * render it.
 *
 * This is the whole point of the phase it belongs to: the Hyperframes loop
 * puts snapshots before the render and says "passing checks alone is not a
 * reason to render". Without it the first time anyone sees their design is
 * after the tokens are gone.
 */
export async function previewDesignV3(
    job: DesignV3Job & { frames?: number },
    hooks: DesignV3Hooks = {}
): Promise<DesignV3PreviewResult> {
    if (!job.brandId || !job.designId) throw new Error("previewDesignV3: brandId and designId are required");
    if (!job.projectPrefix) throw new Error("previewDesignV3: the design has nothing to preview");

    const workDir = await mkdtemp(join(tmpdir(), `hfp-${job.designId}-`));
    try {
        const warnings = await materialiseAndGate(job, workDir, hooks);

        await hooks.onProgress?.(0.2);

        const shots = await snapshotProject(workDir, join(workDir, ".preview"), {
            frames: job.frames ?? 6,
        });

        // Uploaded one at a time, in order. A filmstrip is read left to right,
        // and Promise.all would return them in whatever order they finished.
        const frames: DesignV3PreviewFrame[] = [];
        for (const shot of shots) {
            const png = await readFile(shot.path);
            frames.push({ url: await upload(png, "png", "image/png"), timeMs: shot.timeMs });
            await hooks.onProgress?.(0.2 + (0.8 * frames.length) / shots.length);
        }

        return { frames, warnings };
    } finally {
        await rm(workDir, { recursive: true, force: true }).catch(() => undefined);
    }
}
