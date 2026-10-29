package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Deterministic checks on an AI-authored script, run before it is persisted.
//
// This is the cheapest quality lever in the whole virality feature and the
// only one that costs no tokens. A model that gets "the hook opens with a
// greeting" back as an instruction fixes it on the next step, with no judge
// model involved and nobody reading anything. It catches the class of mistake
// that is MECHANICAL — a missing section, a banned phrase, three competing
// calls to action — and leaves taste to the scorer in Phase 2.
//
// It is deliberately NOT the hard gate design_validate.go is. A design that
// fails validation is never written, which is right: nobody has seen it. A
// script has been streaming into the user's editor token by token for twenty
// seconds, and silently discarding it would be a worse experience than the
// problem it fixes. So a fatal problem buys ONE bounded repair turn, and
// whatever comes back is persisted either way.
//
// Checks that belong elsewhere are not duplicated here, and listing them is
// part of the contract: aspect ratio and safe zones are design concerns and
// live in design_validate.go; Instagram's six deprioritizers are properties of
// the rendered media, not the script; caption and per-platform publish fields
// are checked by the frontend's publish-readiness gate. A script validator
// that pretended to cover those would report false confidence.

// scriptSeverity decides what a failed check costs.
type scriptSeverity int

const (
	// severityWarn costs points in the Phase 2 scorer and nothing else.
	severityWarn scriptSeverity = iota
	// severityCap persists, but ceilings the overall score in Phase 2.
	severityCap
	// severityFatal buys one repair turn before the script is persisted.
	severityFatal
)

func (s scriptSeverity) String() string {
	switch s {
	case severityFatal:
		return "fatal"
	case severityCap:
		return "cap"
	default:
		return "warn"
	}
}

// ScriptProblem is one failed check, phrased as an instruction the model can
// act on rather than a diagnostic for a developer. The ID is stable so the
// Phase 2 scorer can pre-fill the matching sub-criterion instead of paying a
// judge model to re-derive something already known.
type ScriptProblem struct {
	ID       string         `json:"id"`
	Severity scriptSeverity `json:"-"`
	Message  string         `json:"message"`
}

