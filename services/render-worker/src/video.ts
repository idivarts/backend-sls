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
import { spawn, type ChildProcess } from "node:child_process";
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
    // audio stage downloaded. Cleared in the outer finally so a design that
    // throws mid-encode leaves nothing behind — on Batch the task exits anyway,
    // but a warm Lambda keeps its 512 MB /tmp between invocations.
    const tempFiles = [outPath, ...audioStage.tempFiles];

    // Hoisted so the outer finally can reach it, whatever went wrong.
    let ffmpeg: ChildProcess | null = null;

    try {
        console.log(
            `[render] encoding ${frames} frames @ ${fps}fps, ${rect.width}x${rect.height}, ` +
                `${totalMs}ms, audio=${audioStage.filter ? "yes" : "no"}`
        );

        const ff = spawn("ffmpeg", args, { stdio: ["pipe", "ignore", "pipe"] });
        ffmpeg = ff;
        let ffErr = "";
        ff.stderr.on("data", (d) => {
            ffErr += String(d);
        });
        // Once FFmpeg is gone its stdin emits EPIPE, and a stream 'error' with
        // no listener takes the whole process down with it. The write loop below
        // detects the exit properly, via ffGone.
        ff.stdin.on("error", () => undefined);

        const done = new Promise<void>((resolve, reject) => {
            ff.on("error", reject);
            ff.on("close", (code) =>
                code === 0 ? resolve() : reject(new Error(`ffmpeg exited ${code}: ${ffErr.trim()}`))
            );
        });

        // A dead FFmpeg never drains its pipe again, so awaiting "drain" alone
        // blocks the frame loop FOREVER — no error, no exit, just a render
        // frozen at whatever percentage it had reached. This turns that into the
        // real failure, with FFmpeg's own stderr attached.
        const ffGone = done.then(
            () => Promise.reject(new Error(`ffmpeg exited before all ${frames} frames were written`)),
            (err) => Promise.reject(err)
        );
        // The race below is its only consumer; without this an unraced rejection
        // would surface as an unhandled rejection instead of the real error.
        ffGone.catch(() => undefined);

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
                // rather than buffering the whole video in memory — but never
                // wait on a process that has already gone away.
                if (!ff.stdin.write(frame)) {
                    await Promise.race([
                        new Promise<void>((resolve) => ff.stdin.once("drain", () => resolve())),
                        ffGone,
                    ]);
                }

                const pct = Math.floor(((i + 1) / frames) * 100);
                if (pct - lastReported >= config.progressStepPct) {
                    lastReported = pct;
                    // Logged as well as written to Firestore: when a render
                    // stalls, this is the only record of where it got to.
                    console.log(`[render] frame ${i + 1}/${frames} (${pct}%)`);
                    onProgress?.((i + 1) / frames);
                }
            }
        } finally {
            ff.stdin.end();
        }

        await done;
        const mp4 = await readFile(outPath);
        return { mp4, poster: poster ?? Buffer.alloc(0) };
    } finally {
        // A job that failed part-way can leave FFmpeg alive holding the pipe.
        // On Batch the task exits anyway, but a warm Lambda would accumulate one
        // orphaned encoder per failed render.
        if (ffmpeg && ffmpeg.exitCode === null && !ffmpeg.killed) ffmpeg.kill("SIGKILL");
        await Promise.all(tempFiles.map((p) => unlink(p).catch(() => undefined)));
    }
}
