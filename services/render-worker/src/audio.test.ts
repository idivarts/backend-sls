import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { buildAudioStage, type ClipAudio } from "./audio";

/**
 * The filter graph is the part of the audio stage worth asserting on: it is a
 * pure string, and a wrong one is a video that sounds wrong rather than an
 * exception anybody notices. Every case below is one a render actually hits.
 *
 * Only the clip-audio paths run here. Music and voiceover are downloaded
 * before the graph is built, so covering them would be a test of `fetch`.
 */

const clip = (over: Partial<ClipAudio> = {}): ClipAudio => ({
    inputIndex: 0,
    startMs: 0,
    ...over,
});

describe("a clip's own sound", () => {
    it("is a soundtrack on its own", async () => {
        // The reported bug: one generated clip that speaks, no music, no
        // voiceover. This returned `-an` and rendered the clip mute.
        const stage = await buildAudioStage(undefined, 6000, 1, [clip()]);
        assert.ok(stage.filter, "expected a filter graph, got none");
        assert.deepEqual(stage.outputArgs.slice(0, 2), ["-map", "[aout]"]);
        assert.ok(!stage.outputArgs.includes("-an"));
        assert.match(stage.filter!, /\[0:a\]/);
    });

    it("still renders silent when nothing has audio", async () => {
        const stage = await buildAudioStage(undefined, 6000, 1, []);
        assert.equal(stage.filter, null);
        assert.deepEqual(stage.outputArgs, ["-an"]);
    });

    it("lands where the picture lands", async () => {
        // A second clip starting at 6s must be delayed by 6s, or its sound
        // plays over the first clip.
        const stage = await buildAudioStage(undefined, 12000, 2, [
            clip({ inputIndex: 0, startMs: 0 }),
            clip({ inputIndex: 1, startMs: 6000 }),
        ]);
        assert.ok(!/\[0:a\][^;]*adelay/.test(stage.filter!), "the first clip must not be delayed");
        assert.match(stage.filter!, /\[1:a\][^;]*adelay=6000:all=1/);
    });

    it("names only the inputs it was given", async () => {
        // Naming [i:a] for an input with no audio track does not lose that
        // clip's sound, it fails the whole graph — so a mute clip must be
        // absent from it entirely.
        const stage = await buildAudioStage(undefined, 12000, 3, [clip({ inputIndex: 2, startMs: 8000 })]);
        assert.match(stage.filter!, /\[2:a\]/);
        assert.ok(!stage.filter!.includes("[0:a]"));
        assert.ok(!stage.filter!.includes("[1:a]"));
    });

    it("keeps pitch when the picture is sped up", async () => {
        const stage = await buildAudioStage(undefined, 3000, 1, [clip({ speed: 2 })]);
        assert.match(stage.filter!, /atempo=2\.0000/);
    });

    it("chains atempo past what one instance accepts", async () => {
        // atempo takes 0.5–2.0, so 4x is two stages. A single atempo=4 is
        // silently rejected by FFmpeg's option parser.
        const stage = await buildAudioStage(undefined, 3000, 1, [clip({ speed: 4 })]);
        assert.match(stage.filter!, /atempo=2\.0,atempo=2\.0000/);
    });

    it("mixes several clips without halving them", async () => {
        // amix normalizes by input count unless told not to. Sequential clips
        // never overlap, so normalizing would halve every one of them.
        const stage = await buildAudioStage(undefined, 12000, 2, [
            clip({ inputIndex: 0 }),
            clip({ inputIndex: 1, startMs: 6000 }),
        ]);
        assert.match(stage.filter!, /amix=inputs=2:[^[]*normalize=0/);
    });

    it("limits the mix so an honest sum cannot clip", async () => {
        const stage = await buildAudioStage(undefined, 6000, 1, [clip()]);
        assert.match(stage.filter!, /alimiter=limit=0\.95/);
    });
});
