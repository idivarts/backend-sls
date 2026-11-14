import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { applyEdits, findElements, removeAttribute, setAttribute } from "./html-edit";
import { stripPlayerRuntime } from "./hyperframes";
import {
    captionSpans,
    pathFromResolveJSON,
    readDeclarations,
    durationSeconds,
    startSeconds,
    transcriptPathFromJSON,
    voiceIdFromEntity,
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

describe("stripPlayerRuntime", () => {
    // index.html carries the player's runtime tag so the app can play the
    // film with no second copy of the document. It points at CloudFront,
    // and a render must work with no network — proved under --network none.
    it("takes out the player's runtime tag", () => {
        const html = `<html><head>
    <script src="https://cdn/design-player/0.9.39/runtime.js"></script>
    <script src="https://cdn/design-player/0.9.39/gsap-3.15.0.min.js"></script>
    </head><body>x</body></html>`;
        const out = stripPlayerRuntime(html);
        assert.ok(!out.includes("runtime.js"), out);
        // GSAP stays: vendorGsap rewrites it to a local file, and the film
        // needs it either way.
        assert.ok(out.includes("gsap-3.15.0.min.js"), out);
    });

    it("leaves a composition's own scripts alone", () => {
        const html = `<html><body><script src="scenes/runtime.js"></script><script>gsap.timeline()</script></body></html>`;
        assert.equal(stripPlayerRuntime(html), html);
    });
});

describe("voiceIdFromEntity", () => {
    it("passes a real HeyGen id through", () => {
        assert.equal(
            voiceIdFromEntity("83548de556df48ba8c09c42a57c51d85"),
            "83548de556df48ba8c09c42a57c51d85"
        );
        assert.equal(
            voiceIdFromEntity("  83548DE556DF48BA8C09C42A57C51D85 "),
            "83548de556df48ba8c09c42a57c51d85"
        );
    });

    it("drops anything that is not one, so the default voice speaks", () => {
        // The contract documents data-media-entity as the BRAND for a logo,
        // so a model writing data-media-entity="narrator" on a voice out of
        // symmetry must not turn a working voiceover into a 400.
        for (const e of ["narrator", "warm female voice", "", undefined, "83548de5"]) {
            assert.equal(voiceIdFromEntity(e), undefined, `expected ${String(e)} to be dropped`);
        }
    });
});

describe("wordsFromTranscribeJSON, against what the CLI really prints", () => {
    // Verbatim from `hyperframes transcribe <voiceover.wav> --json` run in
    // the render image on a real HeyGen voiceover. Note that the LAST line
    // has no words in it: reading only the final line finds nothing.
    const REAL = [
        '{"type":"progress","phase":"transcription","model":"base.en","status":"started","durationSeconds":2.693}',
        '{"type":"words","model":"base.en","words":[{"text":"Hit","start":0,"end":0.341},{"text":"the","start":0.341,"end":0.682},{"text":"2pm","start":0.682,"end":1.022},{"text":"wall,","start":1.022,"end":1.59},{"text":"not","start":1.59,"end":1.931},{"text":"anymore.","start":1.931,"end":2.84}],"through":2.84}',
        '{"type":"progress","phase":"transcription","model":"base.en","status":"completed"}',
        '{"ok":true,"engine":"whisper","model":"base.en","detectedLanguage":null,"wordCount":6,"durationSeconds":2.84,"speechOnsetSeconds":null,"transcriptPath":"/proj/transcript.json"}',
    ].join("\n");

    it("finds the words on the line that has them", () => {
        const words = wordsFromTranscribeJSON(REAL);
        assert.equal(words.length, 6);
        assert.deepEqual(words[0], { text: "Hit", start: 0, end: 0.341 });
        assert.deepEqual(words[5], { text: "anymore.", start: 1.931, end: 2.84 });
    });

    it("makes a caption track out of them", () => {
        const spans = captionSpans(wordsFromTranscribeJSON(REAL), 1.5);
        assert.match(spans, /^<span class="clip" data-start="1\.50" data-duration="0\.34">Hit<\/span>/);
        assert.ok(spans.includes('>anymore.</span>'));
        assert.equal(spans.split("<span").length - 1, 6);
    });

    it("is empty, not broken, when only progress and a summary arrive", () => {
        const noWords = [
            '{"type":"progress","phase":"transcription","status":"started"}',
            '{"ok":true,"wordCount":0}',
        ].join("\n");
        assert.deepEqual(wordsFromTranscribeJSON(noWords), []);
    });
});

describe("wordsFromTranscribeJSON, against the transcript FILE", () => {
    // Verbatim from the transcript.json the CLI wrote beside the project.
    // Pretty-printed, one array — not NDJSON — and its timings are tighter
    // than the streamed line's: the stream ends the last word at 2.84s on a
    // 2.69s clip, the file at 2.68s.
    const FILE = `[
  { "text": "Hit", "start": 0.32, "end": 0.35, "id": "w0" },
  { "text": "the", "start": 0.35, "end": 0.63, "id": "w1" },
  { "text": "anymore.", "start": 2.01, "end": 2.68, "id": "w5" }
]`;

    it("reads a pretty-printed array, not just NDJSON", () => {
        const words = wordsFromTranscribeJSON(FILE);
        assert.equal(words.length, 3);
        assert.deepEqual(words[0], { text: "Hit", start: 0.32, end: 0.35 });
    });

    it("gives a word the transcriber squeezed to nothing a readable minimum", () => {
        // 0.35 - 0.32 = 0.03s would be one frame at 30fps: a flicker.
        const spans = captionSpans(wordsFromTranscribeJSON(FILE));
        assert.match(spans, /data-start="0\.32" data-duration="0\.08">Hit</);
    });
});

describe("transcriptPathFromJSON", () => {
    it("takes the path off the summary line", () => {
        const out = [
            '{"type":"progress","phase":"transcription","status":"started"}',
            '{"ok":true,"wordCount":6,"transcriptPath":"/work/proj/transcript.json"}',
        ].join("\n");
        assert.equal(transcriptPathFromJSON(out), "/work/proj/transcript.json");
    });

    it("is null when no line names one", () => {
        assert.equal(transcriptPathFromJSON('{"ok":true,"wordCount":0}'), null);
        assert.equal(transcriptPathFromJSON("not json at all"), null);
    });
});

describe("startSeconds", () => {
    it("reads a data-start", () => {
        assert.equal(startSeconds({ attrs: { "data-start": "4" } }), 4);
        assert.equal(startSeconds({ attrs: { "data-start": "0" } }), 0);
        assert.equal(startSeconds({ attrs: { "data-start": "2.5" } }), 2.5);
    });

    it("says undefined — not zero — when there is nothing to read", () => {
        // The difference matters: a caption track falls back to the
        // container's data-start only when the AUDIO does not say, and "0"
        // is a thing the audio can say.
        assert.equal(startSeconds({ attrs: {} }), undefined);
        assert.equal(startSeconds(undefined), undefined);
        assert.equal(startSeconds({ attrs: { "data-start": "soon" } }), undefined);
    });
});

describe("durationSeconds", () => {
    it("reads a data-duration", () => {
        assert.equal(durationSeconds({ attrs: { "data-duration": "3" } }), 3);
        assert.equal(durationSeconds({ attrs: { "data-duration": "4.5" } }), 4.5);
    });

    it("is undefined when the element does not say", () => {
        assert.equal(durationSeconds({ attrs: {} }), undefined);
        assert.equal(durationSeconds(undefined), undefined);
        assert.equal(durationSeconds({ attrs: { "data-duration": "a while" } }), undefined);
    });
});

describe("a declaration that has already been served", () => {
    const resolved = `<audio id="vo" data-media="voice" data-media-intent="Hit the 2 PM wall?"
        data-media-resolved="assets/voice/a.wav" src="assets/voice/a.wav"></audio>`;

    it("is not resolved a second time", () => {
        // What stops the second run is data-media-resolved agreeing with
        // the src — not the declaration being stripped, which is how this
        // used to work and which threw away the words the voiceover says.
        assert.deepEqual(readDeclarations(resolved), []);
    });

    it("still carries the words, so a script can show them", () => {
        const el = findElements(resolved, (e) => e.attrs["id"] === "vo")[0];
        assert.equal(el.attrs["data-media-intent"], "Hit the 2 PM wall?");
    });

    it("is resolved again once the script editor clears what it resolved to", () => {
        // Which is exactly how a rewritten line asks to be spoken again.
        const rewritten = `<audio id="vo" data-media="voice" data-media-intent="Tired of sugar?"></audio>`;
        const found = readDeclarations(rewritten);
        assert.equal(found.length, 1);
        assert.equal(found[0].intent, "Tired of sugar?");
    });

    it("is resolved again when the src was changed out from under it", () => {
        const stale = `<audio id="vo" data-media="voice" data-media-intent="x"
            data-media-resolved="assets/voice/a.wav" src="assets/voice/b.wav"></audio>`;
        assert.equal(readDeclarations(stale).length, 1);
    });
});

describe("a caption track follows its voiceover", () => {
    // What marks a track done is the AUDIO it was built from, not the
    // absence of an attribute. The first version removed data-captions
    // once the words were in — so a rewritten line came back spoken
    // correctly with the old caption still on screen, the film saying one
    // thing and showing another.
    const built = `<div id="cap" class="caption-track" data-captions="vo"
        data-captions-for="assets/voice/a.wav" data-start="0" data-duration="3"><span
        class="clip" data-start="0.2" data-duration="0.3">Old</span></div>`;

    it("still says which voiceover it follows after it has been built", () => {
        const el = findElements(built, (e) => e.attrs["id"] === "cap")[0];
        assert.equal(el.attrs["data-captions"], "vo");
    });

    it("is up to date only while the audio it was built from is the current one", () => {
        const el = findElements(built, (e) => e.attrs["id"] === "cap")[0];
        const upToDate = (voicePath: string) => (el.attrs["data-captions-for"] || "").trim() === voicePath;
        assert.equal(upToDate("assets/voice/a.wav"), true);
        // A rewritten line gets a new file, named from a hash of its
        // words — so this disagrees and the track is transcribed again.
        assert.equal(upToDate("assets/voice/b.wav"), false);
    });
});
