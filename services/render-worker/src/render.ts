/**
 * One render job, start to finish: read the revision, drive the browser, upload
 * the output, write the result back.
 *
 * Both entrypoints (the image Lambda and the video Batch task) call this, so
 * there is one definition of what rendering means — the same mistake the old
 * code avoided by sharing `useDesignRender` between the Studio and the Media
 * Stage, kept here.
 *
 * ── Every video goes through the composition path ────────────────────────────
 * A "video" job renders a design, exactly as it always has, but it reaches the
 * renderer as a ONE-CLIP COMPOSITION derived from that design in memory. The
 * timeline is the only primitive; a design is a clip source.
 *
 * That is deliberately not two paths kept alive side by side. A one-clip
 * composition delegates back to renderVideo unchanged, so the bytes of every
 * video the product has ever made are unaffected, while nothing has to know
 * whether a content has a timeline yet. Nothing is migrated and no document is
 * written for it.
 */
import { closeBrowser, getBrowser } from "./browser";
import { config } from "./config";
import { renderComposition } from "./composition";
import { extractFrame } from "./frame";
import { ingestVideo } from "./ingest";
import { captureSlides } from "./image";
import { loadDesign } from "./page";
import { upload } from "./s3";
import {
    failIngest,
    getComposition,
    getContentAudio,
    getRevision,
    getStudioForRender,
    markFailed,
    markStarted,
    markStudioRenderFailed,
    markStudioRenderStarted,
    recordFrameAsset,
    setIngest,
    setProgress,
    setResult,
    setStudioRenderResult,
} from "./firestore";
import { renderFailed, renderSucceeded } from "./metrics";
import { makeThumbnail } from "./thumbnail";
import type { RenderJob, RenderTarget } from "./types";
import { compositionFromDesign, measureComposition } from "../../../shared-libs/design/composition-runtime";
import { parseSceneHtmlRef, studioToComposition } from "../../../shared-libs/design/studio-runtime";

/**
 * Turn a thrown error into something worth showing a brand manager. Anything
 * unrecognised stays generic — a stack trace in the UI helps nobody, and the
 * real detail is already in CloudWatch.
 */
function userFacing(err: unknown): string {
    const msg = err instanceof Error ? err.message : String(err);
    if (/could not fetch (music|voiceover)/i.test(msg)) {
        return "We couldn't load the soundtrack for this video. Check the audio, then try again.";
    }
    if (/could not fetch (image|clip)_/i.test(msg)) {
        return "We couldn't load one of the clips in this timeline. Check its media, then try again.";
    }
    // The composition validator's messages are written FOR the user — "This
    // generated clip hasn't finished yet", "A footage clip needs a url" — so
    // they are passed through rather than flattened into the generic line.
    if (/^(A |An |This |The |Overlay |Placed |Frame rate|Speed|Duplicate|Unknown|Transparent)/.test(msg)) {
        return msg;
    }
    if (/ffmpeg/i.test(msg)) {
        return "Something went wrong while encoding the video. Please try again.";
    }
    if (/timeout|Timeout/.test(msg)) {
        return "This design took too long to render. Try simplifying it, or try again.";
    }
    return "Something went wrong while rendering this design. Please try again.";
}

/**
 * Enforce config.jobTimeoutMs — the "hard ceiling on a single job" that was
 * declared and then never applied to anything, which is how a render sat at 69%
 * indefinitely instead of failing.
 *
 * A rejected promise alone would not be enough: the work behind it keeps
 * running, and on a warm Lambda the next invocation would inherit a wedged
 * Chromium. So the browser is torn down on the way out, which also unblocks
 * whatever CDP call was hanging and lets the real error surface.
 */
async function withJobTimeout<T>(work: Promise<T>, revisionId: string): Promise<T> {
    let timer: NodeJS.Timeout | undefined;
    const ceiling = new Promise<never>((_, reject) => {
        timer = setTimeout(() => {
            console.error(`[render] ${revisionId} hit the ${config.jobTimeoutMs}ms job ceiling — tearing down`);
            void closeBrowser();
            reject(new Error(`render timeout after ${config.jobTimeoutMs}ms`));
        }, config.jobTimeoutMs);
        timer.unref?.();
    });
    try {
        return await Promise.race([work, ceiling]);
    } finally {
        if (timer) clearTimeout(timer);
    }
}

