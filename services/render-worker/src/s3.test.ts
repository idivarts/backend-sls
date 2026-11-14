import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { PLAYER_ARTEFACTS } from "./s3";

/**
 * The regression this guards. pkg/hfproject writes its player pages into
 * the SAME prefix as the project, and the old preview.html was
 * index.html with two script tags — so materialising the whole prefix handed
 * the CLI two root-level compositions and it refused the project:
 *
 *   Multiple root-level HTML files with data-composition-id:
 *   index.html, preview.html.
 *
 * which broke every preview and render for designs authored after the player
 * shipped.
 */
describe("player artefacts", () => {
    it("names exactly the two pages the player writes", () => {
        assert.deepEqual([...PLAYER_ARTEFACTS].sort(), ["player.html", "preview.html"]);
    });

    it("does not claim the project's own entry point", () => {
        assert.equal(PLAYER_ARTEFACTS.has("index.html"), false);
    });

    // Matched against the key RELATIVE TO THE PREFIX, so a scene that happens
    // to be called player.html is still a scene.
    it("is a root-level match, not a basename one", () => {
        assert.equal(PLAYER_ARTEFACTS.has("compositions/player.html"), false);
    });
});

describe("what downloadPrefix leaves behind", () => {
    // These are the two filters downloadPrefix applies, stated as data so
    // the rule is readable without running S3.
    const skipped = (relative: string) =>
        PLAYER_ARTEFACTS.has(relative) || relative === "revisions" || relative.startsWith("revisions/");

    it("skips the player pages, which are ours and not part of the film", () => {
        assert.equal(skipped("player.html"), true);
        assert.equal(skipped("preview.html"), true);
    });

    it("skips a design's past selves", () => {
        // A job pointed at the design ROOT lists every revision under it.
        assert.equal(skipped("revisions/7/index.html"), true);
        assert.equal(skipped("revisions/7/assets/voice/a.wav"), true);
        assert.equal(skipped("revisions"), true);
    });

    it("keeps the project itself, including names that merely start the same way", () => {
        for (const keep of [
            "index.html",
            "compositions/hook.html",
            "assets/fonts/inter-700.woff2",
            "revisions.html",
            "assets/revisions/note.txt",
        ]) {
            assert.equal(skipped(keep), false, `should have kept ${keep}`);
        }
    });
});