// MarshalJSON emits the severity as its name rather than its ordinal. The
// number means nothing to a client, and Phase 2's panel has to branch on it.
func (p ScriptProblem) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID       string `json:"id"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
	}{p.ID, p.Severity.String(), p.Message})
}

// Words per second for turning a word count into a duration. Conversational
// delivery over social video sits near this; it is a default we tune, not a
// law, which is why everything downstream of it is a warning.
const wordsPerSecond = 2.5

// Target duration per content format, in seconds, used for the length check.
// A format with no entry is simply not length-checked rather than guessed at.
var formatTargetSeconds = map[string]struct{ min, max float64 }{
	"reel":  {15, 60},
	"story": {5, 15},
	"video": {30, 180},
}

var (
	// Openers that spend the one moment that decides whether anyone watches.
	// Anchored to the start of the hook, so a script ABOUT greeting customers
	// is not flagged for the word "hi" in its body.
	reThroatClearing = regexp.MustCompile(`(?i)^\W*(hey|hi|hello|yo)\b[ ,]*(guys|everyone|there|all|folks)?\b|^\W*(what'?s up|in this (video|one|short)|let me tell you|so basically|welcome back|today (i|we)'?(m|re|ll))\b`)

	// Engagement bait. Meta demotes it, LinkedIn suppresses it: it costs reach
	// rather than earning it, so it is worse than having no ask at all.
	reEngagementBait = regexp.MustCompile(`(?i)\b(like (this )?if|tag (someone|a friend) who|comment ['"]?yes['"]?|share if you|double tap if|drop a ['"]?\w+['"]? (in the comments|below) if|like and (share|subscribe))\b`)

	// "Follow for more" is almost never the ask the surface rewards.
	reFollowForMore = regexp.MustCompile(`(?i)\bfollow (me |us )?for more\b`)

	// Section headers the generator is asked to produce.
	reHookHeading = regexp.MustCompile(`(?im)^#{1,4}\s*hook\b`)
	reCTAHeading  = regexp.MustCompile(`(?im)^#{1,4}\s*(cta|call[ -]to[ -]action)\b`)
	reBodyHeading = regexp.MustCompile(`(?im)^#{1,4}\s*(body|scene)`)

	// A scene or beat marker, for the cadence check.
	reSceneMarker = regexp.MustCompile(`(?im)^\s*\*{0,2}(scene|beat)\s*\d`)

	// Cues that the piece carries meaning with the sound off.
	// Labels that mean "the viewer can read this with the sound off".
	reSoundOffWord = regexp.MustCompile(`(?i)\b(on-?screen|caption|subtitle|overlay|burned[- ]in|text)\b`)

	// Imperative asks, for counting calls to action. "pin" and "reply" are
	// here because they are real asks on X and LinkedIn, and leaving them out
	// made a perfectly good "pin this" CTA look like no CTA at all.
	reAsk = regexp.MustCompile(`(?i)\b(send\s+(\w+\s+)?(this|it|that)|save (this|it)|share (this|it)|pin (this|it|a)|reply with|drop (a|your)|comment\b|follow\b|subscribe\b|tap (the )?link|click (the )?link|sign up|book (a|your)|try (it|this)|download)\b`)

	// Markdown decoration to strip before counting words.
	reMarkdownNoise = regexp.MustCompile(`[*_#>` + "`" + `]+`)
)

// scriptSection returns the body of a named markdown section, or "" when the
// script has no such heading.
func scriptSection(script, headingPattern string) string {
	// (?ism): case-insensitive, `.` spans newlines so a section can be several
	// paragraphs, and `^` matches at every line start so the heading is found
	// mid-document rather than only at the top.
	re := regexp.MustCompile(`(?ism)^#{1,4}\s*` + headingPattern + `[^\n]*\n(.*?)(?:\n#{1,4}\s|\z)`)
	m := re.FindStringSubmatch(script)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// spokenWords counts the words a presenter would actually say, which is what
// decides a script's duration.
//
// Camera directions, on-screen text and the model's own commentary are not
// speech. Counting them made a twenty-second reel read as "about 80 seconds"
// and fired a length warning on a script that was the right length.
func spokenWords(script string) int {
	n := 0
	for _, l := range normaliseScript(script) {
		if l.Note {
			continue
		}
		n += len(strings.Fields(l.Body))
	}
	return n
}

// The model does not write one markdown dialect, it writes about fourteen.
// Across a handful of generations the same cue appeared as `**VO:**`, `*VO:*`,
// `- *VO:*`, `**Line (on-screen + VO):**`, `**On-screen text:**`, `*On-screen:*`
// and `- *On-screen:*`. Matching dialects one at a time is how the first three
// rounds of this validator were spent: each round fixed the shapes in the last
// run and the next generation produced new ones.
//
// So nothing below matches a dialect. Every line is normalised ONCE —
// list bullet stripped, emphasis stripped, label identified by what it says
// rather than how it is decorated — and the checks read the normalised form.
var (
	reListBullet = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s+`)
	reEmphasis   = regexp.MustCompile(`\*{1,3}|_{2,}`)
	// A label is any short leading phrase before a colon. The words inside it
	// decide what kind it is; the brackets, slashes and extra words that the
	// model sprinkles in ("Line (on-screen + VO)") do not have to be enumerated.
	reLeadingLabel = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z /+&()-]{0,40}?)\s*:\s*`)
	// Words that mean "a presenter says this out loud".
	reSpokenWord = regexp.MustCompile(`(?i)\b(vo|v/o|voice ?over|narration|narrator|spoken|speech|says|line|script|dialogue|host|talent)\b`)
	// Words that mean "this is a production note, not speech".
	reProductionWord = regexp.MustCompile(`(?i)\b(visual|on-?screen|caption|subtitle|text|overlay|cue|b-?roll|sfx|sound|music|title|shot|graphic)\b`)
)

// normalisedLine is one line of a script with its decoration removed.
type normalisedLine struct {
	Label  string // the leading label, lowercased, "" when the line has none
	Body   string // everything after the label, unquoted and unemphasised
	Spoken bool   // the label says a presenter says this
	Note   bool   // the label says this is a production note
}

// normaliseScript strips list bullets and emphasis from every line and splits
// off any leading label, so the checks can ask "is this spoken?" instead of
// trying to recognise every way the model might have written "VO:".
//
// A line can be both spoken and a note — "Line (on-screen + VO)" is a real
// example — in which case spoken wins, because the words after it are said
// aloud whatever else is happening on the frame.
func normaliseScript(script string) []normalisedLine {
	var out []normalisedLine
	for _, raw := range strings.Split(script, "\n") {
		line := reListBullet.ReplaceAllString(raw, "")
		line = reEmphasis.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n := normalisedLine{Body: line}
		// A line that is entirely parenthesised is a stage direction, whatever
		// it does or does not call itself.
		if strings.HasPrefix(line, "(") && strings.HasSuffix(line, ")") {
			n.Note = true
			out = append(out, n)
			continue
		}
		if m := reLeadingLabel.FindStringSubmatch(line); m != nil {
			label := strings.ToLower(strings.TrimSpace(m[1]))
			spoken := reSpokenWord.MatchString(label)
			note := reProductionWord.MatchString(label)
			// Only treat it as a label when it actually names something we
			// recognise. Otherwise "Monday: the day it broke" would lose its
			// first two words.
			if spoken || note {
				n.Label = label
				n.Body = strings.TrimSpace(line[len(m[0]):])
				n.Spoken = spoken
				n.Note = note && !spoken
			}
		}
		n.Body = strings.Trim(n.Body, `"“”`)
		out = append(out, n)
	}
	return out
}

// openingLines returns the first n lines of a block that a viewer actually
// perceives — spoken words and on-screen text, not camera directions.
func openingLines(block string, n int) []string {
	var out []string
	for _, l := range normaliseScript(block) {
		if l.Body == "" {
			continue
		}
		out = append(out, l.Body)
		if len(out) == n {
			break
		}
	}
	return out
}

// spokenOpening returns the first line a presenter actually says.
//
// This is what the rambling-opening check measures, and getting it wrong is
// not harmless: before normalisation it fell back to the first line of the
// hook, which is usually a camera direction, and measured a shot description
// as if someone were reading it aloud.
func spokenOpening(hook string) string {
	lines := normaliseScript(hook)
	for _, l := range lines {
		if l.Spoken && l.Body != "" {
			return l.Body
		}
	}
	// No line is explicitly marked as speech: fall back to the first line that
	// is at least not a production note.
	for _, l := range lines {
		if !l.Note && l.Body != "" {
			return l.Body
		}
	}
	return ""
}

// firstSentence returns the opening sentence of a line.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".!?"); i > 0 {
		return strings.TrimSpace(s[:i+1])
	}
	return s
}

