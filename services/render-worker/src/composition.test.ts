import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { measureComposition, type Composition } from "../../../shared-libs/design/composition-runtime";
import { __test, type Segment } from "./composition";

const { buildVideoGraph, normalize, kenBurns, planClips } = __test;

const W = 1080;
const H = 1920;
const FPS = 30;

/**
 * Build the segments the final pass would see for a composition, without
 * rendering anything. The graph is the part of the composition renderer worth
 * asserting on: it is pure, and a wrong filter string is a broken video rather
 * than an exception.
 */
function segmentsFor(composition: Composition): Segment[] {
    const measurement = measureComposition(composition);
    const { plans } = planClips(measurement);
    return measurement.clips.map((measured) => ({
        measured,
        plan: plans[measured.index],
        inputArgs: ["-i", `clip${measured.index}.mp4`],
        filter: "",
    }));
}

function comp(
    clips: { ms: number; fade?: number }[],
): Composition {
    return {
        width: W,
        height: H,
        fps: FPS,
        tracks: [
            {
                kind: "video",
                clips: clips.map((c, i) => ({
                    id: `c${i}`,
                    durationMs: c.ms,
                    ...(c.fade ? { transitionIn: { kind: "fade" as const, ms: c.fade } } : {}),
                    source: { kind: "design" as const, revisionId: `rev${i}` },
                })),
            },
        ],
    };
}

describe("normalize", () => {
    const chain = normalize(W, H, FPS);

    it("fits and pads rather than cropping", () => {
        // A clip shot in the wrong aspect ratio should look letterboxed, not
        // silently reframed into something the user never saw.
        assert.match(chain, /scale=1080:1920:force_original_aspect_ratio=decrease/);
        assert.match(chain, /pad=1080:1920/);
    });

    it("pins the rate, pixel format, aspect and timebase", () => {
        // Every one of these is load-bearing: concat and xfade both refuse
        // inputs that disagree on any of them, and a missing settb is the
        // difference between a cross-fade and "timebase do not match".
        for (const part of ["fps=30", "format=yuv420p", "setsar=1", "settb=AVTB"]) {
            assert.ok(chain.includes(part), `normalize should contain ${part}, got ${chain}`);
        }
    });

    it("keeps the source size when asked for no size", () => {
        const sizeless = normalize(0, 0, FPS);
        assert.ok(!sizeless.includes("scale="), sizeless);
        assert.ok(!sizeless.includes("pad="), sizeless);
        assert.ok(sizeless.includes("settb=AVTB"), sizeless);
    });
});

describe("kenBurns", () => {
    it("is nothing at all when the still should sit there", () => {
        assert.equal(kenBurns(undefined, 90, W, H), "");
        assert.equal(kenBurns("none", 90, W, H), "");
    });

    // zoompan's default output size is hd720. Leaving `s` out renders a
    // portrait reel at 1280x720 and the pad downstream letterboxes it into a
    // stripe — silently, with no error anywhere.
    it("always states its output size", () => {
        const chain = kenBurns("in", 90, W, H);
        assert.ok(chain.includes(`s=${W}x${H}`), chain);
    });

    it("crops to cover first, so the zoom has pixels to move into", () => {
        const chain = kenBurns("in", 90, W, H);
        assert.match(chain, /force_original_aspect_ratio=increase/);
        assert.match(chain, /crop=1080:1920/);
        assert.ok(chain.indexOf("crop=") < chain.indexOf("zoompan="), chain);
    });

    it("pushes in and pulls out", () => {
        assert.match(kenBurns("in", 30, W, H), /z='1\+0\.15\*on\/30'/);
        assert.match(kenBurns("out", 30, W, H), /z='1\.15-0\.15\*on\/30'/);
    });
});

