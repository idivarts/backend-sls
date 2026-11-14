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
