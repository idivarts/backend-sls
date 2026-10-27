package ai

// The design craft contract — the standards a Trendly design is held to.
//
// Everything the model knew about making a design used to live in the
// generate_design tool description, which is the wrong place for it twice over:
// a tool description is read as "how to call this function", and it has to stay
// short enough not to crowd out the other tools. So the mechanics (structure,
// what the renderer supports, how motion is seeked) stayed there, and the CRAFT
// moved here, into the system prompt for the design surface.
//
// The rules below are the ones whose absence is visible in the output. Each is
// phrased as a number or a decision rather than an adjective, because "make it
// balanced" changes nothing and "keep text within 8% of the edge" does.
//
// This sits alongside the brand's own Design System block (design_system.go),
// which carries palette, fonts, voice and per-brand rules. Where the two
// disagree, the brand wins — this is the floor, not the identity.
const designCraftInstructions = "\n\n--- DESIGN STANDARDS (the bar every design you make is held to) ---\n" +
	"These are defaults, not overrides: wherever this brand's Design System above " +
	"states a specific rule — a type scale, a palette, a layout convention — that " +
	"rule wins and these fill in the rest.\n\n" +

	"LAYOUT\n" +
	"- Keep every element inside a margin of at least 8% of the slide's shorter edge. " +
	"Social platforms crop, and a frame with content at the bezel reads as a mistake.\n" +
	"- For a 9:16 story or reel, keep nothing important in the top 14% or bottom 20%: " +
	"those bands sit under the platform's own UI.\n" +
	"- Pick ONE focal point per slide and let it dominate. Two elements competing at " +
	"the same weight is the most common way a design reads as amateur.\n" +
	"- Optical centering, not mathematical: text centered by its box looks low, because " +
	"the cap height sits above the baseline. Nudge up by about 2% of the height.\n" +
	"- Align everything to a small set of edges. Three left edges within a few pixels " +
	"of each other look broken; one shared edge looks deliberate.\n" +

	"\nTYPOGRAPHY\n" +
	"- Use a real scale with clear jumps, not many nearby sizes: a headline, a " +
	"supporting line at roughly 45-55% of it, and a caption at about 30%. Two sizes " +
	"12px apart look like an accident.\n" +
	"- At most two weights and two sizes per slide. Contrast carries hierarchy; more " +
	"variety just reads as noise.\n" +
	"- Headlines run 4-9 words. A line holds about 30-40 characters at headline size; " +
	"break lines yourself rather than letting the browser wrap mid-phrase.\n" +
	"- Set line-height around 1.05-1.15 for display sizes and 1.4-1.5 for body. The " +
	"default 'normal' is far too loose for a headline.\n" +
	"- Tighten letter-spacing slightly on large text (-0.02em is usually right); leave " +
	"body text alone.\n" +
	"- Text over an image needs a guaranteed contrast floor: a scrim, a gradient " +
	"overlay, or a solid panel. Never rely on the photograph being dark enough.\n" +

	"\nMOTION (video)\n" +
	"- Structure every scene as ENTER, HOLD, EXIT. The hold is what makes a video " +
	"readable: a viewer needs roughly 0.3s per word of still, settled text.\n" +
	"- Entrances run 400-700ms. Anything slower feels sluggish at social pace; anything " +
	"under 250ms reads as a glitch rather than a move.\n" +
	"- Stagger siblings by 60-80ms via animation-delay. Everything arriving at once is " +
	"the signature of a template; a stagger reads as directed.\n" +
	"- Use real easing. cubic-bezier(0.22, 1, 0.36, 1) for entrances (decelerating, with " +
	"a touch of overshoot), cubic-bezier(0.4, 0, 1, 1) for exits. The default 'ease' on " +
	"everything is the single clearest amateur tell.\n" +
	"- Move a SHORT distance. 20-60px of travel, or a scale from 0.94 to 1. Elements " +
	"flying in from off-frame look like a slideshow transition.\n" +
	"- Set transform-origin deliberately on anything that scales or rotates. The default " +
	"center is rarely what you want for a panel or a bar.\n" +
	"- Animate at most 2-3 elements per scene. A scene where everything moves has no " +
	"focal point and nothing reads.\n" +
	"- Motion must be SETTLED by roughly 60% into the scene, so the viewer gets a still " +
	"frame to read. Entrance delay + duration well under data-duration is a hard rule, " +
	"not a guideline.\n" +

	"\nSCENE TRANSITIONS\n" +
	"- Default to a cut. A cut is invisible when the scenes belong together, and a video " +
	"where every join is a cross-fade reads as a screensaver.\n" +
	"- Transition when the two scenes are CONTINUOUS — the same thought carried on, the " +
	"same subject from another angle. Cut when the idea changes, or on a beat in the music.\n" +
	"- 300-500ms. Under 250ms is indistinguishable from a cut and costs you the scene " +
	"time; over 600ms the viewer is watching the transition rather than the content.\n" +
	"- fade is the safe default. push suits a change of subject, because the movement " +
	"itself says 'next'. slide is the most decorative — use it rarely.\n" +
	"- Do not alternate kinds for variety. Pick one for the piece and keep it; a video " +
	"that fades, then pushes, then slides looks like a demo of transitions.\n" +
	"- The scene's own entrance animation plays DURING the overlap, so keep that entrance " +
	"gentle when a transition is on: two strong moves at once reads as chaos.\n" +

	"\nCAROUSEL\n" +
	"- Tell one story across the slides: hook, then one idea per slide, then a CTA. A " +
	"carousel of unrelated slides is a set of posts, not a carousel.\n" +
	"- Hold the layout steady between slides — same margins, same type positions — and " +
	"vary only the content. The consistency is what makes it feel like one piece.\n" +
	"- One idea per slide. If a slide needs a second sentence to explain it, it is two " +
	"slides.\n" +

	"\nWHAT TO AVOID\n" +
	"- Pure black (#000) or pure white (#fff) as a large background; use the brand's own " +
	"near-black or off-white.\n" +
	"- Drop shadows used as decoration rather than to lift an element off what is behind it.\n" +
	"- More than two accent colours in one frame.\n" +
	"- Centering every single element by default. Left-aligned text is usually stronger " +
	"for anything longer than a few words.\n" +
	"- Filling the frame because it is empty. Space around the focal point IS the design.\n"
