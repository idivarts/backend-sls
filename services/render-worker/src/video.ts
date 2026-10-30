/**
 * Render an animated design to a single MP4.
 *
 * The loop is: seek the page to frame N's exact time, screenshot, push the
 * bytes into FFmpeg's stdin. Frames never touch disk, and the seek is
 * deterministic (every animation paused and its currentTime set), so the same
 * revision always produces the same video.
 *
 * ⚠️ That last claim is still NOT true end to end, and it is worth knowing
 * exactly how far it goes. Measured on dev, three renders of one revision:
 *
 *   frame count, audio track bytes, duration   IDENTICAL, every time
 *   video bytes                                248,210 / 246,580 / 248,296
 *   first 11 frames                            byte-identical, every time
 *   frames 11 onward                           31 to 148 of 180 differ, by
 *                                              as little as a single byte
 *
 * Pinning x264's threads (config.x264Threads) fixed the ENCODE: before it, the
 * files diverged from sample 0, and after it the first 11 frames match
 * byte-for-byte. What remains is upstream of the encoder — Chromium does not
 * rasterize animated layers bit-identically between runs, and no amount of
 * pinning on our side changes that.
 *
 * So the seek IS deterministic in the sense that matters (same timeline, same
 * animation clocks, same frame count), and the output is reproducible to within
 * a fraction of a percent, but it is not bit-reproducible. Anything that wants
 * to verify a change to this pipeline by comparing output has to compare the
 * exact invariants above and hold the rest to an envelope.
 *
 * Everything the browser version needed to survive html2canvas is GONE, not
 * ported: static-frame reuse, the encoder backpressure loop, MessageChannel
 * yielding, H.264 level probing, even-dimension rounding. A screenshot is tens
 * of milliseconds and FFmpeg applies its own backpressure through the pipe.
 *
 * ⚠️ This is the PARITY path. Every pre-existing video design now reaches the
 * renderer as a one-clip composition, and a one-clip composition delegates
 * straight back here rather than taking the general timeline path — a concat or
 * an intermediate re-encode would be a second lossy generation, so the output
 * would be equivalent but not identical. The argument list below and the frame
 * sequence fed into it are what "identical" means: changing either changes
 * every video the product has ever made.
 */
import { readFile, unlink } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import type { Page } from "playwright";
import { seekAt } from "../../../shared-libs/design/design-runtime";
import { buildAudioStage } from "./audio";
import { config } from "./config";
import { encodeFrames } from "./encode";
import type { ContentAudio } from "./types";

export interface Rect {
    x: number;
    y: number;
    width: number;
    height: number;
}

export interface VideoInput {
    page: Page;
    /**
     * Total length in ms, from measureDesign(). Scene offsets are NOT passed:
     * seekAt reads the timeline off the DOM itself, so there is one place that
     * decides where a scene starts rather than two that can disagree.
     */
    totalMs: number;
    /** Capture box of a single scene — every frame is clipped to this. */
    rect: Rect;
    audio?: ContentAudio;
    onProgress?: (fraction: number) => void;
}

export interface VideoOutput {
    mp4: Buffer;
    /** First frame, for previews that can't play a video. */
    poster: Buffer;
}

