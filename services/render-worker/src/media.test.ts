import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { applyEdits, findElements, removeAttribute, setAttribute } from "./html-edit";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
    captionSpans,
    pathFromResolveJSON,
    readDeclarations,
    syncPlayerPage,
    wordsFromTranscribeJSON,
} from "./media";

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

describe("captions", () => {
    it("reads words from the transcribe envelope, wherever they sit", () => {
        assert.equal(wordsFromTranscribeJSON(`[{"text":"Hi","start":0,"end":0.4}]`).length, 1);
        assert.equal(wordsFromTranscribeJSON(`{"words":[{"text":"Hi","start":0,"end":0.4}]}`).length, 1);
        assert.equal(wordsFromTranscribeJSON(`{"result":{"words":[{"text":"Hi","start":0,"end":0.4}]}}`).length, 1);
    });

    it("returns nothing rather than guessing", () => {
        assert.deepEqual(wordsFromTranscribeJSON("not json"), []);
        assert.deepEqual(wordsFromTranscribeJSON(`{"ok":true}`), []);
        // A malformed word is dropped, not coerced into a span at time NaN.
        assert.deepEqual(wordsFromTranscribeJSON(`{"words":[{"text":"Hi"}]}`), []);
    });

    // The runtime already times any .clip with data-start/data-duration —
    // that is how every scene in these films works — so a caption needs no
    // component and no script of its own.
    it("writes one timed span per word", () => {
        const out = captionSpans([
            { text: "Hit", start: 0, end: 0.4 },
            { text: "the", start: 0.4, end: 0.55 },
        ]);
        assert.match(out, /<span class="clip" data-start="0\.00" data-duration="0\.40">Hit<\/span>/);
        assert.match(out, /<span class="clip" data-start="0\.40" data-duration="0\.15">the<\/span>/);
    });

    // A transcript starts at zero; a voiceover four seconds into the film
    // does not.
    it("shifts the transcript onto the film's clock", () => {
        const out = captionSpans([{ text: "Hi", start: 0, end: 0.5 }], 4);
        assert.match(out, /data-start="4\.00"/);
    });

    it("gives a zero-width word something to be seen in", () => {
        const out = captionSpans([{ text: "x", start: 1, end: 1 }]);
        assert.match(out, /data-duration="0\.08"/);
    });

    it("escapes a word so it cannot become markup", () => {
        assert.ok(captionSpans([{ text: "<b>", start: 0, end: 1 }]).includes("&lt;b&gt;"));
    });
});

describe("syncPlayerPage", () => {
    // preview.html is what the player loads. It is written once, at
    // authoring time, from an index.html that has no media in it yet — so
    // without this the rendered film has the photograph and the film the
    // user watches does not.
    it("rebuilds the player page from the current index, keeping its script tags", async () => {
        const dir = await mkdtemp(join(tmpdir(), "hfsync-"));
        await writeFile(
            join(dir, "preview.html"),
            `<html><head>
    <script src="https://cdn/design-player/0.9.39/runtime.js"></script>
    <script src="https://cdn/design-player/0.9.39/gsap-3.15.0.min.js"></script>
    </head><body><img data-media="image" data-media-intent="old"></body></html>`,
            "utf8"
        );
        await writeFile(
            join(dir, "index.html"),
            `<html><head><title>t</title></head><body><img src=".media/images/image_001.png"></body></html>`,
            "utf8"
        );

        assert.equal(await syncPlayerPage(dir), true);
        const out = await readFile(join(dir, "preview.html"), "utf8");
        // The resolved media is there now…
        assert.ok(out.includes('src=".media/images/image_001.png"'), out);
        // …the stale declaration is gone…
        assert.ok(!out.includes("data-media-intent"), out);
        // …and both player scripts survived, so the film still plays.
        assert.ok(out.includes("design-player/0.9.39/runtime.js"), out);
        assert.ok(out.includes("design-player/0.9.39/gsap-3.15.0.min.js"), out);
    });

    it("leaves a project alone when it has no player page", async () => {
        const dir = await mkdtemp(join(tmpdir(), "hfsync-"));
        await writeFile(join(dir, "index.html"), "<html><body>x</body></html>", "utf8");
        assert.equal(await syncPlayerPage(dir), false);
    });
});