describe("buildVideoGraph", () => {
    it("labels a single clip and ends on [outv]", () => {
        const graph = buildVideoGraph(segmentsFor(comp([{ ms: 4000 }])), W, H, FPS);
        assert.match(graph, /^\[0:v\]/);
        assert.ok(graph.endsWith("[v0]null[outv]"), graph);
        // Nothing to join, so no compositing filter should appear at all.
        assert.ok(!graph.includes("xfade"), graph);
        assert.ok(!graph.includes("concat"), graph);
    });

    it("concatenates a hard cut", () => {
        const graph = buildVideoGraph(segmentsFor(comp([{ ms: 2000 }, { ms: 2000 }])), W, H, FPS);
        assert.ok(graph.includes("[v0][v1]concat=n=2:v=1:a=0[x1]"), graph);
        assert.ok(!graph.includes("xfade"), graph);
    });

    it("cross-fades a transition, at the arriving clip's own start", () => {
        const graph = buildVideoGraph(
            segmentsFor(comp([{ ms: 3000 }, { ms: 3000, fade: 500 }])),
            W,
            H,
            FPS
        );
        assert.ok(
            graph.includes("[v0][v1]xfade=transition=fade:duration=0.500:offset=2.500[x1]"),
            graph
        );
    });

    // xfade's offset is measured from the start of its FIRST input, which after
    // chaining is the composition's own start. So each offset is the measured
    // start of the clip arriving — NOT a running total, which is the easy bug:
    // it would put every join after the first in the wrong place.
    it("uses composition-absolute offsets down a chain", () => {
        const composition = comp([
            { ms: 3000 },
            { ms: 3000, fade: 500 },
            { ms: 3000, fade: 1000 },
        ]);
        const measured = measureComposition(composition);
        assert.deepEqual(
            measured.clips.map((c) => c.startMs),
            [0, 2500, 4500]
        );

        const graph = buildVideoGraph(segmentsFor(composition), W, H, FPS);
        assert.ok(graph.includes("offset=2.500[x1]"), graph);
        assert.ok(graph.includes("[x1][v2]xfade=transition=fade:duration=1.000:offset=4.500[x2]"), graph);
        assert.ok(graph.endsWith("[x2]null[outv]"), graph);
    });

    it("mixes cuts and fades in one chain", () => {
        const graph = buildVideoGraph(
            segmentsFor(comp([{ ms: 2000 }, { ms: 2000 }, { ms: 2000, fade: 400 }])),
            W,
            H,
            FPS
        );
        assert.ok(graph.includes("[v0][v1]concat=n=2:v=1:a=0[x1]"), graph);
        assert.ok(graph.includes("[x1][v2]xfade=transition=fade:duration=0.400:offset=3.600[x2]"), graph);
    });

    it("normalizes every input exactly once", () => {
        const segments = segmentsFor(comp([{ ms: 1000 }, { ms: 1000 }]));
        const graph = buildVideoGraph(segments, W, H, FPS);
        // A second normalize pass per input would be dead work on every frame.
        const settbCount = graph.split("settb=AVTB").length - 1;
        assert.equal(settbCount, segments.length, graph);
    });

    it("keeps a clip's own filter ahead of the normalization", () => {
        // setpts for speed, or a zoompan, has to run before the frame is
        // resized and re-timed, not after.
        const segments = segmentsFor(comp([{ ms: 1000 }]));
        segments[0].filter = "setpts=PTS/2";
        const graph = buildVideoGraph(segments, W, H, FPS);
        assert.ok(graph.indexOf("setpts=PTS/2") < graph.indexOf("scale="), graph);
    });
});

describe("planClips", () => {
    const planFor = (clips: { ms: number; fade?: number }[]) =>
        planClips(measureComposition(comp(clips)));

    it("snaps a clip to a whole number of frames", () => {
        // 1001ms at 30fps is 30.03 frames. The encode can only write 30, so the
        // plan has to say 30 — not 30.03 rounded somewhere else later.
        const { plans, totalFrames } = planFor([{ ms: 1001 }]);
        assert.equal(plans[0].frames, 30);
        assert.equal(totalFrames, 30);
    });

    // The reason this function exists. Chained, xfade needs its first input to
    // run exactly up to offset+duration. In frame terms that is
    // startFrames[i] + overlapFrames[i] == end of the accumulated chain. If the
    // plan and the encode disagreed by even one frame, the cross-fade would have
    // nothing to fade from at its tail.
    it("keeps every xfade's boundary condition exact, for awkward lengths", () => {
        const lengths = [1001, 1017, 2333, 999, 3001, 4567];
        const fades = [333, 401, 117, 1234];
        for (const a of lengths) {
            for (const b of lengths) {
                for (const fade of fades) {
                    const { plans } = planFor([{ ms: a }, { ms: b, fade }]);
                    const chainEndBeforeJoin = plans[0].startFrames + plans[0].frames;
                    assert.equal(
                        plans[1].startFrames + plans[1].overlapFrames,
                        chainEndBeforeJoin,
                        `a=${a} b=${b} fade=${fade}`
                    );
                    // And the runtime's invariant still holds in frame space.
                    assert.ok(plans[1].startFrames > plans[0].startFrames);
                }
            }
        }
    });

    it("holds the boundary down a three-clip chain", () => {
        const { plans, totalFrames } = planFor([
            { ms: 1001 },
            { ms: 2333, fade: 401 },
            { ms: 999, fade: 117 },
        ]);
        let end = plans[0].frames;
        for (let i = 1; i < plans.length; i++) {
            assert.equal(plans[i].startFrames + plans[i].overlapFrames, end, `join ${i}`);
            end = plans[i].startFrames + plans[i].frames;
        }
        assert.equal(totalFrames, end);
    });

    it("never gives a clip zero frames", () => {
        // A 10ms clip is less than one frame at 30fps. Zero frames is an input
        // FFmpeg cannot read at all, so it gets one.
        const { plans } = planFor([{ ms: 10 }]);
        assert.equal(plans[0].frames, 1);
    });

    it("re-clamps the overlap in frame space", () => {
        // Both sides are rounded independently, so an overlap clamped to half in
        // milliseconds can land back over half in frames — and that clamp is the
        // only thing keeping offsets increasing.
        const { plans } = planFor([{ ms: 1001 }, { ms: 1001, fade: 500 }]);
        assert.ok(plans[1].overlapFrames <= Math.floor(plans[0].frames / 2), JSON.stringify(plans));
        assert.ok(plans[1].startFrames > 0);
    });
});
