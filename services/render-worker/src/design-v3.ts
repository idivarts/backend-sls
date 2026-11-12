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
import { mkdtemp, rm, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { checkProject, listProjectFiles, renderProject, vendorGsap } from "./hyperframes";
import { downloadPrefix, upload } from "./s3";

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

export async function renderDesignV3(
    job: DesignV3Job,
    hooks: DesignV3Hooks = {}
): Promise<DesignV3Result> {
    if (!job.brandId || !job.designId) throw new Error("renderDesignV3: brandId and designId are required");
    if (!job.projectPrefix) throw new Error("renderDesignV3: the design has no project to render");

    const workDir = await mkdtemp(join(tmpdir(), `hf-${job.designId}-`));
    try {
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

        const outcome = await checkProject(workDir);
        if (!outcome.ok) {
            const problem = outcome.problem ?? "This design has a problem that would stop it rendering.";
            await hooks.onCheckFailed?.(problem);
            throw new Error(`check failed: ${problem}`);
        }

        await hooks.onProgress?.(0.1);

        const outputPath = join(workDir, "out.mp4");
        await renderProject(workDir, outputPath, { quality: job.quality ?? "delivery" });

        await hooks.onProgress?.(0.9);

        const mp4 = await readFile(outputPath);
        const url = await upload(mp4, "mp4", "video/mp4");

        return {
            url,
            bytes: mp4.byteLength,
            warnings: outcome.warnings.map((w) => w.message),
        };
    } finally {
        // Batch tasks exit anyway, but a warm Lambda keeps /tmp between
        // invocations and a project with media in it is not small.
        await rm(workDir, { recursive: true, force: true }).catch(() => undefined);
    }
}
