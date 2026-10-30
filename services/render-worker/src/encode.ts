/**
 * Feed generated frames into FFmpeg over a pipe, safely.
 *
 * Extracted from `video.ts` verbatim rather than reimplemented, because every
 * line of the write loop below is there for a failure that actually happened:
 * an unlistened EPIPE taking the process down, and a render frozen at a
 * percentage forever because it was awaiting "drain" from an FFmpeg that had
 * already exited. The composition renderer needs the same loop for each of its
 * design segments, and a second copy of this is a second place for those bugs
 * to come back.
 *
 * What is NOT here: anything about WHAT to encode. The caller owns the whole
 * argument list, including the output path, so the single-design parity path
 * can pass byte-for-byte the arguments it always has.
 */
import { spawn, type ChildProcess } from "node:child_process";

export interface EncodeFramesInput {
    /** The complete FFmpeg argument list, output path included. */
    args: string[];
    /** How many frames will be written. */
    frames: number;
    /** Produce frame `i`. Called in order, one at a time. */
    produce: (index: number) => Promise<Buffer>;
    /** Called with every frame as it is written, for a poster or a checksum. */
    onFrame?: (index: number, frame: Buffer) => void;
    /** Called as frames land, already throttled by `progressStepPct`. */
    onProgress?: (fraction: number) => void;
    /** Report progress at most every this many percent. */
    progressStepPct?: number;
    /** Prefix for the progress logs, so interleaved segments stay readable. */
    label?: string;
}

export async function encodeFrames(input: EncodeFramesInput): Promise<void> {
    const { args, frames, produce, onFrame, onProgress } = input;
    const stepPct = input.progressStepPct ?? 5;
    const label = input.label ?? "render";

    // Hoisted so the finally can reach it, whatever went wrong.
    let ffmpeg: ChildProcess | null = null;
    try {
        const ff = spawn("ffmpeg", args, { stdio: ["pipe", "ignore", "pipe"] });
        ffmpeg = ff;
        let ffErr = "";
        ff.stderr.on("data", (d) => {
            ffErr += String(d);
        });
        // Once FFmpeg is gone its stdin emits EPIPE, and a stream 'error' with
        // no listener takes the whole process down with it. The write loop
        // below detects the exit properly, via ffGone.
        ff.stdin.on("error", () => undefined);

        const done = new Promise<void>((resolve, reject) => {
            ff.on("error", reject);
            ff.on("close", (code) =>
                code === 0 ? resolve() : reject(new Error(`ffmpeg exited ${code}: ${ffErr.trim()}`))
            );
        });

        // A dead FFmpeg never drains its pipe again, so awaiting "drain" alone
        // blocks the frame loop FOREVER — no error, no exit, just a render
        // frozen at whatever percentage it had reached. This turns that into
        // the real failure, with FFmpeg's own stderr attached.
        const ffGone = done.then(
            () => Promise.reject(new Error(`ffmpeg exited before all ${frames} frames were written`)),
            (err) => Promise.reject(err)
        );
        // The race below is its only consumer; without this an unraced
        // rejection would surface as an unhandled rejection instead of the
        // real error.
        ffGone.catch(() => undefined);

        let lastReported = -1;
        try {
            for (let i = 0; i < frames; i++) {
                const frame = await produce(i);
                onFrame?.(i, frame);

                // Respect the pipe's backpressure: if FFmpeg hasn't drained,
                // wait rather than buffering the whole video in memory — but
                // never wait on a process that has already gone away.
                if (!ff.stdin.write(frame)) {
                    await Promise.race([
                        new Promise<void>((resolve) => ff.stdin.once("drain", () => resolve())),
                        ffGone,
                    ]);
                }

                const pct = Math.floor(((i + 1) / frames) * 100);
                if (pct - lastReported >= stepPct) {
                    lastReported = pct;
                    // Logged as well as reported: when a render stalls, this is
                    // the only record of where it got to.
                    console.log(`[${label}] frame ${i + 1}/${frames} (${pct}%)`);
                    onProgress?.((i + 1) / frames);
                }
            }
        } finally {
            ff.stdin.end();
        }

        await done;
    } finally {
        // A job that failed part-way can leave FFmpeg alive holding the pipe.
        // On Batch the task exits anyway, but a warm Lambda would accumulate
        // one orphaned encoder per failed render.
        if (ffmpeg && ffmpeg.exitCode === null && !ffmpeg.killed) ffmpeg.kill("SIGKILL");
    }
}

/**
 * Run FFmpeg with no piped input and wait for it.
 *
 * The composition renderer's final pass reads files rather than a pipe, so it
 * needs the process handling without the write loop.
 */
export async function runFfmpeg(args: string[], label = "ffmpeg"): Promise<void> {
    await new Promise<void>((resolve, reject) => {
        const ff = spawn("ffmpeg", args, { stdio: ["ignore", "ignore", "pipe"] });
        let err = "";
        ff.stderr.on("data", (d) => {
            err += String(d);
        });
        ff.on("error", reject);
        ff.on("close", (code) =>
            code === 0 ? resolve() : reject(new Error(`${label} exited ${code}: ${err.trim()}`))
        );
    });
}