var (
	// A line that is entirely italic, or entirely parenthesised, is the
	// model talking to the user about the script rather than writing it:
	// "*Cue: you to camera.*", "*(Structure: hook / payoff / one ask.)*".
	// Bold labels like "**VO:**" are untouched — the pattern requires a
	// single leading asterisk followed by a non-asterisk.
	reItalicOnlyLine = regexp.MustCompile(`(?m)^\s*\*[^*\s][^\n]*\*\s*$`)
	reParenOnlyLine  = regexp.MustCompile(`(?m)^\s*\([^\n]*\)\s*$`)

	// Where the script stops and the chat message resumes. Everything from
	// here on is the model addressing the user.
	//
	// A horizontal rule is deliberately NOT a marker. It reads like one, but
	// this model uses `---` as a separator between every section, so treating
	// it as the boundary cut scripts in half — or, because only the first
	// match was considered, cut nothing at all.
	// Two shapes. A note heading ("Two notes:", "*Note on the ask:*") need NOT
	// end its line -- the model often continues straight into the prose -- so
	// that branch is not anchored to the line end. A closing question to the
	// user is anchored, because mid-line "want" is an ordinary word.
	reCommentaryTail = regexp.MustCompile(`(?im)^\s*\*{0,2}((a few |two |some )?notes?\b[^:\n]{0,60}:|(want|would you like|should i|shall i|do you want|let me know|i can also)\b[^\n]*$)`)
)

