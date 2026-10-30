/**
 * Build the FFmpeg audio stage: a looping music bed, ducked under a voiceover,
 * mixed to one stereo track.
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
    firstInputIndex = 1
): Promise<AudioStage> {
    const empty: AudioStage = { inputArgs: [], filter: null, outputArgs: ["-an"], tempFiles: [] };
    if (!audio || (!audio.musicUrl && !audio.voiceoverUrl)) return empty;

    const musicVol = audio.musicVolume ?? 0.7;
    const voiceVol = audio.voiceoverVolume ?? 1;
    const duck = audio.duckMusic !== false;

    const inputArgs: string[] = [];
    const tempFiles: string[] = [];
    const labels: { music?: number; voice?: number } = {};
    let index = firstInputIndex;

    try {
        if (audio.musicUrl) {
            const path = await download(audio.musicUrl, "music");
            tempFiles.push(path);
            // -stream_loop before the input: the bed repeats for the whole video
            // rather than ending halfway through a longer reel.
            inputArgs.push("-stream_loop", "-1", "-i", path);
            labels.music = index++;
        }
        if (audio.voiceoverUrl) {
            const path = await download(audio.voiceoverUrl, "voiceover");
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
    if (labels.music !== undefined && labels.voice !== undefined) {
        parts.push(`[${labels.music}:a]volume=${musicVol}${musicFade}[m]`);
        // apad is load-bearing, not cosmetic. sidechaincompress reads its two
        // inputs in lockstep, and the music is infinite (-stream_loop -1) while
        // a voiceover is not. When the voice hit EOF the compressor stopped
        // producing, the mix stalled behind it, FFmpeg's queues filled and it
        // stopped draining the frame pipe — hanging the render mid-encode with
        // no error. Padding the voice with silence keeps both sides alive; the
        // output length is bounded by -t on the encode instead.
        parts.push(`[${labels.voice}:a]volume=${voiceVol},apad[v]`);
        if (duck) {
            // One copy of the voice drives the compressor, the other is heard.
            parts.push(`[v]asplit=2[vkey][vout]`);
            parts.push(`[m][vkey]sidechaincompress=threshold=0.05:ratio=8:release=250[mduck]`);
            parts.push(`[mduck][vout]amix=inputs=2:duration=first:dropout_transition=0[aout]`);
        } else {
            parts.push(`[m][v]amix=inputs=2:duration=first:dropout_transition=0[aout]`);
        }
    } else if (labels.music !== undefined) {
        parts.push(`[${labels.music}:a]volume=${musicVol}${musicFade}[aout]`);
    } else if (labels.voice !== undefined) {
        // Padded for the same reason as above, and because -shortest used to
        // truncate the whole video to the voiceover when it was the only track.
        parts.push(`[${labels.voice}:a]volume=${voiceVol},apad[aout]`);
    }

    return {
        inputArgs,
        filter: parts.join(";"),
        outputArgs: ["-map", "[aout]", "-c:a", "aac", "-b:a", "128k"],
        tempFiles,
    };
}
