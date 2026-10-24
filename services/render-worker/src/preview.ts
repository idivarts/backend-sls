/**
 * Render a design to a handful of small still frames, for the AI to LOOK at.
 *
 * This is not the publish path and must not be confused with it. `render.ts`
 * produces the artifact a brand ships, writes it to S3 and reports progress to
 * Firestore; this produces throwaway JPEGs that go straight back in the Lambda
 * response so the model that just authored the HTML can see what it actually
 * made before anyone else does.
 *
 * Everything that makes the publish path expensive is deliberately absent: no
 * S3, no Firestore writes, no FFmpeg, no MP4. The whole point is to be cheap
 * enough to run two or three times inside one chat turn.
 *
 * Why JPEG and why small: the frames ride back in a synchronous Lambda response,
 * which is capped at 6 MB, and then into a model's context, which is billed by
 * the pixel. Inspection only needs to show overflow, collision, clipping and
 * whether a scene's motion has landed by time T — none of which needs a
 * publication-grade PNG.
 */
import type { Browser, Page } from "playwright";
import { seekAt } from "../../../shared-libs/design/design-runtime";
import { getRevision } from "./firestore";
import { loadDesign } from "./page";

/** One frame to capture: a scene index, and how far into that scene. */
export interface PreviewFrame {
    slide: number;
    ms: number;
}

export interface PreviewRequest {
    brandId: string;
    contentId: string;
    revisionId: string;
    /** Explicit frames. Omitted → a default sample (see defaultFrames). */
    frames?: PreviewFrame[];
    /** Longest edge of a returned frame, in px. Clamped to 320-900. */
    maxEdge?: number;
}

export interface PreviewResponse {
    ok: boolean;
    error?: string;
    docType?: "image" | "video";
    slideCount?: number;
    width?: number;
    height?: number;
    /** data:image/jpeg;base64,… — one per requested frame, in order. */
    images?: string[];
    /** The frames actually captured, so the caller can label them. */
    captured?: PreviewFrame[];
}

/**
 * A hard cap on how much we will render and ship back. Eight frames at ~120 KB
 * leaves plenty of head-room under Lambda's 6 MB response limit, and a model
 * that cannot find the problem in eight frames will not find it in twenty.
 */
const MAX_FRAMES = 8;

/**
 * What to look at when the caller does not say.
 *
 * For a still design, every slide once — there is nothing else to see. For a
 * video, three points INSIDE each scene rather than at its edges: 15% catches
 * "the entrance has not arrived yet", 55% is the scene at rest, and 90% catches
 * motion that finished early or never settled. Sampling at 0 and at the scene
 * boundary is what makes animation bugs invisible, since both ends are the
 * frames most likely to look fine.
 */
export function defaultFrames(docType: string, slideCount: number): PreviewFrame[] {
    const slides = Math.max(1, slideCount);
    if (docType !== "video") {
        return Array.from({ length: Math.min(slides, MAX_FRAMES) }, (_, i) => ({ slide: i, ms: 0 }));
    }
    const out: PreviewFrame[] = [];
    // Scene durations are read from the page after it loads; these fractions are
    // resolved against each scene's real data-duration in capturePreview.
    for (let i = 0; i < slides; i++) {
        for (const f of [0.15, 0.55, 0.9]) out.push({ slide: i, ms: -f });
    }
    return out.slice(0, MAX_FRAMES);
}

/**
 * A negative ms is a FRACTION of the scene's own duration (-0.15 = 15% in),
 * resolved once the page has told us how long each scene actually is. Callers
 * that know the timings can pass absolute milliseconds instead.
 */
function resolveMs(ms: number, sceneDurationMs: number): number {
    if (ms >= 0) return ms;
    return Math.round(Math.min(1, -ms) * sceneDurationMs);
}

interface Rect {
    x: number;
    y: number;
    width: number;
    height: number;
}

/**
 * Capture the requested frames.
 *
 * The two doc types are captured differently, and the difference matters.
 *
 * A STILL design is a set of independent slides, each its own output, so each
 * one is captured as an element — exactly what the publish path does.
 *
 * A VIDEO is a single moving picture, so it is captured through a FIXED window
 * (the first slide's box) while seekAt moves the strip behind it — again
 * exactly what the publish path does. Capturing the active scene as an element
 * instead would follow it under its transition transform and show the incoming
 * scene alone, hiding the very compositing the frame was taken to check.
 */
async function captureFrames(
    page: Page,
    frames: PreviewFrame[],
    offsets: number[],
    durations: number[],
    isVideo: boolean,
    clip: Rect | undefined
): Promise<{ images: string[]; captured: PreviewFrame[] }> {
    const slides = await page.$$("[data-slide]");
    const targets = slides.length ? slides : [await page.$("body")];
    const images: string[] = [];
    const captured: PreviewFrame[] = [];

    for (const frame of frames) {
        const index = Math.max(0, Math.min(frame.slide, targets.length - 1));
        const el = targets[index];
        if (!el) continue;
        const ms = resolveMs(frame.ms, durations[index] ?? 3000);

        let buf: Buffer;
        if (isVideo && clip) {
            // seekAt takes GLOBAL time, so a scene-local ms is lifted onto the
            // whole timeline. That also means an inspection frame shows any
            // transition the model wrote — which is the point: a cross-fade it
            // cannot see is a cross-fade it cannot fix.
            await page.evaluate(seekAt, { ms: (offsets[index] ?? 0) + ms });
            buf = await page.screenshot({ type: "jpeg", quality: 78, clip });
        } else {
            buf = await el.screenshot({ type: "jpeg", quality: 78 });
        }
        images.push(`data:image/jpeg;base64,${buf.toString("base64")}`);
        captured.push({ slide: index, ms });
    }
    return { images, captured };
}

/**
 * Load a revision and return a few frames of it.
 *
 * `maxEdge` becomes a deviceScaleFactor rather than a post-resize: Chromium
 * paints once, at the size we want, instead of painting a 1080-wide frame and
 * throwing most of it away.
 */
export async function runPreview(browser: Browser, req: PreviewRequest): Promise<PreviewResponse> {
    const rev = await getRevision(req.brandId, req.contentId, req.revisionId);
    if (!rev) return { ok: false, error: "design revision not found" };
    if (!rev.html.trim()) return { ok: false, error: "design revision has no html" };

    const maxEdge = Math.min(900, Math.max(320, req.maxEdge ?? 640));
    const scale = Math.min(1, maxEdge / Math.max(rev.width, rev.height));

    const loaded = await loadDesign(browser, rev.html, rev.width, rev.height, scale);
    try {
        const { offsets, durations, count } = loaded.measurement;
        const wanted = (req.frames?.length ? req.frames : defaultFrames(rev.docType, count)).slice(0, MAX_FRAMES);
        const { images, captured } = await captureFrames(
            loaded.page,
            wanted,
            offsets,
            durations,
            rev.docType === "video",
            loaded.rects[0]
        );
        return {
            ok: true,
            docType: rev.docType,
            slideCount: count,
            width: rev.width,
            height: rev.height,
            images,
            captured,
        };
    } finally {
        await loaded.close();
    }
}
