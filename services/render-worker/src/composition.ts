/**
 * Render a composition — a timeline of clips — to a single MP4.
 *
 * ── The one rule this file exists to keep ───────────────────────────────────
 * Footage stays in FFmpeg, HTML stays in Chromium, and compositing happens in
 * FFmpeg. Never the other way around, and in particular never a <video> element
 * inside a design: the frame loop only pauses and seeks getAnimations(), so a
 * video element would sit frozen at frame 0 or drift on wall-clock, and
 * reproducibility — the whole reason rendering moved off the client — would be
 * gone.
 *
 * ── Why design clips go to intermediate files ───────────────────────────────
 * The tidier shape would be one FFmpeg invocation with every clip as an input:
 * one encode, no generation loss. It does not work for design clips. FFmpeg
 * pulls from its inputs as the filter graph demands, and a cross-fade demands
 * two of them interleaved — so two design pipes would have to be fed
 * concurrently, from one Playwright page that can only be at one instant at a
 * time. Buffering a whole clip to feed it on demand puts an uncompressed video
 * in memory.
 *
 * So: design clips are rendered to near-lossless intermediates first (one page,
 * sequential, no deadlock), everything else is a direct FFmpeg input, and the
 * final pass composites and encodes once.
 *
 * ── And why one whole design skips all of this ──────────────────────────────
 * A composition that is exactly one design is what every video design already
 * was. It delegates to renderVideo, unchanged, because an intermediate plus a
 * final encode is a second lossy generation: the output would be equivalent but
 * not identical, and byte-identical output on every pre-existing video design
 * is this phase's acceptance test.
 */