// stripCommentary removes the model's conversational wrapper and its
// production notes, leaving the words that are actually the script.
//
// This is not cosmetic. Without it the checks read the model's own prose
// ABOUT the script as if it were the script, and the failure is perfectly
// backwards: a model that correctly avoided engagement bait and then
// explained that it had avoided it — "the ask is an opinion prompt, not
// engagement bait … without tripping its suppression of 'comment YES'
// prompts" — was flagged for using the very phrase it was declining to use.
// The same wrapper inflated the ask count and the spoken-word count.
//
// Found by running against real dev output. The unit fixtures were clean
// handcrafted scripts and could never have surfaced it, which is the whole
// argument for verifying on dev rather than trusting a green test run.
func stripCommentary(script string) string {
	// Cut the conversational tail at the EARLIEST marker that still sits in
	// the last third of the document. Taking the first match outright would
	// let a stray early match disarm the cut; requiring the last would miss a
	// notes block followed by a closing question. Scanning only the tail
	// region gives both.
	if locs := reCommentaryTail.FindAllStringIndex(script, -1); locs != nil {
		threshold := len(script) / 3
		for _, loc := range locs {
			if loc[0] > threshold {
				script = script[:loc[0]]
				break
			}
		}
	}
	script = reItalicOnlyLine.ReplaceAllString(script, "")
	script = reParenOnlyLine.ReplaceAllString(script, "")
	return script
}

