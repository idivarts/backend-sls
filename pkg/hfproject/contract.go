package hfproject

// The authoring contract, in one place.
//
// Two things write Hyperframes projects: the interactive chat tool
// (write_hyperframes_project) and the async authoring job. They must be held
// to the SAME rules, or a design written one way renders and the same design
// written the other way does not — and nobody would know which path produced
// the broken one.
//
// Every rule below was either taken from the `hyperframes` skill shipped in
// the npm package, or learned by rendering a real reel and reading what
// `check --json` complained about. The ones that cost a cycle to find are
// marked in the prose, because a rule with a reason survives an edit and a
// bare rule does not.
//
// ⚠️ ONE DOCUMENT, NOT A DIRECTORY OF COMPOSITIONS — the rule that cost the
// most to find.
//
// Hyperframes' own shape is a root that mounts one sub-composition per scene
// through data-composition-src, and that is what this contract asked for
// until 2026-11-13. It does not survive OUR render environment. Measured, on
// one real project, in the worker's own image:
//
//	hyperframes play      → correct: mounted, styled, scrubbable
//	hyperframes snapshot  → correct, at every timestamp sampled
//	hyperframes render    → WRONG: unstyled default-serif text in the
//	                        top-left, every frame identical, as though the
//	                        sub-compositions had never mounted
//
// The same project, the same container, the same browser. Ruled out along the
// way: GSAP outside <template> (dropped — a scene file's outside-template
// content is discarded), inside it (a cloned template's scripts do not
// execute), inlined whole, --shm-size=512m, network on, --no-best-effort,
// deleting /.dockerenv (which does flip forceScreenshot to false, but
// BeginFrame still cannot run without a GPU), and pointing the CLI at
// Playwright's chromium_headless_shell.
//
// Render is forced into screenshot capture inside Docker with no GPU, and
// sub-composition mounting does not survive that path. A SINGLE-DOCUMENT
// project renders correctly on exactly the same setup — styled, animated,
// 383 KB against the broken 62.5 KB — and passes `check` with ok=true.
//
// So the contract now asks for one document. Two bugs die with it: the
// mounting failure, and the GSAP-in-a-composition problem that produced the
// "Composition uses GSAP but no GSAP script is loaded" wall (a scene file is
// a separate context and there was no way to get GSAP into it from out here).
// One document has one <head>, one script tag, one timeline.
//
// If the render path is ever fixed — a GPU on the task, chrome-headless-shell
// for arm64, or an upstream fix — sub-compositions become available again and
// this decision is worth revisiting. Nothing else depends on it.

