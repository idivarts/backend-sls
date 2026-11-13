import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { applyEdits, findElements, removeAttribute, setAttribute } from "./html-edit";
import { pathFromResolveJSON, readDeclarations } from "./media";

/**
 * The whole point of parsing rather than matching: a film's document carries
 * a GSAP timeline full of quotes and angle brackets, and a regex cannot tell
 * that from markup.
 */
const FILM = `<!doctype html>
<html>
  <head><style>.clip{opacity:0}</style></head>
  <body>
    <div id="root" data-composition-id="main" data-duration="9">
      <section id="scene-hook" data-scene="hook">
        <img id="hero" class="clip" data-media="image" data-media-intent="cold brew over ice" />
        <h1 id="hook-title">Hit the 2 PM wall?</h1>
      </section>
      <audio data-media="bgm" data-media-intent="upbeat lo-fi"></audio>
    </div>
    <script>
      // This is not markup: <img data-media="image" data-media-intent="trap">
      const s = "<section data-media='bgm'>";
      if (a < b && c > d) { gsap.to("#hero", { opacity: 1 }); }
    </script>
  </body>
</html>`;

describe("readDeclarations", () => {
    it("finds the real declarations and ignores the ones inside a script", () => {
        const found = readDeclarations(FILM);
        assert.deepEqual(
            found.map((d) => `${d.type}:${d.intent}`),
            ["image:cold brew over ice", "bgm:upbeat lo-fi"]
        );
    });

    // A paid call on a guess is the thing to avoid: no words, no search.
    it("skips a declaration with no intent and one with a type we do not know", () => {
        const html = `<div data-media="image"></div><div data-media="hologram" data-media-intent="x"></div>`;
        assert.equal(readDeclarations(html).length, 0);
    });
});

describe("editing in place", () => {
    it("writes src and clears the declaration, touching nothing else", () => {
        const els = findElements(FILM, (el) => el.attrs["data-media"] === "image");
        assert.equal(els.length, 1);
        const el = els[0];
        const out = applyEdits(FILM, [
            setAttribute(el, "src", "assets/media/hero.png"),
            removeAttribute(el, "data-media", FILM)!,
            removeAttribute(el, "data-media-intent", FILM)!,
        ]);
        // A new attribute is inserted immediately after the tag name, so it
        // leads; everything that was already on the tag keeps its spelling
        // and its order.
        assert.match(out, /<img src="assets\/media\/hero\.png" id="hero" class="clip"\s*\/>/);
        // Scoped to the img: the <audio> declaration and the decoy inside
        // the script are both meant to survive untouched.
        const imgTag = out.slice(out.indexOf("<img"), out.indexOf("/>") + 2);
        assert.ok(!imgTag.includes("data-media"), `declaration still on the img: ${imgTag}`);
        assert.ok(out.includes(`<audio data-media="bgm"`), "the audio declaration was disturbed");
        // Everything around it is byte-identical, the script included.
        assert.ok(out.includes(`const s = "<section data-media='bgm'>";`));
        assert.ok(out.includes("if (a < b && c > d)"));
        assert.ok(out.includes(`<h1 id="hook-title">Hit the 2 PM wall?</h1>`));
        assert.equal(out.split("\n").length, FILM.split("\n").length);
    });

    it("replaces an attribute that is already there rather than adding a second", () => {
        const html = `<img id="a" src="old.png" data-media="image" data-media-intent="x">`;
        const el = findElements(html, () => true).find((e) => e.tagName === "img")!;
        const out = applyEdits(html, [setAttribute(el, "src", "new.png")]);
        assert.ok(out.includes(`src="new.png"`));
        assert.equal(out.match(/src=/g)?.length, 1);
    });

    it("escapes a value so it cannot end the attribute", () => {
        const html = `<img id="a">`;
        const el = findElements(html, () => true).find((e) => e.tagName === "img")!;
        const out = applyEdits(html, [setAttribute(el, "src", `x" onerror="alert(1)`)]);
        assert.ok(!out.includes(`onerror="alert(1)"`));
        assert.ok(out.includes("&quot;"));
    });

    // Applied back to front, so each splice leaves earlier offsets valid.
    it("refuses overlapping edits instead of producing half of each", () => {
        assert.throws(
            () => applyEdits("0123456789", [{ start: 2, end: 6, text: "a" }, { start: 4, end: 8, text: "b" }]),
            /overlapping/
        );
    });

    it("applies several edits to one document correctly", () => {
        const out = applyEdits("0123456789", [
            { start: 0, end: 2, text: "AA" },
            { start: 8, end: 10, text: "ZZ" },
        ]);
        assert.equal(out, "AA234567ZZ");
    });
});

describe("pathFromResolveJSON", () => {
    it("takes the path the CLI reported", () => {
        assert.equal(
            pathFromResolveJSON(`{"path":"assets/media/track.mp3","type":"bgm"}`, "/tmp/p"),
            "assets/media/track.mp3"
        );
    });

    // The envelope is not documented as stable, so the likely keys are tried.
    it("accepts the other shapes the envelope has had", () => {
        assert.equal(pathFromResolveJSON(`{"file":"assets/a.png"}`, "/tmp/p"), "assets/a.png");
        assert.equal(pathFromResolveJSON(`{"asset":{"path":"assets/b.png"}}`, "/tmp/p"), "assets/b.png");
    });

    it("makes an absolute path project-relative", () => {
        assert.equal(pathFromResolveJSON(`{"path":"/tmp/p/assets/c.png"}`, "/tmp/p"), "assets/c.png");
    });

    // Progress lines before the envelope are normal; the last line is it.
    it("reads the last line when the CLI printed others first", () => {
        assert.equal(pathFromResolveJSON("searching…\n{\"path\":\"assets/d.png\"}", "/tmp/p"), "assets/d.png");
    });

    it("reports nothing rather than guessing", () => {
        assert.equal(pathFromResolveJSON("not json", "/tmp/p"), null);
        assert.equal(pathFromResolveJSON(`{"ok":true}`, "/tmp/p"), null);
    });
});