/** Which document this job reports into. */
function jobTarget(job: RenderJob): RenderTarget {
    return {
        brandId: job.brandId,
        contentId: job.contentId,
        revisionId: job.revisionId,
        kind: job.docType === "composition" ? "composition" : "design",
    };
}

export async function runJob(job: RenderJob): Promise<void> {
    try {
        await withJobTimeout(runJobInner(job), job.revisionId);
    } catch (err) {
        // runJobInner marks its own failures, but the ceiling fires OUTSIDE it,
        // so without this the revision would sit in "rendering" until the
        // reconcile cron swept it 45 minutes later.
        if (err instanceof Error && err.message.startsWith("render timeout")) {
            await markFailed(jobTarget(job), userFacing(err)).catch(() => undefined);
        }
        throw err;
    }
}

async function runJobInner(job: RenderJob): Promise<void> {
    const target = jobTarget(job);
    const startedAt = Date.now();

    try {
        if (job.docType === "ingest") {
            await runIngestJob(job);
            return;
        }
        if (job.docType === "frame") {
            await runFrameJob(job);
            return;
        }
        if (job.docType === "studio") {
            await runStudioJob(job, startedAt);
            return;
        }
        if (job.docType === "composition") {
            await runCompositionJob(job, target, startedAt);
            return;
        }
        await runDesignJob(job, target, startedAt);
    } catch (err) {
        console.error(`[render] ${job.revisionId} failed:`, err);
        renderFailed({
            docType: job.docType,
            durationMs: Date.now() - startedAt,
            revisionId: job.revisionId,
            reason: err instanceof Error ? err.message : String(err),
        });
        await markFailed(target, userFacing(err)).catch(() => undefined);
        throw err;
    }
}

/** An authored timeline. */
async function runCompositionJob(
    job: RenderJob,
    target: RenderTarget,
    startedAt: number
): Promise<void> {
    const { brandId, contentId, revisionId } = job;

    const stored = await getComposition(brandId, contentId, revisionId);
    if (!stored) throw new Error(`composition ${revisionId} not found`);

    // A duplicate queue delivery must not re-render (or re-charge). Only an
    // explicit force re-renders something already done.
    if (stored.renderUrl && !job.force) {
        console.log(`[render] ${revisionId} already rendered, skipping`);
        return;
    }

    await markStarted(target, job.jobId);
    const audio = await getContentAudio(brandId, contentId);
    const browser = await getBrowser();

    const { mp4, poster } = await renderComposition({
        browser,
        composition: stored.composition,
        loadRevision: async (designRevisionId) => {
            const revision = await getRevision(brandId, contentId, designRevisionId);
            if (!revision) throw new Error(`design revision ${designRevisionId} not found`);
            if (!revision.html) throw new Error(`design revision ${designRevisionId} has no html`);
            return { html: revision.html, width: revision.width, height: revision.height };
        },
        audio,
        onProgress: (f) => void setProgress(target, f).catch(() => undefined),
    });

    const videoUrl = await upload(mp4, "mp4", "video/mp4");
    const posterUrl = poster.length ? await upload(poster, "png", "image/png") : undefined;

    await setResult(target, [videoUrl], "video", posterUrl, config.scale);
    renderSucceeded({
        docType: "composition",
        durationMs: Date.now() - startedAt,
        outputs: 1,
        bytes: mp4.length,
        revisionId,
    });
}