// AuthoringContract is the shape and the rules of a Hyperframes project.
const AuthoringContract = `A Hyperframes project is rendered to MP4 by seeking a paused GSAP timeline frame by frame.

WRITE ONE FILE: index.html. Everything — styles, scenes, the timeline — lives in it.
Do NOT create files under compositions/ and do NOT use data-composition-src: a
mounted sub-composition does not render in this environment, and the film comes
out as unstyled text.

index.html:
<html data-composition-variables='[{"id":"headline","type":"string","label":"Headline","default":"…"}]'>
  <head>
    <style>
      /* every style the film needs */
      .scene { position:absolute; inset:0; display:flex; flex-direction:column;
               align-items:center; justify-content:center; }
    </style>
  </head>
  <body style="margin:0;background:#0E0B09">
    <div id="root" data-composition-id="main" data-start="0" data-duration="12"
         data-width="1080" data-height="1920" style="position:absolute;inset:0;overflow:hidden">

      <section id="hook" class="scene" data-scene="hook" data-start="0" data-duration="4">
        <h1 id="hook-title" class="clip" data-start="0" data-duration="4" data-var-text="headline">…</h1>
      </section>

      <section id="benefits" class="scene" data-scene="benefits" data-start="4" data-duration="5">
        …
      </section>

    </div>
    <script>
      const tl = gsap.timeline({ paused: true });
      tl.fromTo("#hook-title", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.5 }, 0.1);
      window.__timelines["main"] = tl;
    </script>
  </body>
</html>

RULES — each is enforced, and breaking one fails the render:
1. ONE timeline, registered LAST, after every tween: window.__timelines["main"] = tl. The id on the root must be data-composition-id="main".
2. Every timed element INSIDE a scene carries class="clip", data-start and data-duration in SECONDS, and shows over [start, start+duration). A clip with data-start="0.15" leaves frame 0 empty — and frame 0 is the thumbnail social players use, so the first scene's main element starts at 0.
3. The film's length is the ROOT's data-duration. Overrun is cut; an early end holds the last frame.
4. Each scene is a <section> directly under the root carrying data-scene="<id>", data-start and data-duration — and NOT class="clip", because a clip that contains other elements is flagged by check. The runtime still hides a section outside its time range. These ids and times are what the app draws its timeline from, so every scene needs them, and scenes must not overlap in time.
5. Use fromTo, never from. The renderer re-seeks, and "from" records its start state once, so it drifts on seek-back.
6. Nothing may depend on real time: no Date.now(), no unseeded Math.random(), no fetch, no timers, no repeat:-1. Everything visible is a function of timeline time.
7. Never tween display, visibility or autoAlpha on a .clip — animate a child. Never put a CSS transform on an element GSAP transforms.
8. Size the root and every scene with inset: 0, never hard-coded pixels.
9. Ids must be unique across the document — prefix them with the scene (#hook-title, not #title).
10. Never add a <script src> for GSAP. The worker vendors GSAP and injects it into <head>; a tag you write is one we would immediately rewrite.
11. Fonts: only 'Noto Sans', 'Liberation Sans', 'DejaVu Sans' and the generic families resolve. Anything else needs an @font-face to a file in assets/.
12. Something must keep moving: a stretch over 3s with nothing changing fails the check. Mark a deliberate pause data-hold.
13. Keep content inside the middle 90% of the canvas and text inside the middle 80%. Phones crop the edges.
14. Scenes that are not on screen must not be visible: give every scene opacity:0 in CSS and let the timeline bring it in and take it out. Two scenes drawn at once is the most common way a film comes out unreadable.
15. Do not tween a property the browser snaps to whole pixels during layout — letterSpacing, width, height, margin, padding. check rejects them. Animate transform, opacity, scale, x and y instead.
MEDIA — DECLARE IT, DO NOT DRAW IT AND DO NOT LINK IT.
You cannot fetch anything, and a remote URL breaks a render that must work
with no network. When a scene wants a photograph, a video, music or a sound,
declare it and it is resolved into the project before the film is checked:

  <img id="hero-shot" class="clip" data-start="0" data-duration="3"
       data-media="image" data-media-intent="cold brew poured over ice, warm late-afternoon light" />

  <audio id="score" data-media="bgm" data-media-intent="upbeat lo-fi, no vocals, 90bpm"></audio>

VOICEOVER AND CAPTIONS. A voiceover is media like any other, and its intent
is the words to be spoken:

  <audio id="vo" data-media="voice" data-media-intent="Hit the 2 PM wall? Not any more."></audio>

For word-synced captions against it, add an empty container pointing at that
audio by id. It is filled with one timed span per word, from a real
transcript of the audio that was generated:

  <div id="captions" class="caption-track" data-captions="vo" data-start="0" data-duration="9"></div>

Style .caption-track and its spans yourself, from the brand tokens. Give the
container a sensible place on the frame; the words inherit whatever you set.
Ask for captions only when the film has a voiceover.

EVERY media element needs its own id. The renderer discovers media by id and
refuses a film without one: "<audio> has data-start but no id attribute ...
this audio will be SILENT in renders".

data-media is one of: image, icon, logo, bgm, sfx, voice.
data-media-intent is what it should be, in plain words — the more specific,
the better the result. For a logo, add data-media-entity="<the brand>".
Leave src off entirely; it is written for you.

Declare media only where the film genuinely needs a photograph or a sound.
Type, shape, colour and motion you can draw yourself are better drawn: they
cost nothing, they always match the brand, and they never fail to resolve.
`