// ValidateScript runs every deterministic check over a generated script and
// returns the problems, most severe first.
//
// `format` is the content format ("reel", "video", …) and may be empty, in
// which case the length checks are skipped rather than guessed at.
func ValidateScript(rawScript, format string) []ScriptProblem {
	var problems []ScriptProblem
	add := func(id string, sev scriptSeverity, msg string) {
		problems = append(problems, ScriptProblem{ID: id, Severity: sev, Message: msg})
	}

	script := stripCommentary(rawScript)
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		add("D1", severityFatal, "The script is empty. Write it with the sections asked for: a hook, a body of scenes, and one call to action.")
		return problems
	}

	hook := scriptSection(script, `hook`)
	cta := scriptSection(script, `(?:cta|call[ -]to[ -]action)`)

	// D1 — the hook is the gate; without one nothing downstream matters.
	if !reHookHeading.MatchString(script) {
		add("D1", severityFatal, "The script has no Hook section. Add one, as a '## Hook' heading, carrying the first 1-3 seconds.")
	} else if hook == "" {
		add("D1", severityFatal, "The Hook section is empty. Write the opening 1-3 seconds: a specific claim, number or stake, stated in the first sentence.")
	}

	// D2 — throat-clearing spends the only moment that decides anything.
	// Checked across the hook's opening lines, not just the literal first
	// sentence: the spoken line is often the second or third thing in the
	// block, after an on-screen-text or visual cue.
	for _, line := range openingLines(hook, 3) {
		if reThroatClearing.MatchString(line) {
			add("D2", severityCap, fmt.Sprintf("The hook opens with throat-clearing (%q). Cut the greeting and open on the subject or the claim itself.", truncate(line, 60)))
			break
		}
	}

	// There is deliberately no "the opening rambles" check.
	//
	// What matters is how long before the SUBJECT arrives, and that is not
	// computable: a good hook can put it in word two or word nine. Sentence
	// length was the available proxy and it is a bad one. At three seconds it
	// flagged a brisk eight-word claim; raised to six it flagged "Our second
	// location cost 40% more to open than our first — and it had nothing to do
	// with rent", which lands its number in word four and is exactly the hook
	// the craft contract asks for.
	//
	// Judging whether an opening is brisk is a judgement, so it belongs to the
	// Phase 2 scorer's H2 criterion rather than to a word count here.

	// D7 — exactly one ask. Two asks is zero asks.
	if !reCTAHeading.MatchString(script) {
		add("D7", severityFatal, "The script has no call to action. Add a '## CTA' section with exactly ONE ask, and pick the one this platform rewards.")
	} else if cta == "" {
		// A heading with nothing under it is as useless as no heading, and
		// easier to miss because the section looks present.
		add("D7", severityFatal, "The CTA section is empty. Write exactly ONE ask, and pick the one this platform rewards.")
	} else if n := countAsks(cta); n > 1 {
		add("D7", severityCap, fmt.Sprintf("The call to action contains %d separate asks. Keep exactly one — two asks is zero asks.", n))
	}
	// There is deliberately no "the CTA contains no recognisable ask" check.
	//
	// It was here, and both times it fired on dev it was wrong: once on
	// "curious what blindsided you on an expansion?" (an opinion prompt, and
	// the ask LinkedIn's own playbook asks for) and once on "send them this
	// before they make another pot" (a vocabulary gap). Natural language has
	// endless ways to ask for something, and every round of broadening the
	// pattern bought one more false positive.
	//
	// The structural cases are still caught above and still fatal: no CTA
	// section, and an empty one. Whether a paragraph of prose amounts to an
	// ask is a judgement, and judgement is what the Phase 2 scorer is for —
	// a regex that guesses at it only teaches people to distrust the score.
	// The ask vocabulary stays, because a missing verb makes the multi-ask
	// count UNDER-report, which is the safe direction.

	// D8 — bait costs reach rather than earning it.
	if m := reEngagementBait.FindString(script); m != "" {
		add("D8", severityCap, fmt.Sprintf("The script uses engagement bait (%q). Meta demotes it and LinkedIn suppresses it, so it costs reach. Replace it with a genuine ask.", m))
	}

	// Not fatal, not a cap: wrong rather than forbidden.
	if reFollowForMore.MatchString(cta) {
		add("D7b", severityWarn, "The ask is 'follow for more', which is almost never the action a feed surface rewards. Prefer a send or a save.")
	}

	// D6 — a body that is one undifferentiated block has no beats.
	if reBodyHeading.MatchString(script) {
		if n := len(reSceneMarker.FindAllString(script, -1)); n > 0 && n < 2 {
			add("D6", severityWarn, "The body has a single scene. Break it into beats so something changes every few seconds.")
		}
	}

	// D9 — an opened loop the script never closes is a bait-and-switch.
	if hook != "" && cta != "" {
		if !closesItsLoop(hook, script) {
			add("D9", severityWarn, "The hook's specific promise does not appear to be closed anywhere later. Make the payoff echo the hook's own words.")
		}
	}

	// D10 — most feed viewing is sound-off.
	if !hasSoundOffCues(script) {
		add("D10", severityWarn, "The script gives no on-screen text or caption cues. Most feed viewing is muted, so the idea has to survive without audio.")
	}

	// D4/D13 — length against the format's band.
	if band, ok := formatTargetSeconds[strings.ToLower(strings.TrimSpace(format))]; ok {
		secs := float64(spokenWords(script)) / wordsPerSecond
		switch {
		case secs > band.max*1.2:
			add("D13", severityWarn, fmt.Sprintf("At about %.0f seconds of speech this is well past the %.0f-%.0f second band for a %s. Cut it back.", secs, band.min, band.max, format))
		case secs < band.min*0.8:
			add("D13", severityWarn, fmt.Sprintf("At about %.0f seconds of speech this is short for a %s (%.0f-%.0f seconds). There is room for another beat.", secs, format, band.min, band.max))
		}
	}

	sortProblemsBySeverity(problems)
	return problems
}

// reViewerQuestion is a question put to the viewer: second person, ending in
// a question mark.
var reViewerQuestion = regexp.MustCompile(`(?i)\byou(r|'ve|'re)?\b[^?]*\?`)