/** A design revision: one MP4 for a video, one PNG per slide for an image. */
async function runDesignJob(job: RenderJob, target: RenderTarget, startedAt: number): Promise<void> {
    const { brandId, contentId, revisionId } = job;

    const revision = await getRevision(brandId, contentId, revisionId);
    if (!revision) throw new Error(`revision ${revisionId} not found`);
    if (!revision.html) throw new Error(`revision ${revisionId} has no html`);

    if (revision.renderUrl && !job.force) {
        console.log(`[render] ${revisionId} already rendered, skipping`);
        return;
    }

    await markStarted(target, job.jobId);

    if (job.docType === "video") {
        const audio = await getContentAudio(brandId, contentId);
        const browser = await getBrowser();

        // The implicit composition. Its durationMs only has to be positive for
        // validation — the passthrough reads the design's own measurement for
        // the real length, exactly as the pre-composition path did.
        const composition = compositionFromDesign({
            revisionId,
            width: revision.width,
            height: revision.height,
            fps: config.fps,
            durationMs: revision.durationMs ?? 0,
        });

        const { mp4, poster } = await renderComposition({
            browser,
            composition,
            loadRevision: async () => ({
                html: revision.html,
                width: revision.width,
                height: revision.height,
            }),
            audio,
            onProgress: (f) => void setProgress(target, f).catch(() => undefined),
        });

        const videoUrl = await upload(mp4, "mp4", "video/mp4");
        const posterUrl = poster.length ? await upload(poster, "png", "image/png") : undefined;

        await setResult(target, [videoUrl], "video", posterUrl, config.scale);
        renderSucceeded({
            docType: "video",
            durationMs: Date.now() - startedAt,
            outputs: 1,
            bytes: mp4.length,
            revisionId,
        });
        return;
    }

    const browser = await getBrowser();
    const loaded = await loadDesign(browser, revision.html, revision.width, revision.height);
    try {
        const slides = await captureSlides(loaded.page, (done, total) =>
            void setProgress(target, done / total).catch(() => undefined)
        );
        if (!slides.length) throw new Error("the design produced no slides");

        const urls: string[] = [];
        const thumbUrls: (string | undefined)[] = [];
        for (const png of slides) {
            urls.push(await upload(png, "png", "image/png"));
            const thumb = await makeThumbnail(png);
            thumbUrls.push(thumb ? await upload(thumb, "webp", "image/webp") : undefined);
        }

        await setResult(target, urls, "image", undefined, config.scale, thumbUrls);
        renderSucceeded({
            docType: "image",
            durationMs: Date.now() - startedAt,
            outputs: urls.length,
            bytes: slides.reduce((n, b) => n + b.length, 0),
            revisionId,
        });
    } finally {
        await loaded.close();
    }
}

/**
 * Ingest one uploaded video: measure it, pull a poster, sample frames.
 *
 * Reports its own failure onto the asset rather than only throwing. Footage is
 * usable without ingest — it can still be trimmed and placed by hand — so a
 * failed ingest must degrade to "we could not read this one" rather than
 * looking like a lost upload.
 */
/**
 * Render a Design Studio.
 *
 * The studio is CONVERTED into a Composition and handed to the existing
 * renderer. `studioToComposition` is shared with the app, so the timeline the
 * user scrubbed and the one that encodes are the same arithmetic — including
 * how much time a transition takes off its two neighbours.
 *
 * ⚠️ A non-empty `problems` list REFUSES the render rather than rendering what
 * it can. A layered scene cannot export yet (a composition clip is one
 * full-frame source with no concept of a slot), and rendering only the base
 * layer would publish something nobody approved. The backend checks the same
 * conditions before charging; this is the backstop for a studio edited between
 * the request and the job starting.
 */
