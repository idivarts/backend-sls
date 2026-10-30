/**
 * Build the FFmpeg audio stage: the clips' own sound, a voiceover over it, and
 * a looping music bed ducked under both — mixed to one stereo track.
 *
 * Clip audio arrives late. The composition's soundtrack used to be the ONLY
 * thing that could be heard: every clip was muxed with `-an`, so a generated
 * clip that spoke, or footage shot with sound, rendered silent. That was fine
 * while the only video model we offered was asked for silence, and wrong as
 * soon as anybody asked one to talk.
 *
 * ⚠️ The filter graph in `docs/render-worker.md` was wrong and is corrected
 * here. `sidechaincompress` outputs ONLY its first input (the ducked music) —
 * mapping that straight to the output drops the voiceover entirely. The voice
 * has to be split: one copy drives the ducking, the other is mixed back in.
 *
 * Unlike the browser's `prepAudio`, a failed fetch here is NOT swallowed. The
 * old path resolved null and shipped a silent video with no error, which is
 * indistinguishable to the user from "the soundtrack feature is broken".
 */
import { unlink, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import type { ContentAudio } from "./types";

export interface AudioStage {
    /** FFmpeg args that declare the audio inputs, in order after the video. */
    inputArgs: string[];
    /** The -filter_complex value, or null when there is no audio. */
    filter: string | null;
    /** Args that map and encode the mixed track. */
    outputArgs: string[];
    /**
     * Temp files this stage downloaded. The caller MUST unlink them once FFmpeg
     * has exited — they are inputs to a running process, so they cannot be
     * cleaned up here. On Batch the task exits and takes them with it, but the
     * same code runs in a warm Lambda where /tmp is 512 MB and survives between
     * invocations, so leaking them would eventually fill the disk.
     */
    tempFiles: string[];
}

async function download(url: string, name: string): Promise<string> {
    const res = await fetch(url);
    if (!res.ok) {
        throw new Error(`could not fetch ${name} (HTTP ${res.status})`);
    }
    const path = join(tmpdir(), `render_${name}_${Date.now()}`);
    await writeFile(path, Buffer.from(await res.arrayBuffer()));
    return path;
}

/**
 * Seconds of fade on the music bed, in and out.
 *
 * Applied always rather than behind a switch. `musicFade` existed as a toggle
 * in the UI and was implemented NOWHERE — not here, not in the browser preview
 * — so it was a control that lied. A bed that starts at full level and stops
 * dead on the last frame sounds unfinished every time, and nobody has ever
 * wanted the other option, so this is a default rather than a decision.
 */
const FADE_IN_S = 0.4;
const FADE_OUT_S = 0.6;

/**
 * One clip's own audio, as a position on the finished timeline.
 *
 * The clip is ALREADY an FFmpeg input — the picture needs it — so this names
 * that input rather than opening the file again, and `startMs` comes from the
 * same frame-aligned plan the video graph lays out. Nothing here re-trims: the
 * input was declared with `-ss`/`-t`, which cut the audio with the picture.
 */
export interface ClipAudio {
    /** Position of this clip's input in the FFmpeg argument list. */
    inputIndex: number;
    /** Where the clip starts in the finished video, from the frame-aligned plan. */
    startMs: number;
    /** Playback speed applied to the picture; the sound has to match it. */
    speed?: number;
}

/**
 * `atempo` only accepts 0.5–2.0 per instance, so anything beyond that is a
 * chain. Changing tempo rather than resampling is the point: a clip played at
 * 2x should sound faster, not an octave higher.
 */
function atempoChain(speed: number): string {
    if (!(speed > 0) || speed === 1) return "";
    const steps: string[] = [];
    let remaining = speed;
    while (remaining > 2) {
        steps.push("atempo=2.0");
        remaining /= 2;
    }
    while (remaining < 0.5) {
        steps.push("atempo=0.5");
        remaining /= 0.5;
    }
    steps.push(`atempo=${remaining.toFixed(4)}`);
    return steps.join(",");
}

/**
 * `amix` divides every input by the number of inputs unless told not to, so a
 * `musicVolume` of 0.7 played at 0.35 the moment a voiceover existed, and two
 * sequential clips — which never overlap — would each have played at half
 * volume for no reason at all. `normalize=0` makes the volume controls mean
 * what they say; the limiter on the way out is what stops the honest sum from
 * clipping.
 */
const MIX = "duration=longest:dropout_transition=0:normalize=0";
const LIMITER = "alimiter=limit=0.95";

/**
 * `firstInputIndex` is where this stage's inputs land in the FFmpeg argument
 * list. It defaults to 1 because the design renderer has exactly one input
 * before it — the frame pipe. A composition has one input PER CLIP, so it
 * passes the real count; getting this wrong does not fail loudly, it silently
 * filters the wrong stream and ships a video with a clip's own audio where the
 * soundtrack should be.
 */
export async function buildAudioStage(
    audio?: ContentAudio,
    totalMs?: number,
    firstInputIndex = 1,
    clips: ClipAudio[] = []
): Promise<AudioStage> {
    const empty: AudioStage = { inputArgs: [], filter: null, outputArgs: ["-an"], tempFiles: [] };
    // Clip audio alone is a soundtrack. This used to test the soundtrack only,
    // which is why a content with one talking clip and no music rendered mute.
    if (!audio?.musicUrl && !audio?.voiceoverUrl && clips.length === 0) return empty;

    const musicVol = audio?.musicVolume ?? 0.7;
    const voiceVol = audio?.voiceoverVolume ?? 1;
    const duck = audio?.duckMusic !== false;

    const inputArgs: string[] = [];
    const tempFiles: string[] = [];
    const labels: { music?: number; voice?: number } = {};
    let index = firstInputIndex;

    try {
        if (audio?.musicUrl) {
            const path = await download(audio.musicUrl!, "music");
            tempFiles.push(path);
            // -stream_loop before the input: the bed repeats for the whole video
            // rather than ending halfway through a longer reel.
            inputArgs.push("-stream_loop", "-1", "-i", path);
            labels.music = index++;
        }
        if (audio?.voiceoverUrl) {
            const path = await download(audio.voiceoverUrl!, "voiceover");
            tempFiles.push(path);
            inputArgs.push("-i", path);
            labels.voice = index++;
        }
    } catch (err) {
        // A failed voiceover fetch must not strand the music file: nothing
        // downstream will ever see tempFiles if we throw from here.
        await Promise.all(tempFiles.map((p) => unlink(p).catch(() => undefined)));
        throw err;
    }

    // The bed is looped and infinite, so an out-fade has to be anchored to the
    // video's length rather than the clip's. Without a known length we fade in
    // only: guessing where the end is would cut the music early.
    const totalS = totalMs && totalMs > 0 ? totalMs / 1000 : 0;
    const musicFade =
        totalS > FADE_IN_S + FADE_OUT_S
            ? `,afade=t=in:st=0:d=${FADE_IN_S},afade=t=out:st=${(totalS - FADE_OUT_S).toFixed(3)}:d=${FADE_OUT_S}`
            : `,afade=t=in:st=0:d=${FADE_IN_S}`;

    const parts: string[] = [];

    // ── Foreground: what the viewer is meant to hear ────────────────────────
    // The clips' own sound and the voiceover. Both are finite and neither is
    // the bed, so they are assembled first and the music is placed under
    // whatever they turn out to be.
    const fg: string[] = [];

    if (clips.length) {
        const laid = clips.map((c, i) => {
            const chain = [atempoChain(c.speed ?? 1), "asetpts=PTS-STARTPTS"].filter(Boolean);
            // Every clip is seeked (`-ss`), so its audio arrives with the
            // source's timestamps. Reset them, then place the clip where the
            // frame-aligned plan put the picture: `adelay` is what keeps sound
            // and image together on a timeline of several clips.
            if (c.startMs > 0) chain.push(`adelay=${Math.round(c.startMs)}:all=1`);
            parts.push(`[${c.inputIndex}:a]${chain.join(",")}[cl${i}]`);
            return `[cl${i}]`;
        });
        if (laid.length === 1) {
            fg.push(laid[0]);
        } else {
            parts.push(`${laid.join("")}amix=inputs=${laid.length}:${MIX}[clips]`);
            fg.push("[clips]");
        }
    }

    if (labels.voice !== undefined) {
        parts.push(`[${labels.voice}:a]volume=${voiceVol}[v]`);
        fg.push("[v]");
    }

    let foreground: string | null = null;
    if (fg.length === 1) {
        foreground = fg[0];
    } else if (fg.length > 1) {
        parts.push(`${fg.join("")}amix=inputs=${fg.length}:${MIX}[fg]`);
        foreground = "[fg]";
    }

    // ── The bed, under it ───────────────────────────────────────────────────
    if (labels.music !== undefined && foreground) {
        parts.push(`[${labels.music}:a]volume=${musicVol}${musicFade}[m]`);
        // apad is load-bearing, not cosmetic. sidechaincompress reads its two
        // inputs in lockstep, and the music is infinite (-stream_loop -1) while
        // the foreground is not. When the voice hit EOF the compressor stopped
        // producing, the mix stalled behind it, FFmpeg's queues filled and it
        // stopped draining the frame pipe — hanging the render mid-encode with
        // no error. Padding keeps both sides alive; the output length is
        // bounded by -t on the encode instead.
        parts.push(`${foreground}apad[fgp]`);
        if (duck) {
            // One copy drives the compressor, the other is heard. The key is
            // now the whole foreground, so the bed gets out of the way of a
            // talking clip exactly as it does for a voiceover.
            parts.push(`[fgp]asplit=2[fgkey][fgout]`);
            parts.push(`[m][fgkey]sidechaincompress=threshold=0.05:ratio=8:release=250[mduck]`);
            parts.push(`[mduck][fgout]amix=inputs=2:duration=first:normalize=0:dropout_transition=0,${LIMITER}[aout]`);
        } else {
            parts.push(`[m][fgp]amix=inputs=2:duration=first:normalize=0:dropout_transition=0,${LIMITER}[aout]`);
        }
    } else if (labels.music !== undefined) {
        parts.push(`[${labels.music}:a]volume=${musicVol}${musicFade}[aout]`);
    } else if (foreground) {
        // Padded because -shortest used to truncate the whole video to the
        // voiceover when it was the only track; -t bounds the output instead.
        parts.push(`${foreground}apad,${LIMITER}[aout]`);
    }

    return {
        inputArgs,
        filter: parts.join(";"),
        outputArgs: ["-map", "[aout]", "-c:a", "aac", "-b:a", "128k"],
        tempFiles,
    };
}
