/**
 * Render an animated design to a single MP4.
 *
 * The loop is: seek the page to frame N's exact time, screenshot, push the
 * bytes into FFmpeg's stdin. Frames never touch disk, and the seek is
 * deterministic (every animation paused and its currentTime set), so the same
 * revision always produces the same video.
 *
 * Everything the browser version needed to survive html2canvas is GONE, not
 * ported: static-frame reuse, the encoder backpressure loop, MessageChannel
 * yielding, H.264 level probing, even-dimension rounding. A screenshot is tens
 * of milliseconds and FFmpeg applies its own backpressure through the pipe.
 */
import { spawn } from "node:child_process";
import { readFile, unlink } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import type { Page } from "playwright";
import { sceneAt, seekScene } from "../../../shared-libs/design/design-runtime";
import { buildAudioStage } from "./audio";
import { config } from "./config";
import type { ContentAudio } from "./types";

export interface Rect {
    x: number;
    y: number;
    width: number;
    height: number;
}

export interface VideoInput {
    page: Page;
    /** Scene start offsets in ms and the total length, from measureDesign(). */
    offsets: number[];
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
    const { page, offsets, totalMs, rect, audio, onProgress } = input;
    const fps = config.fps;
    const frames = Math.max(1, Math.round((totalMs / 1000) * fps));

    const audioStage = await buildAudioStage(audio);
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
        "-pix_fmt", "yuv420p",
        "-shortest",
        "-movflags", "+faststart",
        "-y", outPath,
    ];

    const ff = spawn("ffmpeg", args, { stdio: ["pipe", "ignore", "pipe"] });
    let ffErr = "";
    ff.stderr.on("data", (d) => {
        ffErr += String(d);
    });
    const done = new Promise<void>((resolve, reject) => {
        ff.on("error", reject);
        ff.on("close", (code) =>
            code === 0 ? resolve() : reject(new Error(`ffmpeg exited ${code}: ${ffErr.trim()}`))
        );
    });

    let poster: Buffer | null = null;
    let lastReported = -1;

    try {
        for (let i = 0; i < frames; i++) {
            const t = (i * 1000) / fps;
            const idx = sceneAt(offsets, t);
            await page.evaluate(seekScene, { index: idx, localMs: t - offsets[idx] });

            const frame = await page.screenshot({ type: "png", clip: rect });
            if (i === 0) poster = frame;

            // Respect the pipe's backpressure: if FFmpeg hasn't drained, wait
            // rather than buffering the whole video in memory.
            if (!ff.stdin.write(frame)) {
                await new Promise<void>((resolve) => ff.stdin.once("drain", () => resolve()));
            }

            const pct = Math.floor(((i + 1) / frames) * 100);
            if (pct - lastReported >= config.progressStepPct) {
                lastReported = pct;
                onProgress?.((i + 1) / frames);
            }
        }
    } finally {
        ff.stdin.end();
    }

    await done;
    const mp4 = await readFile(outPath);
    await unlink(outPath).catch(() => undefined);
    return { mp4, poster: poster ?? Buffer.alloc(0) };
}