async function runStudioJob(job: RenderJob, startedAt: number): Promise<void> {
    const { brandId, contentId } = job;
    const studioId = job.studioId ?? job.revisionId;
    if (!studioId) throw new Error("a studio job needs a studio id");

    const loaded = await getStudioForRender(brandId, contentId, studioId);
    if (!loaded) throw new Error(`studio ${studioId} not found`);

    // A duplicate queue delivery must not re-render, or re-charge.
    if (loaded.studio.renderUrl && !job.force) {
        console.log(`[render] studio ${studioId} already rendered, skipping`);
        return;
    }

    const { composition, audio: studioAudio, problems, notes } = studioToComposition(
        loaded.studio as never,
        loaded.scenes as never,
        loaded.media
    );
    if (problems.length) {
        // The user's own words back, not a stack trace: the first problem is
        // the one to fix, and listing all of them turns a fixable state into a
        // wall of text.
        await markStudioRenderFailed(brandId, contentId, studioId, problems[0]);
        throw new Error(`studio ${studioId} is not renderable: ${problems.join("; ")}`);
    }
    if (notes.length) console.log(`[render] studio ${studioId}: ${notes.join(" ")}`);

    await markStudioRenderStarted(brandId, contentId, studioId, job.jobId);
    const browser = await getBrowser();

    const { mp4, poster } = await renderComposition({
        browser,
        composition,
        // A studio scene carries its HTML on the layer rather than in a
        // `designs` revision, so the synthetic id is resolved from the scenes
        // already in hand. An ordinary revision id still falls through to
        // Firestore, which is what lets a studio mix in an older design.
        loadRevision: async (ref) => {
            const parsed = parseSceneHtmlRef(ref);
            if (parsed) {
                const scene = loaded.scenes[parsed.sceneId] as
                    | { layers?: { kind?: string; html?: string }[] }
                    | undefined;
                const html = scene?.layers?.find((l) => l.kind === "html")?.html;
                if (!html) throw new Error(`scene ${parsed.sceneId} has no html`);
                return { html, width: loaded.studio.width, height: loaded.studio.height };
            }
            const revision = await getRevision(brandId, contentId, ref);
            if (!revision?.html) throw new Error(`design revision ${ref} has no html`);
            return { html: revision.html, width: revision.width, height: revision.height };
        },
        // The studio's own sound wins outright. The content-level soundtrack
        // is only consulted when the studio declares none at all — it belongs
        // to the OLD design stage, which has its own audio panel, and a studio
        // that has been given a bed must not also inherit one the user set
        // somewhere else. (That fallback goes away with the old stage.)
        audio: loaded.hasOwnAudio ? studioAudio : await getContentAudio(brandId, contentId),
    });

    const videoUrl = await upload(mp4, "mp4", "video/mp4");
    const posterUrl = poster.length ? await upload(poster, "png", "image/png") : undefined;

    const measured = measureComposition(composition);
    await setStudioRenderResult(
        brandId,
        contentId,
        studioId,
        videoUrl,
        posterUrl,
        measured.clips.length,
        measured.totalMs
    );
    renderSucceeded({
        docType: "studio",
        durationMs: Date.now() - startedAt,
        outputs: 1,
        bytes: mp4.length,
        revisionId: studioId,
    });
}

/**
 * Cut one frame out of a video and file it as media.
 *
 * No render target and no status document: a frame job is not a render of
 * anything, it produces a new asset. Which is why a failure here only logs and
 * rethrows — there is no half-finished document to mark, and Batch's retry is
 * the right handler for a transient download.
 */
async function runFrameJob(job: RenderJob): Promise<void> {
    const sourceMediaId = job.revisionId;
    const url = job.sourceUrl ?? "";
    if (!sourceMediaId || !url) throw new Error("a frame job needs a media id and a url");

    const frame = await extractFrame(url, job.atMs ?? -1);
    const uploaded = await upload(frame.bytes, "png", "image/png");
    const mediaId = await recordFrameAsset(job.brandId, {
        url: uploaded,
        sizeBytes: frame.bytes.length,
        width: frame.width,
        height: frame.height,
        sourceMediaId,
        atMs: frame.atMs,
        sourceDurationMs: frame.sourceDurationMs,
    });
    console.log(
        `[frame] ${sourceMediaId} @${frame.atMs}ms of ${frame.sourceDurationMs}ms -> ${mediaId}`
    );
}

async function runIngestJob(job: RenderJob): Promise<void> {
    const assetId = job.revisionId;
    const url = job.sourceUrl ?? "";
    if (!assetId || !url) throw new Error("an ingest job needs an asset id and a url");

    try {
        const out = await ingestVideo(url, job.brandId);
        await setIngest(job.brandId, assetId, out);
        console.log(
            `[ingest] ${assetId}: ${out.durationMs}ms ${out.width}x${out.height} ` +
                `@${out.fps}fps audio=${out.hasAudio} frames=${out.sampleFrameUrls.length}`
        );
    } catch (err) {
        const reason = err instanceof Error ? err.message : String(err);
        console.error(`[ingest] ${assetId} failed:`, reason);
        await failIngest(job.brandId, assetId, reason).catch(() => undefined);
        throw err;
    }
}
