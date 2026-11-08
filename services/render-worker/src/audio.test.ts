import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { buildAudioStage, type ClipAudio } from "./audio";

/**
 * The filter graph is the part of the audio stage worth asserting on: it is a
 * pure string, and a wrong one is a video that sounds wrong rather than an
 * exception anybody notices. Every case below is one a render actually hits.
 *
 * The clip-audio paths need no network. The PLACEMENT paths do — a studio's
 * narration is downloaded before the graph is built — so `fetch` is stubbed for
 * those rather than left uncovered: where a placed voice lands in the graph is
 * the whole point of it, and it is expressed as a string.
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

describe("the clip's own level", () => {
    it("is applied when it is not full", async () => {
        const stage = await buildAudioStage(undefined, 6000, 1, [clip({ volume: 0.4 })]);
        assert.match(stage.filter!, /volume=0\.4/);
    });

    it("is left out of the chain when it is full", async () => {
        // Not `volume=1`, which is a filter that does nothing.
        const stage = await buildAudioStage(undefined, 6000, 1, [clip()]);
        assert.ok(!/\[0:a\][^;]*volume=/.test(stage.filter!));
    });
});

// ───────────────────────── Placed sound (a studio's) ───────────────────────

/** A placement's bytes never reach FFmpeg in a test, only its position does. */
async function withStubbedFetch<T>(run: () => Promise<T>): Promise<T> {
    const real = globalThis.fetch;
    globalThis.fetch = (async () =>
        new Response(new Uint8Array([0]), { status: 200 })) as typeof globalThis.fetch;
    try {
        return await run();
    } finally {
        globalThis.fetch = real;
    }
}

const cleanup = async (stage: { tempFiles: string[] }) => {
    const { unlink } = await import("node:fs/promises");
    await Promise.all(stage.tempFiles.map((f) => unlink(f).catch(() => undefined)));
};

