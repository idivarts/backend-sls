package ai

// The pattern library — the L2 layer.
//
// L0 says what a hook must achieve; this says what hooks look like. The
// difference matters in practice: given only the standard, a model writes the
// same two hook shapes over and over (a question, then a bold claim), because
// those are the shapes most represented in its training data. Given a named
// menu it picks deliberately, and the output stops sounding like one voice
// with eight topics.
//
// Every archetype carries an anti-use. That is the half that does the work —
// "curiosity gap" with no "do not use it when the payoff is ordinary" produces
// clickbait, which is the failure mode this whole feature exists to avoid.
//
// This is reference material, not a precedence layer: nothing here overrides
// the brand's Design System, its performance data, or the craft contract. It
// is a menu, and the model is told to choose from it rather than obey it.
const viralityPatternsBlock = "\n--- HOOK AND STRUCTURE PATTERNS (a menu, not a formula) ---\n" +
	"Choose deliberately. Reaching for the same shape every time is what makes a feed " +
	"of posts read as one template with different words in it.\n\n" +

	"HOOK ARCHETYPES\n" +
	"- SPECIFIC NUMBER — lead with a real figure. Strong when the number is surprising " +
	"or hard-won. Not when it is round, estimated, or obviously invented.\n" +
	"- CONTRARIAN CLAIM — state the opposite of the received wisdom. Strong when the " +
	"piece genuinely defends it. Not as a pose you walk back in scene two.\n" +
	"- IN MEDIAS RES — open at the result or the moment of failure, then explain. " +
	"Strong for anything with a before/after. Not when there is no narrative to rewind.\n" +
	"- THE QUESTION THEY ALREADY HAVE — ask the exact thing the viewer was wondering. " +
	"Strong when you know the audience precisely. Not a rhetorical question, and never " +
	"one answerable 'no' in a way that ends the video.\n" +
	"- NEGATIVE FRAMING — 'stop doing X'. Strong when X is genuinely common. Not when it " +
	"is a strawman nobody actually does.\n" +
	"- DEMONSTRATED OUTCOME — show the finished thing in frame before explaining it. " +
	"Strong for anything visual or physical. Not for abstract or service topics where " +
	"there is nothing to show.\n" +
	"- CURIOSITY GAP — name a thing and withhold one specific detail. Strong only when " +
	"the payoff is genuinely worth the wait. Never when the answer is ordinary: an " +
	"unpaid gap is the fastest way to teach an audience to scroll past you.\n" +
	"- PATTERN INTERRUPT — an unexpected visual or statement with no setup. Strong in a " +
	"saturated feed. Not when it is unrelated to the content, which reads as a trick.\n" +
	"- COST OR STAKE — what it cost, what was lost, what is at risk. Strong when real " +
	"and specific. Not when vague ('a lot of time').\n\n" +

	"STRUCTURES\n" +
	"- HOOK / PROOF / PAYOFF / ONE ASK — the default. Claim, then earn it, then deliver, " +
	"then one action.\n" +
	"- LIST — 'three ways to…'. Each item must be independently useful, and the best " +
	"item goes second, not first: first is spent establishing the format, and a viewer " +
	"who got value at item two stays for three.\n" +
	"- BEFORE / AFTER — the state, the change, the result. Needs a visible delta.\n" +
	"- MYTH / CORRECTION — what people believe, why it is wrong, what is true instead. " +
	"Pairs naturally with the contrarian hook.\n" +
	"- WALKTHROUGH — a real sequence, in order. The weakest for reach and the strongest " +
	"for saves, so pick it when saves are the goal and say so in the ask.\n" +
	"- STORY — one subject, one incident, one turn. The only structure where withholding " +
	"the payoff to the end is correct.\n\n" +

	"USING THEM\n" +
	"- Name the archetype and structure you chose when you explain a draft, so the user " +
	"can ask for a different one instead of asking for 'something better'.\n" +
	"- When a piece has to work on several platforms, the structure usually holds and " +
	"the hook usually does not — rewrite the hook per platform before rewriting anything " +
	"else.\n"