import { readFile, unlink, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import type { Browser } from "playwright";
import {
    measureComposition,
    type CompositionMeasurement,
    singleDesignPassthrough,
    validateComposition,
    type Clip,
    type Composition,
    type MeasuredClip,
} from "../../../shared-libs/design/composition-runtime";
import { buildAudioStage } from "./audio";
import { config } from "./config";
import { runFfmpeg } from "./encode";
import { loadDesign } from "./page";
import { renderDesignSegment, renderVideo, type VideoOutput } from "./video";
import type { ContentAudio } from "./types";

export interface CompositionInput {
    browser: Browser;
    composition: Composition;
    /**
     * Resolve a design clip's stored HTML. Injected rather than imported so
     * this file does not reach into Firestore — and so a test can render a
     * composition from HTML it holds in hand.
     */
    loadRevision: (revisionId: string) => Promise<{ html: string; width: number; height: number }>;
    audio?: ContentAudio;
    onProgress?: (fraction: number) => void;
}

/**
 * One clip's place on the timeline, measured in FRAMES rather than milliseconds.
 *
 * This exists because of an exact equality that xfade depends on. Chained,
 * xfade needs its first input to run exactly up to `offset + duration`, and
 * that holds in millisecond arithmetic — but a clip is ENCODED as a whole
 * number of frames, so a duration that is not a multiple of the frame interval
 * comes out up to half a frame away from what the timeline declared. Feed the
 * graph millisecond offsets and the drift lands exactly on the boundary
 * condition, where a clip one frame short leaves the cross-fade with nothing to
 * fade from.
 *
 * So the renderer plans in frames, converts to seconds only at the edges, and
 * declared length and encoded length cannot disagree. The shared runtime stays
 * fps-agnostic, which is right for layout: frames only exist here.
 */
interface ClipPlan {
    index: number;
    /** Frames this clip contributes. */
    frames: number;
    /** Frames it overlaps the previous clip. 0 for a cut and for the first clip. */
    overlapFrames: number;
    /** Its start on the composition timeline, in frames. */
    startFrames: number;
}

/** A clip reduced to what the final FFmpeg pass needs from it. */
interface Segment {
    measured: MeasuredClip;
    plan: ClipPlan;
    /** FFmpeg args that declare this clip's input, in track order. */
    inputArgs: string[];
    /** Filter chain applied to this input before it is composited. */
    filter: string;
}

/**
 * Snap the measurement onto the frame grid, preserving the runtime's invariant
 * that offsets strictly increase.
 */
function planClips(measurement: CompositionMeasurement): { plans: ClipPlan[]; totalFrames: number } {
    const fps = measurement.fps;
    const toFrames = (ms: number) => Math.round((ms / 1000) * fps);

    const plans: ClipPlan[] = [];
    let cursor = 0;
    for (const clip of measurement.clips) {
        const frames = Math.max(1, toFrames(clip.durationMs));
        let overlapFrames = clip.index === 0 ? 0 : toFrames(clip.transition.ms);
        if (overlapFrames > 0) {
            // Re-clamp in frame space. The runtime already clamped in ms, but
            // rounding both sides independently could put the overlap back over
            // half of the shorter clip, and the whole point of that clamp is
            // that offsets keep increasing.
            const half = Math.floor(Math.min(plans[clip.index - 1].frames, frames) / 2);
            if (overlapFrames > half) overlapFrames = half;
        }
        const startFrames = clip.index === 0 ? 0 : cursor - overlapFrames;
        plans.push({ index: clip.index, frames, overlapFrames, startFrames });
        cursor = startFrames + frames;
    }
    return { plans, totalFrames: cursor };
}

export async function renderComposition(input: CompositionInput): Promise<VideoOutput> {
    const { browser, composition, audio, onProgress } = input;

    const fatal = validateComposition(composition).filter((p) => p.fatal);
    if (fatal.length) {
        // The first message is user-facing and reads as a sentence; the rest go
        // to the log so a support question has the whole picture.
        if (fatal.length > 1) {
            console.error("[composition] problems:", fatal.map((p) => `${p.path}: ${p.message}`));
        }
        throw new Error(fatal[0].message);
    }

    const passthrough = singleDesignPassthrough(composition);
    if (passthrough) {
        return renderSingleDesign(input, passthrough.revisionId);
    }
    return renderTimeline(input);
}

/**
 * One whole design: load it and hand it to renderVideo exactly as the
 * pre-composition path did.
 *
 * Note what is NOT taken from the composition here — not its fps, not its
 * width or height, not its duration. The design's own measurement decides all
 * of those, as it always has. A composition derived from a design carries the
 * same numbers, but reading them from the composition instead would make the
 * output depend on a derived copy rather than on the design itself, and the
 * first rounding difference between the two would be a different video.
 */
async function renderSingleDesign(input: CompositionInput, revisionId: string): Promise<VideoOutput> {
    const revision = await input.loadRevision(revisionId);
    const loaded = await loadDesign(input.browser, revision.html, revision.width, revision.height);
    try {
        return await renderVideo({
            page: loaded.page,
            totalMs: loaded.measurement.total,
            rect: loaded.rects[0],
            audio: input.audio,
            onProgress: input.onProgress,
        });
    } finally {
        await loaded.close();
    }
}

async function renderTimeline(input: CompositionInput): Promise<VideoOutput> {
    const { composition, audio, onProgress } = input;
    const measurement = measureComposition(composition);
    const { width, height, fps } = measurement;
    const { plans, totalFrames } = planClips(measurement);
    // The frame-aligned total, not the measurement's. This is the length that
    // will actually be encoded, so it is the length the soundtrack's out-fade
    // and the encode's -t must both be anchored to.
    const totalMs = Math.round((totalFrames / fps) * 1000);

    const tempFiles: string[] = [];
    const outPath = join(tmpdir(), `composition_${Date.now()}.mp4`);
    const posterPath = join(tmpdir(), `composition_${Date.now()}_poster.png`);
    tempFiles.push(outPath, posterPath);

    try {
        console.log(
            `[composition] ${measurement.clips.length} clips, ${totalMs}ms, ${width}x${height} @ ${fps}fps`
        );

        // Design clips are rendered first and sequentially: they share one
        // browser, and their progress is the part of the job that actually takes
        // time, so it is the part the user watches.
        const designWork = measurement.clips.filter((c) => c.clip.source.kind === "design");
        let designsDone = 0;
        const segments: Segment[] = [];

        for (const measured of measurement.clips) {
            const source = measured.clip.source;
            const plan = plans[measured.index];
            if (source.kind === "design") {
                const path = join(tmpdir(), `segment_${measured.index}_${Date.now()}.mp4`);
                tempFiles.push(path);
                await renderDesignClip({
                    input,
                    measured,
                    plan,
                    fps,
                    outPath: path,
                    onProgress: (f) => {
                        // Design rendering is treated as the first 90% of the
                        // job; the final encode is the rest. A bar that sits at
                        // 100% through a long mux is worse than one that admits
                        // there is a step left.
                        const share = (designsDone + f) / Math.max(designWork.length, 1);
                        onProgress?.(share * 0.9);
                    },
                });
                designsDone++;
                // No filter of its own. The segment is encoded at the DESIGN's
                // own slide size — which may not be the composition's, and a
                // design in the wrong aspect ratio should be letterboxed, not
                // reframed — so buildVideoGraph's single normalization pass is
                // what fits it to the frame. Adding one here would just be a
                // second fps/format/setsar pass for no effect.
                segments.push({ measured, plan, inputArgs: ["-i", path], filter: "" });
                continue;
            }

            const local = await stage(source, measured, plan, fps, width, height);
            tempFiles.push(...local.tempFiles);
            segments.push({ measured, plan, inputArgs: local.inputArgs, filter: local.filter });
        }

        onProgress?.(0.9);

        // Audio inputs come after every video input, so the stage has to be told
        // where it starts — the default of 1 is only right for a single pipe.
        const audioStage = await buildAudioStage(audio, totalMs, segments.length);
        tempFiles.push(...audioStage.tempFiles);

        const videoFilter = buildVideoGraph(segments, width, height, fps);
        const filter = audioStage.filter ? `${videoFilter};${audioStage.filter}` : videoFilter;

        const args = [
            "-hide_banner",
            "-loglevel", "error",
            ...segments.flatMap((s) => s.inputArgs),
            ...audioStage.inputArgs,
            "-filter_complex", filter,
            "-map", "[outv]",
            ...audioStage.outputArgs,
            "-c:v", "libx264",
            "-preset", config.x264Preset,
            "-crf", String(config.x264Crf),
            "-threads", String(config.x264Threads),
            "-pix_fmt", "yuv420p",
            // Explicit, like the design path: every audio chain here is endless
            // by construction, so -shortest would have nothing finite to stop
            // on but the video, and in the voiceover-only case it truncated the
            // video to the voiceover.
            "-t", (totalFrames / fps).toFixed(3),
            "-movflags", "+faststart",
            "-y", outPath,
        ];
        await runFfmpeg(args, "composition encode");

        // The poster is pulled from the finished file rather than kept from a
        // segment: with a cross-fade on the first join, the composition's first
        // frame is not any clip's first frame.
        await runFfmpeg(
            ["-hide_banner", "-loglevel", "error", "-i", outPath, "-frames:v", "1", "-y", posterPath],
            "composition poster"
        );

        onProgress?.(1);
        const [mp4, poster] = await Promise.all([
            readFile(outPath),
            readFile(posterPath).catch(() => Buffer.alloc(0)),
        ]);
        return { mp4, poster };
    } finally {
        await Promise.all(tempFiles.map((p) => unlink(p).catch(() => undefined)));
    }
}

/** Render one design clip — whole design, or a single scene of it — to a file. */
async function renderDesignClip(args: {
    input: CompositionInput;
    measured: MeasuredClip;
    plan: ClipPlan;
    fps: number;
    outPath: string;
    onProgress?: (fraction: number) => void;
}): Promise<void> {
    const { input, measured, plan, fps, outPath, onProgress } = args;
    const source = measured.clip.source;
    if (source.kind !== "design") throw new Error("renderDesignClip called with a non-design clip");

    const revision = await input.loadRevision(source.revisionId);
    const loaded = await loadDesign(input.browser, revision.html, revision.width, revision.height);
    try {
        // A scene index addresses the design's own timeline, so the offset comes
        // from the DESIGN's measurement, not the composition's — design-runtime
        // owns where a scene starts, and reading it from anywhere else is how
        // the two drift.
        const offsets = loaded.measurement.offsets;
        let offsetMs = 0;
        if (source.sceneIndex !== undefined && source.sceneIndex !== null) {
            const i = Math.min(Math.max(source.sceneIndex, 0), offsets.length - 1);
            offsetMs = offsets[i];
        }
        await renderDesignSegment({
            page: loaded.page,
            rect: loaded.rects[0],
            offsetMs,
            frames: plan.frames,
            fps,
            outPath,
            onProgress,
            label: `clip ${measured.index}`,
        });
    } finally {
        await loaded.close();
    }
}

interface StagedInput {
    inputArgs: string[];
    filter: string;
    tempFiles: string[];
}

/**
 * Turn a non-design clip into an FFmpeg input.
 *
 * Remote urls are downloaded rather than handed to FFmpeg as http inputs. The
 * audio stage already does this, for the reason that applies here too: whether
 * the container's FFmpeg has network protocols compiled in is not something a
 * render should depend on, and a download that fails says so plainly instead of
 * producing an "Invalid data found" that looks like a corrupt file.
 */
async function stage(
    source: Clip["source"],
    measured: MeasuredClip,
    plan: ClipPlan,
    fps: number,
    width: number,
    height: number
): Promise<StagedInput> {
    // Frames, not the declared ms: an input that ends a frame before the graph
    // expects it to is what breaks a cross-fade on the join after it.
    const seconds = (plan.frames / fps).toFixed(3);

    if (source.kind === "image") {
        const path = await download(source.url, `image_${measured.index}`);
        return {
            tempFiles: [path],
            // -loop with an explicit framerate and duration: one still becomes
            // exactly the frames this slot needs.
            inputArgs: ["-loop", "1", "-framerate", String(fps), "-t", seconds, "-i", path],
            filter: kenBurns(source.kenBurns, plan.frames, width, height),
        };
    }

    if (source.kind === "footage" || source.kind === "genvideo") {
        if (!source.url) throw new Error("This clip has no video to render yet.");
        const path = await download(source.url, `clip_${measured.index}`);

        const speed = source.kind === "footage" && source.speed && source.speed > 0 ? source.speed : 1;
        const inMs = source.kind === "footage" ? source.inMs ?? 0 : 0;
        // At 2x, filling a 3-second slot needs 6 seconds of source. Reading only
        // the slot's worth would run the clip out and freeze on its last frame.
        const readSeconds = ((plan.frames / fps) * speed).toFixed(3);

        // Only the source-specific part; buildVideoGraph normalizes size, rate
        // and timebase for every input in one place.
        const chain = speed !== 1 ? `setpts=PTS/${speed}` : "";
        return {
            tempFiles: [path],
            // -ss before -i seeks by keyframe and is far cheaper than decoding
            // from the top; the trim is a user-facing in-point, not a
            // frame-exact edit, so keyframe accuracy is the right trade.
            inputArgs: ["-ss", (inMs / 1000).toFixed(3), "-t", readSeconds, "-i", path],
            filter: chain,
        };
    }

    throw new Error(`Unsupported clip source "${(source as { kind: string }).kind}".`);
}

/**
 * Bring an input to the composition's exact size, rate, pixel format and
 * timebase.
 *
 * Every one of these is load-bearing for the compositing step rather than
 * cosmetic: concat and xfade both refuse inputs that disagree on any of them,
 * and `settb` in particular is the difference between a working cross-fade and
 * "First input link ... timebase do not match". `width`/`height` of 0 keeps the
 * source's own size, for an input already scaled by another filter.
 */
function normalize(width: number, height: number, fps: number): string {
    const parts: string[] = [];
    if (width > 0 && height > 0) {
        // Fit inside the frame and pad, rather than crop: a clip shot in the
        // wrong aspect ratio should look letterboxed, not silently reframed.
        parts.push(`scale=${width}:${height}:force_original_aspect_ratio=decrease`);
        parts.push(`pad=${width}:${height}:(ow-iw)/2:(oh-ih)/2:color=black`);
    }
    parts.push(`fps=${fps}`, "format=yuv420p", "setsar=1", "settb=AVTB");
    return parts.join(",");
}

/**
 * A slow push on a still, so a background photo is not visually dead.
 *
 * `s` is NOT optional, however much it looks it: zoompan's default output size
 * is hd720, so leaving it out silently renders a portrait reel at 1280x720 and
 * the pad downstream letterboxes the result into a stripe. The crop before it
 * is what gives the zoom pixels to move into — a still scaled to fit would zoom
 * into its own letterbox.
 *
 * The zoom is a function of the output frame number alone, so it is exact and
 * reproducible, like every other seek in the renderer.
 */
function kenBurns(
    mode: string | undefined,
    frames: number,
    width: number,
    height: number
): string {
    if (!mode || mode === "none") return "";
    const z = mode === "out" ? `1.15-0.15*on/${frames}` : `1+0.15*on/${frames}`;
    return [
        `scale=${width}:${height}:force_original_aspect_ratio=increase`,
        `crop=${width}:${height}`,
        // No fps= here: buildVideoGraph pins the rate for every input, and a
        // zoompan that set its own would be a second, conflicting opinion.
        `zoompan=z='${z}':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=1:s=${width}x${height}`,
    ].join(",");
}

/**
 * Chain the segments into one video stream, left to right.
 *
 * Pairwise rather than one n-way concat, because a timeline mixes cuts and
 * cross-fades freely and the two filters cannot be interleaved in a single
 * n-way call. xfade's `offset` is measured from the start of its first input,
 * which after chaining IS the composition's start — so the offset of each join
 * is simply the measured start of the clip arriving, with no running total to
 * get wrong.
 */
function buildVideoGraph(segments: Segment[], width: number, height: number, fps: number): string {
    const toSeconds = (frames: number) => (frames / fps).toFixed(3);
    const parts: string[] = [];
    segments.forEach((segment, i) => {
        // An empty filter chain would be "[0:v][v0]" — a null filter is needed
        // to label a stream that nothing else touches.
        const chain = [segment.filter, normalize(width, height, fps)].filter(Boolean).join(",");
        parts.push(`[${i}:v]${chain}[v${i}]`);
    });

    let acc = "v0";
    for (let i = 1; i < segments.length; i++) {
        const next = `x${i}`;
        const overlap = segments[i].plan.overlapFrames;
        if (overlap > 0) {
            const offset = toSeconds(segments[i].plan.startFrames);
            const duration = toSeconds(overlap);
            parts.push(
                `[${acc}][v${i}]xfade=transition=fade:duration=${duration}:offset=${offset}[${next}]`
            );
        } else {
            parts.push(`[${acc}][v${i}]concat=n=2:v=1:a=0[${next}]`);
        }
        acc = next;
    }

    // An explicit copy so the last label is always [outv], whether there was one
    // clip or ten — the -map argument should not have to know which.
    parts.push(`[${acc}]null[outv]`);
    return parts.join(";");
}

async function download(url: string, name: string): Promise<string> {
    const res = await fetch(url);
    if (!res.ok) {
        throw new Error(`could not fetch ${name} (HTTP ${res.status})`);
    }
    const path = join(tmpdir(), `composition_${name}_${Date.now()}`);
    await writeFile(path, Buffer.from(await res.arrayBuffer()));
    return path;
}

/** Exported for tests — the graph is the part worth asserting on. */
export const __test = { buildVideoGraph, normalize, kenBurns, planClips };
export type { ClipPlan, Segment };
