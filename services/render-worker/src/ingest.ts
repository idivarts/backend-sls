/**
 * Look at an uploaded video properly, so the product knows what is in it.
 *
 * This runs HERE rather than in the Go backend for one reason: ffprobe and
 * ffmpeg are in this container and nowhere else. The backend could shell out
 * to neither.
 *
 * What it produces is all in service of PLACEMENT. "Put the bit where she
 * picks up the cup at the front" is only answerable if something recorded that
 * there is a cup and when — so the job measures the footage, pulls a frame to
 * show it by, and samples evenly spaced stills for the vision pass that writes
 * the shot list.
 *
 * It deliberately does NOT transcribe or describe: the transcript is a
 * provider call and the shot list is a model call, and both belong with the
 * other provider calls in the backend. This job's job is the parts that need
 * a video toolchain.
 */
import { spawn } from "node:child_process";
import { mkdir, readFile, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { runFfmpeg } from "./encode";
import { upload } from "./s3";

export interface ProbeResult {
    durationMs: number;
    width: number;
    height: number;
    fps: number;
    hasAudio: boolean;
}

export interface IngestOutput extends ProbeResult {
    posterUrl: string;
    sampleFrameUrls: string[];
}

/**
 * How many stills the vision pass gets.
 *
 * Twelve is a judgement, not a measurement: enough that a thirty-second take
 * yields a frame every couple of seconds, few enough that the multimodal call
 * stays affordable — each frame is an image in the prompt, and the cost is
 * per image. A longer video gets the same twelve, spread wider; the shot list
 * is for finding the right REGION, and a person scrubs from there.
 */
const SAMPLE_FRAMES = 12;

/** Run ffprobe and return what it says about the file. */
export async function probe(path: string): Promise<ProbeResult> {
    const raw = await runProbe([
        "-v", "error",
        "-print_format", "json",
        "-show_format",
        "-show_streams",
        path,
    ]);

    let parsed: any;
    try {
        parsed = JSON.parse(raw);
    } catch {
        throw new Error("could not read this video's format");
    }

    const streams: any[] = parsed.streams ?? [];
    const video = streams.find((s) => s.codec_type === "video");
    if (!video) throw new Error("this file has no video track");
    const hasAudio = streams.some((s) => s.codec_type === "audio");

    // Duration can be absent on the video stream and present on the container,
    // or the other way round, depending on how the file was muxed — a phone
    // export and a screen recording disagree about this routinely.
    const durationSecs =
        Number(parsed.format?.duration) || Number(video.duration) || 0;

    return {
        durationMs: Math.round(durationSecs * 1000),
        width: Number(video.width) || 0,
        height: Number(video.height) || 0,
        fps: parseRate(video.avg_frame_rate) || parseRate(video.r_frame_rate) || 0,
        hasAudio,
    };
}

/**
 * Whether a file carries an audio track at all.
 *
 * Needed because naming `[i:a]` for an input that has none does not degrade —
 * it fails the whole filter graph, taking the render with it. A design segment
 * is muxed `-an`, a still has no sound by definition, and plenty of footage
 * arrives mute, so the question has to be asked per file rather than assumed
 * from the clip's kind.
 *
 * Answers `false` when ffprobe cannot say. Losing one clip's sound is a much
 * better failure than losing the render.
 */
export async function hasAudioTrack(path: string): Promise<boolean> {
    try {
        const raw = await runProbe([
            "-v", "error",
            "-select_streams", "a",
            "-show_entries", "stream=codec_type",
            "-print_format", "json",
            path,
        ]);
        return (JSON.parse(raw).streams ?? []).length > 0;
    } catch {
        return false;
    }
}

/** ffprobe reports frame rates as a fraction, e.g. "30000/1001". */
function parseRate(rate: unknown): number {
    if (typeof rate !== "string" || !rate.includes("/")) return 0;
    const [num, den] = rate.split("/").map(Number);
    if (!den) return 0;
    return Math.round((num / den) * 100) / 100;
}

/**
 * Probe a video and extract its poster plus evenly spaced sample frames.
 *
 * The frames are taken at the MIDPOINT of each slice rather than its start.
 * A cut usually lands on a slice boundary, so sampling the boundary itself is
 * the one place most likely to catch a transition frame — a smear of two shots
 * that describes neither.
 */
export async function ingestVideo(url: string, brandId: string): Promise<IngestOutput> {
    const dir = join(tmpdir(), `ingest_${Date.now()}`);
    await mkdir(dir, { recursive: true });
    const source = join(dir, "source");

    try {
        await download(url, source);
        const probed = await probe(source);
        if (probed.durationMs <= 0) throw new Error("this video has no readable length");

        const posterPath = join(dir, "poster.jpg");
        await runFfmpeg(
            ["-hide_banner", "-loglevel", "error", "-i", source,
             "-frames:v", "1", "-q:v", "3", "-y", posterPath],
            "ingest poster"
        );
        const posterUrl = await upload(await readFile(posterPath), "jpg", "image/jpeg");

        const count = Math.min(SAMPLE_FRAMES, Math.max(1, Math.round(probed.durationMs / 1000)));
        const sampleFrameUrls: string[] = [];
        for (let i = 0; i < count; i++) {
            const atMs = Math.round((probed.durationMs * (i + 0.5)) / count);
            const framePath = join(dir, `sample_${i}.jpg`);
            await runFfmpeg(
                ["-hide_banner", "-loglevel", "error",
                 // -ss before -i seeks by keyframe, which is far cheaper than
                 // decoding from the top for every one of twelve frames. A
                 // still for describing a shot does not need to be the exact
                 // frame asked for.
                 "-ss", (atMs / 1000).toFixed(3), "-i", source,
                 "-frames:v", "1", "-q:v", "4",
                 // Downscaled hard: these are read by a vision model, which
                 // sees nothing extra in a 4K still and charges for the pixels.
                 "-vf", "scale=768:-2",
                 "-y", framePath],
                `ingest frame ${i}`
            );
            sampleFrameUrls.push(await upload(await readFile(framePath), "jpg", "image/jpeg"));
        }

        return { ...probed, posterUrl, sampleFrameUrls };
    } finally {
        await rm(dir, { recursive: true, force: true }).catch(() => undefined);
    }
}

function runProbe(args: string[]): Promise<string> {
    return new Promise((resolve, reject) => {
        const ff = spawn("ffprobe", args, { stdio: ["ignore", "pipe", "pipe"] });
        let out = "";
        let err = "";
        ff.stdout.on("data", (d) => (out += String(d)));
        ff.stderr.on("data", (d) => (err += String(d)));
        ff.on("error", reject);
        ff.on("close", (code) =>
            code === 0 ? resolve(out) : reject(new Error(`ffprobe exited ${code}: ${err.trim()}`))
        );
    });
}

async function download(url: string, path: string): Promise<void> {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`could not fetch the video (HTTP ${res.status})`);
    const { writeFile } = await import("node:fs/promises");
    await writeFile(path, Buffer.from(await res.arrayBuffer()));
}