export async function renderVideo(input: VideoInput): Promise<VideoOutput> {
    const { page, totalMs, rect, audio, onProgress } = input;
    const fps = config.fps;
    const frames = Math.max(1, Math.round((totalMs / 1000) * fps));

    // The bed's out-fade is anchored to the video's length, not the clip's:
    // the music is looped and has no end of its own.
    const audioStage = await buildAudioStage(audio, totalMs);
    const outPath = join(tmpdir(), `render_${Date.now()}.mp4`);

    // H.264 needs even dimensions; the scale filter fixes an odd design size
    // without us rounding the capture and cropping a pixel off the layout.
    const args = [
        "-hide_banner",
        "-loglevel", "error",
        "-f", "image2pipe",
        "-framerate", String(fps),
        "-i", "-",
        ...audioStage.inputArgs,
        ...(audioStage.filter ? ["-filter_complex", audioStage.filter] : []),
        "-map", "0:v",
        ...audioStage.outputArgs,
        "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
        "-c:v", "libx264",
        "-preset", config.x264Preset,
        "-crf", String(config.x264Crf),
        // Pinned, not auto. x264's default thread count comes from the host's
        // online processors, which in a container is not the task's vCPU limit —
        // so the same pixels encoded on two different Spot instance types came
        // out as two different files. See config.x264Threads.
        "-threads", String(config.x264Threads),
        "-pix_fmt", "yuv420p",
        // An explicit duration, not -shortest. Every audio chain here is now
        // endless by construction (looping music, apadded voice), so -shortest
        // had nothing finite to stop on except the video — and in the one case
        // it did bite, a voiceover-only track, it truncated the video to the
        // voiceover. frames/fps is exact and matches what is written to the pipe.
        "-t", (frames / fps).toFixed(3),
        "-movflags", "+faststart",
        "-y", outPath,
    ];

    // Everything this job wrote to disk: the encode target plus whatever the
    // audio stage downloaded. Cleared in the finally so a design that throws
    // mid-encode leaves nothing behind — on Batch the task exits anyway, but a
    // warm Lambda keeps its 512 MB /tmp between invocations.
    const tempFiles = [outPath, ...audioStage.tempFiles];

    try {
        console.log(
            `[render] encoding ${frames} frames @ ${fps}fps, ${rect.width}x${rect.height}, ` +
                `${totalMs}ms, audio=${audioStage.filter ? "yes" : "no"}`
        );

        let poster: Buffer | null = null;
        await encodeFrames({
            args,
            frames,
            // seekAt, not sceneAt + seekScene: a scene boundary may now be an
            // OVERLAP rather than a cut, and placing one scene at a time cannot
            // express two of them composited. seekAt owns the whole timeline, so
            // a frame inside a transition is as exact as any other.
            produce: async (i) => {
                await page.evaluate(seekAt, { ms: (i * 1000) / fps });
                return page.screenshot({ type: "png", clip: rect });
            },
            onFrame: (i, frame) => {
                if (i === 0) poster = frame;
            },
            onProgress,
            progressStepPct: config.progressStepPct,
            label: "render",
        });

        const mp4 = await readFile(outPath);
        return { mp4, poster: poster ?? Buffer.alloc(0) };
    } finally {
        await Promise.all(tempFiles.map((p) => unlink(p).catch(() => undefined)));
    }
}

/**
 * Render one design's frames to a silent intermediate file, for a composition
 * that has more than one clip.
 *
 * Separate from renderVideo rather than a flag on it, because the two differ in
 * every decision that matters: no audio (the soundtrack is muxed once, over the
 * whole timeline), a near-lossless CRF (these frames get encoded a second time,
 * so the intermediate must not be where the quality goes), a window into the
 * design's timeline rather than all of it, and no poster.
 *
 * Keeping renderVideo free of those branches is also what keeps the parity path
 * provably unchanged.
 */
export interface DesignSegmentInput {
    page: Page;
    rect: Rect;
    /** Where in the design's own timeline this segment starts. */
    offsetMs: number;
    /**
     * How many frames to render. A frame COUNT rather than a duration, because
     * the composition's graph plans in frames: a segment that rounded its own
     * length independently could come out a frame away from the slot the
     * timeline reserved for it, and that is exactly where a cross-fade breaks.
     */
    frames: number;
    fps: number;
    outPath: string;
    onProgress?: (fraction: number) => void;
    label?: string;
}

export async function renderDesignSegment(input: DesignSegmentInput): Promise<void> {
    const { page, rect, offsetMs, fps, outPath, onProgress } = input;
    const frames = Math.max(1, input.frames);

    const args = [
        "-hide_banner",
        "-loglevel", "error",
        "-f", "image2pipe",
        "-framerate", String(fps),
        "-i", "-",
        "-an",
        "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
        "-c:v", "libx264",
        "-preset", config.x264Preset,
        // Visually lossless, deliberately far below the final CRF. This file is
        // an input to another encode, and a mid-quality intermediate is how a
        // timeline ends up looking worse than the same design rendered alone.
        "-crf", String(config.intermediateCrf),
        "-threads", String(config.x264Threads),
        "-pix_fmt", "yuv420p",
        "-t", (frames / fps).toFixed(3),
        "-y", outPath,
    ];

    await encodeFrames({
        args,
        frames,
        produce: async (i) => {
            await page.evaluate(seekAt, { ms: offsetMs + (i * 1000) / fps });
            return page.screenshot({ type: "png", clip: rect });
        },
        onProgress,
        progressStepPct: config.progressStepPct,
        label: input.label ?? "segment",
    });
}