// countAsks counts the calls to action in a CTA section.
//
// An imperative is the obvious form, but it is not the only one. An opinion
// prompt — "curious what blindsided you on an expansion?" — is a genuine ask,
// and on LinkedIn it is the RIGHT ask, the one the platform playbook tells the
// model to prefer over a reaction prompt. Counting only imperatives therefore
// reported "no recognisable ask" on exactly the CTAs the craft contract asks
// for, which would have taught anyone reading the score to ignore it.
//
// A question counts as one ask however many sentences it spans, so a single
// prompt phrased across two lines is not read as two competing asks.
func countAsks(cta string) int {
	// Distinct asks, not ask MENTIONS. A script that puts the same ask in the
	// on-screen text and again in the voiceover -- which is correct craft, not
	// a second ask -- was being counted twice and reported as competing CTAs.
	distinct := map[string]bool{}
	for _, m := range reAsk.FindAllString(cta, -1) {
		distinct[strings.ToLower(strings.Join(strings.Fields(m), " "))] = true
	}
	n := len(distinct)
	if reViewerQuestion.MatchString(cta) {
		n++
	}
	return n
}

// hasSoundOffCues reports whether anything in the script is meant to be read
// rather than heard.
func hasSoundOffCues(script string) bool {
	for _, l := range normaliseScript(script) {
		if l.Label != "" && reSoundOffWord.MatchString(l.Label) {
			return true
		}
	}
	return false
}

// closesItsLoop reports whether the hook's distinctive words recur later in
// the script.
//
// Crude on purpose: it is looking for evidence the payoff is about the same
// thing the hook promised, not grading the writing. Short and common words are
// dropped so the overlap means something, and a single recurrence is enough —
// the cost of a false "did not close" is a spurious warning, and the check is
// a warning precisely because it cannot be certain.
func closesItsLoop(hook, script string) bool {
	rest := script
	if i := strings.Index(script, hook); i >= 0 {
		rest = script[i+len(hook):]
	}
	restLower := strings.ToLower(rest)

	var distinctive int
	var found int
	for _, w := range strings.Fields(strings.ToLower(reMarkdownNoise.ReplaceAllString(hook, " "))) {
		w = strings.Trim(w, `.,!?;:"'()[]`)
		if len(w) < 6 || commonWords[w] {
			continue
		}
		distinctive++
		if strings.Contains(restLower, w) {
			found++
		}
	}
	if distinctive == 0 {
		return true // nothing distinctive to track; do not cry wolf
	}
	return found > 0
}

// commonWords are long enough to pass the length filter but carry no topic.
var commonWords = map[string]bool{
	"because": true, "through": true, "really": true, "should": true,
	"actually": true, "something": true, "everything": true, "nothing": true,
	"people": true, "better": true, "little": true, "another": true,
	"before": true, "around": true, "without": true, "thought": true,
}

func sortProblemsBySeverity(p []ScriptProblem) {
	for i := 1; i < len(p); i++ {
		for j := i; j > 0 && p[j].Severity > p[j-1].Severity; j-- {
			p[j], p[j-1] = p[j-1], p[j]
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// HasFatal reports whether any problem is severe enough to be worth a repair
// turn.
func HasFatal(problems []ScriptProblem) bool {
	for _, p := range problems {
		if p.Severity == severityFatal {
			return true
		}
	}
	return false
}

// repairInstruction renders the problems into the user-turn text for the one
// repair pass. Only fatal and cap problems are worth spending a turn on;
// warnings ride into the Phase 2 score instead.
func repairInstruction(problems []ScriptProblem) string {
	var lines []string
	for _, p := range problems {
		if p.Severity == severityWarn {
			continue
		}
		lines = append(lines, "- "+p.Message)
	}
	if len(lines) == 0 {
		return ""
	}
	return "The script you just wrote has these problems:\n" + strings.Join(lines, "\n") +
		"\n\nRewrite it, fixing exactly these and changing nothing else. Return the full script in the same format, with no preamble."
}