describe("placed sound", () => {
    it("delays a placement to its own start, in ms", async () => {
        // The studio's voiceover for scene two. Before this existed the studio
        // path handed the renderer `content.audio` — a document the studio never
        // writes — so there was nothing to delay and nothing to hear.
        const stage = await withStubbedFetch(() =>
            buildAudioStage(
                { placements: [{ url: "https://cdn/vo.mp3", startMs: 2600 }] },
                6000,
                1,
                []
            )
        );
        assert.ok(stage.filter, "expected a filter graph");
        assert.match(stage.filter!, /adelay=2600:all=1/);
        assert.ok(!stage.outputArgs.includes("-an"));
        await cleanup(stage);
    });

    it("does not delay one that starts at zero", async () => {
        const stage = await withStubbedFetch(() =>
            buildAudioStage({ placements: [{ url: "https://cdn/vo.mp3", startMs: 0 }] }, 6000, 1, [])
        );
        assert.ok(!/adelay/.test(stage.filter!));
        await cleanup(stage);
    });

    it("numbers its inputs after the clips, not from one", async () => {
        // An index off by the clip count filters a VIDEO stream as audio. With
        // three clips ahead of it, the single placement is input 3.
        const stage = await withStubbedFetch(() =>
            buildAudioStage({ placements: [{ url: "https://cdn/vo.mp3", startMs: 10 }] }, 6000, 3, [])
        );
        assert.match(stage.filter!, /\[3:a\]/);
        await cleanup(stage);
    });

    it("keeps the bed and every placement in one mix, with the bed ducked", async () => {
        // The exit condition for this phase, as a graph: a bed, two spoken
        // lines, all audible, the bed pushed under them.
        const stage = await withStubbedFetch(() =>
            buildAudioStage(
                {
                    musicUrl: "https://cdn/bed.mp3",
                    placements: [
                        { url: "https://cdn/a.mp3", startMs: 0 },
                        { url: "https://cdn/b.mp3", startMs: 3000 },
                    ],
                },
                9000,
                1,
                []
            )
        );
        const f = stage.filter!;
        // Both lines are in the foreground mix...
        assert.match(f, /\[pl0\]\[pl1\]amix=inputs=2/);
        // ...the bed is keyed off that whole foreground...
        assert.match(f, /sidechaincompress/);
        // ...and the voice is mixed back in rather than being left as the key,
        // which is the bug the file header warns about.
        assert.match(f, /\[mduck\]\[fgout\]amix/);
        assert.match(f, /alimiter=limit=0\.95/);
        await cleanup(stage);
    });

    it("stops ducking when the bed's lane says not to", async () => {
        const stage = await withStubbedFetch(() =>
            buildAudioStage(
                {
                    musicUrl: "https://cdn/bed.mp3",
                    duckMusic: false,
                    placements: [{ url: "https://cdn/a.mp3", startMs: 0 }],
                },
                9000,
                1,
                []
            )
        );
        assert.ok(!/sidechaincompress/.test(stage.filter!));
        await cleanup(stage);
    });

    it("pins every placement to one sample format so a mono voice is not one-eared", async () => {
        const stage = await withStubbedFetch(() =>
            buildAudioStage({ placements: [{ url: "https://cdn/vo.mp3", startMs: 0 }] }, 6000, 1, [])
        );
        assert.match(stage.filter!, /channel_layouts=stereo/);
        await cleanup(stage);
    });

    it("trims on the input rather than in the graph", async () => {
        const stage = await withStubbedFetch(() =>
            buildAudioStage(
                {
                    placements: [
                        { url: "https://cdn/vo.mp3", startMs: 0, trimInMs: 500, trimOutMs: 2000 },
                    ],
                },
                6000,
                1,
                []
            )
        );
        // -ss and -t ahead of the -i they belong to: FFmpeg seeks instead of
        // decoding and throwing away.
        const i = stage.inputArgs.indexOf("-i");
        assert.deepEqual(stage.inputArgs.slice(0, i), ["-ss", "0.500", "-t", "1.500"]);
        assert.ok(!/atrim/.test(stage.filter!));
        await cleanup(stage);
    });

    it("fades out only when it knows where the sound ends", async () => {
        // No trim and no duration: there is no end to anchor a fade to, and
        // inventing one cuts the voice off mid-word.
        const unknown = await withStubbedFetch(() =>
            buildAudioStage(
                { placements: [{ url: "https://cdn/vo.mp3", startMs: 0, fadeOutMs: 300 }] },
                6000,
                1,
                []
            )
        );
        assert.ok(!/afade=t=out/.test(unknown.filter!));
        await cleanup(unknown);

        const known = await withStubbedFetch(() =>
            buildAudioStage(
                {
                    placements: [
                        { url: "https://cdn/vo.mp3", startMs: 0, fadeOutMs: 300, durationMs: 2000 },
                    ],
                },
                6000,
                1,
                []
            )
        );
        assert.match(known.filter!, /afade=t=out:st=1\.700:d=0\.300/);
        await cleanup(known);
    });

    it("never lets the media row's duration truncate the sound", async () => {
        // `-t` HARD TRUNCATES, and nothing guarantees a row's durationMs matches
        // its file — mediastore.Put does not even write the field for generated
        // audio. A row reading short would have cut the last word off every
        // export. The duration may anchor a fade; it may not cut.
        const stage = await withStubbedFetch(() =>
            buildAudioStage(
                { placements: [{ url: "https://cdn/vo.mp3", startMs: 0, durationMs: 1800 }] },
                6000,
                1,
                []
            )
        );
        assert.ok(!stage.inputArgs.includes("-t"), `unexpected -t in ${stage.inputArgs.join(" ")}`);
        await cleanup(stage);
    });

    it("is silent only when there is genuinely nothing to play", async () => {
        const stage = await buildAudioStage({ placements: [] }, 6000, 1, []);
        assert.equal(stage.filter, null);
        assert.deepEqual(stage.outputArgs, ["-an"]);
    });
});
