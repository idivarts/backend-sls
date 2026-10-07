/**
 * One render job, start to finish: read the revision, drive the browser, upload
 * the output, write the result back.
 *
 * Both entrypoints (the image Lambda and the video Batch task) call this, so
 * there is one definition of what rendering means — the same mistake the old
 * code avoided by sharing `useDesignRender` between the Studio and the Media
 * Stage, kept here.
 */
import { getBrowser } from "./browser";
import { config } from "./config";
import { captureSlides } from "./image";
import { loadDesign } from "./page";
import { upload } from "./s3";
import {
    getContentAudio,
    getRevision,
    markFailed,
    markStarted,
    setProgress,
    setResult,
} from "./firestore";
import { renderFailed, renderSucceeded } from "./metrics";
import { makeThumbnail } from "./thumbnail";
import { renderVideo } from "./video";
import type { RenderJob } from "./types";

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
    if (/ffmpeg/i.test(msg)) {
        return "Something went wrong while encoding the video. Please try again.";
    }
    if (/timeout|Timeout/.test(msg)) {
        return "This design took too long to render. Try simplifying it, or try again.";
    }
    return "Something went wrong while rendering this design. Please try again.";
}

export async function runJob(job: RenderJob): Promise<void> {
    const { brandId, contentId, revisionId } = job;
    const startedAt = Date.now();

    const revision = await getRevision(brandId, contentId, revisionId);
    if (!revision) throw new Error(`revision ${revisionId} not found`);
    if (!revision.html) throw new Error(`revision ${revisionId} has no html`);

    // A duplicate queue delivery must not re-render (or re-charge). Only an
    // explicit force re-renders something already done.
    if (revision.renderUrl && !job.force) {
        console.log(`[render] ${revisionId} already rendered, skipping`);
        return;
    }

    await markStarted(brandId, contentId, revisionId, job.jobId);

    try {
        const browser = await getBrowser();
        const loaded = await loadDesign(browser, revision.html, revision.width, revision.height);

        try {
            if (job.docType === "video") {
                const audio = await getContentAudio(brandId, contentId);
                const { mp4, poster } = await renderVideo({
                    page: loaded.page,
                    offsets: loaded.measurement.offsets,
                    totalMs: loaded.measurement.total,
                    rect: loaded.rects[0],
                    audio,
                    onProgress: (f) =>
                        void setProgress(brandId, contentId, revisionId, f).catch(() => undefined),
                });

                const videoUrl = await upload(mp4, "mp4", "video/mp4");
                const posterUrl = poster.length
                    ? await upload(poster, "png", "image/png")
                    : undefined;

                await setResult(
                    brandId,
                    contentId,
                    revisionId,
                    [videoUrl],
                    "video",
                    posterUrl,
                    config.scale
                );
                renderSucceeded({
                    docType: "video",
                    durationMs: Date.now() - startedAt,
                    outputs: 1,
                    bytes: mp4.length,
                    revisionId,
                });
                return;
            }

            const slides = await captureSlides(loaded.page, (done, total) =>
                void setProgress(brandId, contentId, revisionId, done / total).catch(
                    () => undefined
                )
            );
            if (!slides.length) throw new Error("the design produced no slides");

            const urls: string[] = [];
            const thumbUrls: (string | undefined)[] = [];
            for (const png of slides) {
                urls.push(await upload(png, "png", "image/png"));
                const thumb = await makeThumbnail(png);
                thumbUrls.push(thumb ? await upload(thumb, "webp", "image/webp") : undefined);
            }

            await setResult(
                brandId,
                contentId,
                revisionId,
                urls,
                "image",
                undefined,
                config.scale,
                thumbUrls
            );
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
    } catch (err) {
        console.error(`[render] ${revisionId} failed:`, err);
        renderFailed({
            docType: job.docType,
            durationMs: Date.now() - startedAt,
            revisionId,
            reason: err instanceof Error ? err.message : String(err),
        });
        await markFailed(brandId, contentId, revisionId, userFacing(err)).catch(() => undefined);
        throw err;
    }
}
