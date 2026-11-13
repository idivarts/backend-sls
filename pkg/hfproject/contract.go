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
// Deliberately absent: any instruction to add a GSAP <script> tag. Their own
// scaffold emits a jsdelivr one, which would make every render depend on a
// CDN; the worker vendors GSAP and rewrites any CDN tag it finds. Asking a
// model for one would be asking it to write something we immediately undo.

// AuthoringContract is the shape and the rules of a Hyperframes project.
const AuthoringContract = `A Hyperframes project is a DIRECTORY of HTML files, rendered to MP4 by seeking a paused timeline frame by frame.

index.html is the root composition: it declares the film's variables and mounts one host div per scene. Each scene is its own file under compositions/.

ROOT (index.html):
<html data-composition-variables='[{"id":"headline","type":"string","label":"Headline","default":"…"}]'>
  <body style="margin:0">
    <div id="root" data-composition-id="main" data-start="0" data-duration="12"
         data-width="1080" data-height="1920">
      <div id="hook" data-composition-id="hook" data-composition-src="compositions/hook.html"
           data-start="0" data-duration="4" data-track-index="1"
           data-track-kind="graphics" data-width="1080" data-height="1920"></div>
    </div>
    <script>window.__timelines["main"] = gsap.timeline({ paused: true });</script>
  </body>
</html>

SCENE (compositions/hook.html) — everything inside <template>; styles and scripts outside it are DROPPED:
<template>
  <style>#hook { position:absolute; inset:0; background:#0E0B09; }</style>
  <div id="hook" data-composition-id="hook" data-width="1080" data-height="1920">
    <h1 id="hook-title" class="clip" data-start="0" data-duration="4" data-var-text="headline">…</h1>
  </div>
  <script>
    const tl = gsap.timeline({ paused: true });
    tl.fromTo("#hook-title", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.5, ease: "power4.out" }, 0);
    window.__timelines["hook"] = tl;
  </script>
</template>

RULES — each is enforced, and breaking one fails the render:
1. The host id, the scene root id and the timeline key must be THE SAME STRING. A mismatch makes every render wait 45s per scene and then show static frames.
2. Every timed element carries class="clip", data-start and data-duration in SECONDS, and shows over [start, start+duration). A clip with data-start="0.15" leaves frame 0 empty — and frame 0 is the thumbnail social players use, so the first scene's main element starts at 0.
3. The film's length is the ROOT's data-duration. Overrun is cut; an early end holds the last frame.
4. Each composition builds ONE paused timeline and registers it AFTER its tweens: window.__timelines["<data-composition-id>"] = tl.
5. Use fromTo, never from. The host re-seeks a scene each time it shows, and "from" records its start state once, so it drifts on seek-back.
6. Nothing may depend on real time: no Date.now(), no unseeded Math.random(), no fetch, no timers, no repeat:-1. Everything visible is a function of timeline time.
7. Never tween display, visibility or autoAlpha on a .clip — animate a child. Never put a CSS transform on an element GSAP transforms.
8. Size a scene root with inset: 0, never hard-coded pixels.
9. Ids must be unique across the whole assembled page — prefix them with the scene (#hook-title, not #title). A scene's timeline cannot reach outside its own file.
10. Never add a <script src> for GSAP. gsap is already available.
11. Fonts: only 'Noto Sans', 'Liberation Sans', 'DejaVu Sans' and the generic families resolve. Anything else needs an @font-face to a file in assets/.
12. Something must keep moving: a stretch over 3s with nothing changing fails the check. Mark a deliberate pause data-hold.
13. Keep content inside the middle 90% of the canvas and text inside the middle 80%. Phones crop the edges.

MOTION. Give a scene one clear action rather than several small ones, and land it on the thing that carries the meaning rather than on a container sliding in. 0.4–0.6s with ease "power3.out" or "power4.out" reads as deliberate; under 0.2s reads as a glitch.

SIZING. Font sizes are px against the canvas you were given and do NOT scale if it changes. For 1080x1920 a headline is roughly 100–140px and body text 56–84px; for 1920x1080, 110–150px and 60–90px.

VARIABLES. Anything a user may want to change later — copy, a brand colour, a swappable image — is declared in data-composition-variables and bound with data-var-text="id", data-var-src="id" or var(--id) in CSS. This is the only part of a film that can be edited on a phone, so the copy belongs there.`
