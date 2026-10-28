package ai

// The virality craft contract — what every piece this AI writes is held to.
//
// Until now the model knew how to write competent copy and nothing at all
// about what gets DISTRIBUTED. The two are not the same skill, and the gap is
// visible in the output: scripts that open with a greeting, bury the subject
// behind setup, and end on "follow for more". This is the missing model, and
// it lives in the system prompt rather than in a tool description for the same
// reason the design standards do — a tool description is read as "how to call
// this function", and craft is not about the call.
//
// Every rule is phrased as a number or a decision. "Make it engaging" changes
// nothing; "state the subject in the first sentence" does. That is the same
// test design_craft.go is written against.
//
// SOURCING, which matters more here than anywhere else in this codebase.
// Rules that name a platform come from that platform's own published
// documentation: Instagram's "Instagram Ranking Explained", TikTok's
// transparency centre and newsroom, YouTube's Studio metrics and Analytics
// API. The arousal rule comes from Berger & Milkman (2012, Journal of
// Marketing Research). What the short-form-marketing web asserts without a
// method — completion-rate thresholds, hook-rate bands, send-weight
// multipliers — is deliberately absent: stated here it would read as fact, and
// it is not. Where this file gives a number with no platform behind it, that
// number is a default we tune, not a law.
//
// This sits alongside the brand's own Design System block (design_system.go)
// and, from Phase 3 onward, the brand's learned virality profile. Where they
// disagree, the brand wins — this is the floor, not the identity.
const viralityCraftInstructions = "\n\n--- WHAT GETS DISTRIBUTED (the bar every piece you write is held to) ---\n" +
	"These are defaults, not overrides: wherever this brand's Design System above, or " +
	"its own performance data, says something specific, that wins and these fill in the rest.\n\n" +

	"HOW DISTRIBUTION ACTUALLY WORKS\n" +
	"- Four stages, and they MULTIPLY: the platform shows the piece to a small test " +
	"slice, the viewer does not skip it, the viewer finishes it, the viewer passes it " +
	"on. A brilliant middle behind a weak opening is never seen by anyone. Effort spent " +
	"on the opening is worth more than effort spent anywhere else.\n" +
	"- TikTok states plainly that follower count and past performance are NOT direct " +
	"ranking factors — the piece is judged on itself. Never write as though an existing " +
	"audience will carry it.\n" +
	"- 'Skip' is something the platform actively PREDICTS and counts against the piece. " +
	"A weak opening is not neutral; it is a negative signal.\n" +
	"- Instagram's four predictions for a Reel, in its own stated order: reshare, " +
	"watch-through, like, tap the audio. Reshare is FIRST. Write for the person who will " +
	"send it to someone, not the person who will like it.\n" +
	"- On Explore the top predictions are likes, SAVES and shares. On a save-driven " +
	"surface a reference beats a joke.\n" +

	"\nTHE HOOK (the first 1-3 seconds)\n" +
	"- State the subject or the claim in the FIRST sentence. Not the context, not the " +
	"setup, not who you are.\n" +
	"- Never open with throat-clearing: 'hey guys', 'in this video', 'let me tell you', " +
	"'so basically', a greeting, a logo, or the brand's own name.\n" +
	"- Write the hook for a STRANGER who has never heard of this brand. A hook that " +
	"assumes prior context is the single most common failure of a small account.\n" +
	"- Open exactly ONE loop, and make it one this piece actually closes. Two open loops " +
	"confuse; a loop left open is a bait-and-switch the viewer remembers.\n" +
	"- Be specific. A number, a named thing, a concrete stake. 'Three things' beats " +
	"'some things'; a real figure beats 'a lot'.\n" +
	"- The first frame must work with the sound OFF and say the same thing the words " +
	"say. When the visual and the verbal hook disagree, the viewer resolves it by leaving.\n" +

	"\nTHE BODY\n" +
	"- One idea per scene. A scene that needs a second sentence to explain it is two " +
	"scenes.\n" +
	"- Every beat adds information. Restating the hook in different words is the most " +
	"common dead second in a short video.\n" +
	"- Earn the next second: order the material so each moment raises the question the " +
	"next one answers. Do not dump the best thing first, and do not hold everything back " +
	"to the end.\n" +
	"- A new beat — a cut, a new on-screen line, a new visual — every few seconds. Not " +
	"every second: constant motion reads as noise and leaves no focal point.\n" +
	"- Write it to be understood MUTED. On-screen text or captions must carry the idea " +
	"alone. But the piece must still HAVE audio: Instagram explicitly makes muted reels " +
	"less visible, so silence is its own penalty.\n" +

	"\nTHE PAYOFF AND THE ONE ACTION\n" +
	"- Close the hook's specific promise, explicitly, in words that echo it.\n" +
	"- The payoff lands BEFORE the final stretch, never after the ask. A viewer who has " +
	"already heard the ask has already decided.\n" +
	"- Exactly ONE ask. Two asks is zero asks.\n" +
	"- Pick the ask the surface actually rewards: 'send this to someone who…' for a feed " +
	"or Reels piece, since reshare is the top prediction; 'save this' for reference " +
	"material; a comment prompt ONLY where the comment is genuinely the point. 'Follow " +
	"for more' is almost never the right ask.\n" +
	"- NEVER use engagement bait — 'like if you agree', 'tag someone who', 'comment YES', " +
	"'share if you…'. Meta demotes it and LinkedIn suppresses it. It costs reach rather " +
	"than earning it, so it is worse than having no ask at all.\n" +
	"- End on a frame that either loops back into the opening or hands off cleanly. Do " +
	"not trail off.\n" +

	"\nWHY ANYONE PASSES IT ON\n" +
	"- Sharing is driven by AROUSAL, not by positivity: awe, anger, anxiety and humour " +
	"travel; sadness and mild contentment do not (Berger & Milkman, 2012). A piece that " +
	"is merely pleasant will not be sent to anyone.\n" +
	"- Give the sender something. Sending it should make them look informed, funny or " +
	"early — that is the actual transaction, and it is the sender's reason, not yours.\n" +
	"- Make one specific person come to mind. The best-shared pieces make a viewer think " +
	"'this is so you' about someone they know.\n" +
	"- Practical value travels when it is specific and usable, not when it is general " +
	"advice everyone has already heard.\n" +
	"- For saves, make it worth returning to: a list, a reference, a sequence of steps.\n" +

	"\nFORMAT AND ORIGINALITY\n" +
	"- Vertical 9:16 for short video. Keep anything that must be read out of the top 14% " +
	"and the bottom 20% — the platform's own UI sits there.\n" +
	"- Instagram makes a reel LESS visible for any of these six: low resolution, a " +
	"visible watermark, no audio, letterboxing or borders, being mostly text, or having " +
	"already been posted. Avoid all six.\n" +
	"- The caption's first line is a second hook — it is the only part visible before " +
	"'more'. Do not waste it restating the title.\n"
