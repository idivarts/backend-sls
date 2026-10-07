/**
 * Worker configuration, read once at boot.
 *
 * Every value has a default that works, so a missing variable degrades rather
 * than crashes — except the two that cannot be guessed (the S3 bucket and the
 * CDN host), which throw loudly at startup instead of silently uploading a
 * render nobody can reach.
 */

function required(name: string): string {
    const v = process.env[name];
    if (!v) throw new Error(`${name} is not set — the worker cannot upload renders without it`);
    return v;
}

function num(name: string, fallback: number): number {
    const v = Number(process.env[name]);
    return Number.isFinite(v) && v > 0 ? v : fallback;
}

export const config = {
    /** Where renders land. Same bucket + distribution the app's uploads use. */
    s3Bucket: () => required("ATTACHMENT_S3_BUCKET_NAME"),
    cdnBase: () => required("ATTACHMENT_CF_DISTRIBUTION_URL").replace(/\/+$/, ""),
    region: process.env.AWS_REGION || "us-east-1",

    /** Named Firestore database (dev vs prod live in one Firebase project). */
    firestoreDatabaseId: process.env.FIRESTORE_DATABASE_ID || "(default)",

    /**
     * Frames per second for video. 30, not the 15 the client was forced into:
     * html2canvas cost ~1s per frame regardless of resolution, so frame count
     * was the only lever. A real screenshot is ~20-60ms, so smoothness is back
     * on the menu.
     */
    fps: num("RENDER_FPS", 30),

    /** deviceScaleFactor. 1 renders at the design's own CSS pixel size. */
    scale: num("RENDER_SCALE", 1),

    /** x264 quality/speed. veryfast + crf 20 is visually clean at social sizes. */
    x264Preset: process.env.RENDER_X264_PRESET || "veryfast",
    x264Crf: num("RENDER_X264_CRF", 20),

    /** How long one <img> may hold up a capture before we shoot anyway. */
    imageTimeoutMs: num("RENDER_IMAGE_TIMEOUT_MS", 10000),

    /** Hard ceiling on a single job, so a pathological design can't run forever. */
    jobTimeoutMs: num("RENDER_JOB_TIMEOUT_MS", 15 * 60 * 1000),

    /**
     * Progress is a Firestore write, so it is reported every 5% of frames
     * rather than every frame — ~20 writes for a whole video.
     */
    progressStepPct: num("RENDER_PROGRESS_STEP_PCT", 5),
};
