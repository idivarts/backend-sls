import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { PLAIN_CROSSFADE, resolveTransition, __parseNames } from "./xfade";

/**
 * The parse and the fallback are the whole of this module's risk. A name this
 * FFmpeg does not have is not a degraded render — FFmpeg rejects the option and
 * the encode dies — so "what happens to a name we do not recognise" is the
 * assertion that matters.
 */

// Real output, trimmed: the shape is what the parse depends on.
const HELP = `Filter xfade
  Inputs:
     #0: main (video)
     #1: xfade (video)
  Outputs:
     #0: default (video)
xfade AVOptions:
   transition        <int>        ..FV....... set cross fade transition (from -1 to 45) (default fade)
     custom          -1           ..FV....... custom transition
     fade            0            ..FV....... fade transition
     wipeleft        1            ..FV....... wipe left transition
     wiperight       2            ..FV....... wipe right transition
     fadeblack       12           ..FV....... fadeblack transition
     hblur           35           ..FV....... hblur transition
   duration          <duration>   ..FV....... set cross fade duration (default 1)
   offset            <duration>   ..FV....... set cross fade start relative to first input stream (default 0)
   expr              <string>     ..FV....... set expression for custom transition
`;

describe("reading what this ffmpeg can draw", () => {
    it("finds the transition names", () => {
        const names = __parseNames(HELP);
        for (const n of ["fade", "wipeleft", "wiperight", "fadeblack", "hblur", "custom"]) {
            assert.ok(names.has(n), `expected ${n}`);
        }
    });

    it("does not mistake the filter's other options for transitions", () => {
        // `duration`, `offset` and `expr` are indented the same way. Anchoring
        // on the enum's numeric value is what separates them — without it the
        // probe would report "duration" as a transition, which is harmless
        // until something asks for it.
        const names = __parseNames(HELP);
        for (const n of ["duration", "offset", "expr"]) {
            assert.ok(!names.has(n), `${n} is an option, not a transition`);
        }
    });

    it("reports nothing from text it cannot read, rather than guessing", () => {
        assert.equal(__parseNames("ffmpeg: unrecognised option -h").size, 0);
    });
});

describe("choosing a transition", () => {
    const have = new Set(["fade", "wiperight", "fadeblack"]);

    it("uses the one asked for when this build has it", () => {
        assert.equal(resolveTransition("wiperight", have), "wiperight");
    });

    it("falls back to a plain cross-fade for one it does not", () => {
        // ⚠️ The important case. `hlwind` exists in newer FFmpeg and not in
        // Debian's; passing it through would make the render FAIL rather than
        // look different. The fallback is exactly the old behaviour, so the
        // worst case is what we are replacing.
        assert.equal(resolveTransition("hlwind", have), PLAIN_CROSSFADE);
    });

    it("falls back when nothing was asked for", () => {
        assert.equal(resolveTransition(undefined, have), PLAIN_CROSSFADE);
    });
});
