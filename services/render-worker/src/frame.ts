/**
 * Cut one still out of a video and hand back the bytes.
 *
 * This exists for CHAINING. A chained generation takes the last frame of the
 * previous clip as its start frame, which is the only way to build a sequence
 * longer than one model call that still looks like one shot. It also makes an
 * anchor still: one approved frame of a face, referenced by every later clip,
 * which is what keeps a person recognisable across separate beats.
 *
 * It runs here rather than in the Go backend for the same reason ingest does —
 * ffmpeg is in this container and nowhere else.
 *
 * ⚠️ THE LAST FRAME IS NOT `duration`. Seeking to the reported duration lands
 * one frame past the end and ffmpeg returns nothing at all, or a black frame
 * where the container is padded. So the length is probed, a frame's worth is
 * stepped back, and the seek is clamped into the file. The caller asks for
 * `atMs: -1` and does not have to know any of that, which is the point: the
 * backend has no ffprobe, so a caller computing its own offset would be
 * guessing at a number only this container can read.
 */
import { mkdir, readFile, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { runFfmpeg } from "./encode";
import { download, probe } from "./ingest";

export interface ExtractedFrame {
    bytes: Buffer;
    /** Where in the source the frame actually came from, after clamping. */
    atMs: number;
    width: number;
    height: number;
    /** The source's full length, so the caller can record what it cut from. */
    sourceDurationMs: number;
}

/**
 * How far back from the end "the last frame" means when the frame rate is
 * unknown. 60ms is a little over one frame at 15fps and several at 60 — far
 * enough in to be a real frame, near enough the end to be the end.
 */
const LAST_FRAME_BACKOFF_MS = 60;

/**
 * Extract a frame.
 *
 * `atMs` of -1 (or anything negative) means the last frame. Any other value is
 * clamped into the file, because an offset past the end produces either nothing
 * or black — and a black still becomes the opening image of the next clip in a
 * chain, which is a failure that only shows up in the finished video.
 */
export async function extractFrame(url: string, atMs: number): Promise<ExtractedFrame> {
    const dir = join(tmpdir(), `frame_${Date.now()}`);
    await mkdir(dir, { recursive: true });
    const source = join(dir, "source");

    try {
        await download(url, source);
        const probed = await probe(source);
        if (probed.durationMs <= 0) throw new Error("this video has no readable length");

        const target = resolveOffset(atMs, probed.durationMs, probed.fps);
        const out = join(dir, "frame.png");

        await runFfmpeg(
            ["-hide_banner", "-loglevel", "error",
             // -ss AFTER -i decodes from the top, which is slower but lands on
             // the exact frame. For a start frame that matters: a keyframe seek
             // can be a second out, and a second out at the end of a clip is a
             // different shot.
             "-i", source,
             "-ss", (target / 1000).toFixed(3),
             "-frames:v", "1",
             // PNG, not JPEG. This still is fed back into a generation model
             // as a start frame, and JPEG ringing around a face is exactly the
             // artefact the next clip would inherit and amplify.
             "-y", out],
            "frame extract"
        );

        return {
            bytes: await readFile(out),
            atMs: target,
            width: probed.width,
            height: probed.height,
            sourceDurationMs: probed.durationMs,
        };
    } finally {
        await rm(dir, { recursive: true, force: true }).catch(() => undefined);
    }
}

/** Clamp a requested offset into something that will decode to a real frame. */
export function resolveOffset(atMs: number, durationMs: number, fps: number): number {
    const frameMs = fps > 0 ? Math.ceil(1000 / fps) : LAST_FRAME_BACKOFF_MS;
    const last = Math.max(0, durationMs - Math.max(frameMs, 1));
    if (atMs < 0) return last;
    return Math.min(Math.max(0, atMs), last);
}
