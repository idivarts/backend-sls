package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// The virality scorer — a 0-100 read on a piece before it is published,
// overall and by Hook / Body / Action.
//
// WHAT THIS NUMBER IS. It is not a probability of going viral, and the
// product copy says so where the number is shown. Pre-publication virality
// models in the literature are dominated by channel-level features — audience
// size, not craft — so a percentage would be astrology, and the first
// well-scoring flop would destroy trust in the whole feature. What this
// measures is conformance to the craft that drives the metrics the platforms
// demonstrably rank on. That is honest, every point is attached to a fix, and
// it survives being wrong about an individual post.
//
// HOW IT IS BUILT, and why it is built that way. Two findings from the
// LLM-as-judge literature shape the whole design:
//
//   - ONE DIMENSION PER CALL. A single prompt scoring five dimensions anchors
//     on the first and bleeds it into the rest, producing correlated scores
//     that look precise and are not. Each dimension therefore gets its own
//     call, and they run concurrently.
//   - LOW-PRECISION SUB-CRITERIA, HIGH-PRECISION ROLLUP. Asking a model for a
//     number out of 100 is unreliable without gold-set evidence it can use
//     that resolution. Sub-criteria are 0/1/2 and the 0-100 is arithmetic,
//     done here in code, where it is inspectable and a weight can be changed
//     without touching a prompt.
//
// Everything the deterministic layer already settled is PRE-FILLED rather
// than re-asked (script_validate.go). That cuts tokens, removes variance, and
// keeps the judge on the questions that actually need judgement.

// rubricVersion is stamped on every score. Scores from different rubric or
// judge versions are not comparable and must never be plotted together — an
// audit of LLM judges found a same-family model upgrade shifting scores by up
// to 13% of the scale.
const rubricVersion = "1.0.0"

type dimensionKey string

const (
	dimHook   dimensionKey = "hook"
	dimBody   dimensionKey = "body"
	dimAction dimensionKey = "action"
	dimShare  dimensionKey = "share"
	dimFit    dimensionKey = "fit"
)

// criterion is one 0/1/2 judgement.
//
// prefill names the deterministic check that already answers it, if any. When
// that check fired the criterion scores 0 without a model call; when it did
// not, the criterion scores 2. Either way the judge never sees it.
//
// A prefill is only safe where the check is EQUIVALENT to the criterion, not
// merely related. "No engagement bait" is a prefill: the regex answers exactly
// that question. "Exactly one ask, and it is the action this platform
// rewards" is not, even though a check counts asks — the second half is a
// judgement, and prefilling it would have scored every script a free 2 on the
// part that actually matters. Narrower checks are passed to the judge as
// EVIDENCE instead, which informs it without answering for it.
type criterion struct {
	ID      string
	Text    string
	Prefill string
}

type dimension struct {
	Key      dimensionKey
	Label    string
	Weight   int
	Question string
	Criteria []criterion
	// Evidence names the deterministic checks worth showing this dimension's
	// judge. Scoped per dimension on purpose: handing every judge every
	// finding would leak the CTA's problems into the Hook score, which is the
	// cross-contamination the one-call-per-dimension split exists to prevent.
	Evidence []string
}

// The rubric. Weights sum to 100.
//
// Hook is heaviest because the funnel is multiplicative: a brilliant body
// behind a weak opening is never seen. Share Trigger sits above Payoff
// because passing it on is the only stage that compounds. Platform Fit is
// lightest precisely BECAUSE most of it is deterministic and already gated —
// it should rarely be where points are lost.
var rubric = []dimension{
	{
		Key: dimHook, Label: "Hook", Weight: 30, Evidence: []string{"D1", "D2"},
		Question: "Will a stranger scrolling a feed not skip this in the first 1-3 seconds?",
		Criteria: []criterion{
			{ID: "H1", Text: "The first sentence carries a specific claim, number or stake — not a topic announcement."},
			// Judged, not pre-filled. The deterministic version of this check
			// (D3) was removed because sentence length is a bad proxy for
			// when the subject arrives — it flagged good hooks twice. This is
			// exactly the kind of question a judge should answer.
			{ID: "H2", Text: "The opening is brisk: the subject lands in the first few words."},
			{ID: "H3", Text: "It opens exactly one curiosity loop, and one this piece can actually close."},
			{ID: "H4", Text: "No throat-clearing: no greeting, no 'in this video', no brand name first.", Prefill: "D2"},
			{ID: "H5", Text: "The visual and the verbal hook say the same thing, and the first frame reads with sound off."},
			{ID: "H6", Text: "It is written for someone who has never heard of this brand — no assumed context."},
		},
	},
	{
		Key: dimBody, Label: "Body", Weight: 25, Evidence: []string{"D6", "D10", "D13"},
		Question: "Does the middle hold a viewer who already decided to stay?",
		Criteria: []criterion{
			{ID: "B1", Text: "One idea per scene; no scene needs a second sentence to explain itself."},
			{ID: "B2", Text: "Every beat adds information. The hook is not restated in different words."},
			{ID: "B3", Text: "Something changes every few seconds rather than one undifferentiated block."},
			{ID: "B4", Text: "The ordering earns the next second: each moment raises what the next answers."},
			{ID: "B5", Text: "It carries its meaning muted — on-screen text or captions do the work alone."},
			{ID: "B6", Text: "The length suits the format: no scene is overstuffed and nothing drags."},
		},
	},
	{
		Key: dimAction, Label: "Action", Weight: 15, Evidence: []string{"D7", "D7b", "D8", "D9"},
		Question: "Does the promise close, and is the single ask the right one for this surface?",
		Criteria: []criterion{
			{ID: "P1", Text: "The hook's specific promise is explicitly closed, in words that echo it."},
			{ID: "P2", Text: "The payoff lands before the ask, not after it."},
			{ID: "P3", Text: "Exactly one ask, and it is the action this platform actually rewards — a send or a save on a feed surface, rarely a follow."},
			{ID: "P4", Text: "No engagement bait of any kind.", Prefill: "D8"},
			{ID: "P5", Text: "The ending loops back or hands off cleanly rather than trailing off."},
		},
	},
	{
		Key: dimShare, Label: "Share", Weight: 20,
		Question: "Is there a reason for one person to send this to another specific person?",
		Criteria: []criterion{
			{ID: "S1", Text: "Sending it makes the sender look informed, funny or early — there is something in it for them."},
			{ID: "S2", Text: "It carries a high-arousal emotion (awe, anger, anxiety, humour) rather than mild pleasantness."},
			{ID: "S3", Text: "There is one concrete, specific, usable takeaway — not general advice."},
			{ID: "S4", Text: "It makes a particular person come to mind: 'this is so you'."},
			{ID: "S5", Text: "It is worth returning to — a list, a reference, a sequence worth saving."},
		},
	},
	{
		Key: dimFit, Label: "Fit", Weight: 10, Evidence: []string{"D10", "D13"},
		Question: "Is this shaped for the platforms it targets?",
		Criteria: []criterion{
			{ID: "F1", Text: "The structure suits the surface's own ranking behaviour, not a generic social post."},
			{ID: "F2", Text: "It respects the format's length band.", Prefill: "D13"},
			{ID: "F3", Text: "Sound-off cues are present and audio is implied rather than absent.", Prefill: "D10"},
			{ID: "F4", Text: "Any caption or title line does its own job rather than restating the hook verbatim."},
		},
	},
}

// CriterionScore is one judged or pre-filled criterion.
type CriterionScore struct {
	ID       string `json:"id"`
	Score    int    `json:"score"`
	Note     string `json:"note,omitempty"`
	Prefills bool   `json:"prefilled,omitempty"`
}

// DimensionScore is one of the five, already rolled up to 0-100.
type DimensionScore struct {
	Key      string           `json:"key"`
	Label    string           `json:"label"`
	Weight   int              `json:"weight"`
	Score    int              `json:"score"`
	Criteria []CriterionScore `json:"criteria"`
}

// ScoreCap records a ceiling and why it was applied. A cap is always shown
// with its reason — a silent ceiling is indistinguishable from a bug.
type ScoreCap struct {
	At     int    `json:"at"`
	Reason string `json:"reason"`
}

// Fix is one actionable improvement, ranked by what it would buy.
type Fix struct {
	Criterion string `json:"criterion"`
	Dimension string `json:"dimension"`
	Message   string `json:"message"`
}

// ViralityScore is the whole result.
type ViralityScore struct {
	Overall       int              `json:"overall"`
	Band          string           `json:"band"`
	Cap           *ScoreCap        `json:"cap,omitempty"`
	Dimensions    []DimensionScore `json:"dimensions"`
	Fixes         []Fix            `json:"fixes"`
	Problems      []ScriptProblem  `json:"problems,omitempty"`
	RubricVersion string           `json:"rubricVersion"`
	JudgeModel    string           `json:"judgeModel"`
	ScoredAt      int64            `json:"scoredAt"`
}

// bandFor turns the number into the words shown beside it.
func bandFor(n int) string {
	switch {
	case n < 40:
		return "Rework"
	case n < 60:
		return "Weak"
	case n < 75:
		return "Publishable"
	case n < 90:
		return "Strong"
	default:
		return "Exceptional"
	}
}

// judgeVerdict is what one dimension call returns.
type judgeVerdict struct {
	ID    string `json:"id"`
	Score int    `json:"score"`
	Note  string `json:"note"`
}

var reJSONArray = regexp.MustCompile(`(?s)\[.*\]`)

// parseVerdicts pulls the JSON array out of a judge reply.
//
// Lenient by design, matching parseCaptionJSON elsewhere in this package: a
// model that wraps its JSON in a fenced block or a sentence of preamble has
// still answered the question, and failing the whole dimension over
// punctuation would be worse than reading around it.
func parseVerdicts(raw string) ([]judgeVerdict, error) {
	m := reJSONArray.FindString(raw)
	if m == "" {
		return nil, fmt.Errorf("no JSON array in judge reply")
	}
	var out []judgeVerdict
	if err := json.Unmarshal([]byte(m), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// judgePrompt builds the system turn for one dimension.
//
// Deliberately narrow: it carries this dimension's question and criteria and
// nothing else. Giving the judge the other four dimensions is what produces
// the anchoring the per-call split exists to avoid.
func judgePrompt(d dimension, toJudge []criterion, platforms []trendlymodels.Platform, evidence []string) string {
	var sb strings.Builder
	sb.WriteString("You are scoring ONE dimension of a social video script against a fixed rubric.\n\n")
	sb.WriteString("DIMENSION: " + d.Label + "\nTHE QUESTION: " + d.Question + "\n\n")
	if block := platformPlaybookBlock(platforms); block != "" {
		sb.WriteString("The piece targets these platforms, whose rules decide what 'right' means here:\n")
		sb.WriteString(block)
		sb.WriteString("\n")
	}
	if len(evidence) > 0 {
		sb.WriteString("Mechanical checks already found the following. Treat these as evidence, " +
			"not as the answer — they are narrow, and the criteria below are broader:\n")
		for _, e := range evidence {
			sb.WriteString("- " + e + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Score EACH criterion below as exactly 0, 1 or 2:\n")
	sb.WriteString("  2 = clearly met\n  1 = partly met\n  0 = not met\n\n")
	for _, c := range toJudge {
		sb.WriteString("- " + c.ID + ": " + c.Text + "\n")
	}
	sb.WriteString("\nJudge only what is in the script. Do not reward intent, effort or topic interest. " +
		"A criterion the script simply does not address scores 0, not 1.\n\n")
	sb.WriteString("Reply with ONLY a JSON array, one object per criterion, no prose and no code fence:\n")
	sb.WriteString(`[{"id":"` + toJudge[0].ID + `","score":0,"note":"at most 12 words, naming what is missing"}]`)
	return sb.String()
}

// scoreDimension runs one dimension's judge call and merges it with whatever
// the deterministic layer already settled.
func scoreDimension(ctx context.Context, d dimension, script, model string,
	platforms []trendlymodels.Platform, fired map[string]ScriptProblem) (DimensionScore, *openrouter.Usage, error) {

	out := DimensionScore{Key: string(d.Key), Label: d.Label, Weight: d.Weight}

	var toJudge []criterion
	var evidence []string
	byID := map[string]*CriterionScore{}
	for _, c := range d.Criteria {
		if c.Prefill != "" {
			score := 2
			note := ""
			if p, ok := fired[c.Prefill]; ok {
				score = 0
				note = p.Message
			}
			cs := CriterionScore{ID: c.ID, Score: score, Note: note, Prefills: true}
			out.Criteria = append(out.Criteria, cs)
			byID[c.ID] = &out.Criteria[len(out.Criteria)-1]
			continue
		}
		toJudge = append(toJudge, c)
	}
	// Findings this dimension cares about, minus any already consumed as a
	// pre-fill above (showing the judge the answer to a question it is not
	// being asked is noise, and showing it one it IS being asked would be
	// answering for it).
	consumed := map[string]bool{}
	for _, c := range d.Criteria {
		if c.Prefill != "" {
			consumed[c.Prefill] = true
		}
	}
	for _, id := range d.Evidence {
		if consumed[id] {
			continue
		}
		if p, ok := fired[id]; ok {
			evidence = append(evidence, p.Message)
		}
	}
	sort.Strings(evidence)

	var usage *openrouter.Usage
	if len(toJudge) > 0 {
		resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
			Model: model,
			// Low temperature: a judge is scored on reproducibility, and the
			// run-to-run stability target (±4 overall across 5 runs) is not
			// reachable at a conversational temperature.
			Temperature: 0.1,
			MaxTokens:   900,
			Messages: []openrouter.Message{
				{Role: "system", Content: judgePrompt(d, toJudge, platforms, evidence)},
				{Role: "user", Content: script},
			},
		})
		if err != nil {
			return out, nil, err
		}
		usage = resp.Usage
		verdicts, err := parseVerdicts(firstChoiceText(resp))
		if err != nil {
			return out, usage, fmt.Errorf("%s: %w", d.Label, err)
		}
		seen := map[string]bool{}
		for _, v := range verdicts {
			seen[v.ID] = true
			out.Criteria = append(out.Criteria, CriterionScore{
				ID: v.ID, Score: clamp02(v.Score), Note: strings.TrimSpace(v.Note),
			})
		}
		// A criterion the judge skipped scores 0 rather than vanishing: a
		// silently dropped criterion would quietly raise the dimension's
		// average, which is the wrong direction for a missing answer.
		for _, c := range toJudge {
			if !seen[c.ID] {
				out.Criteria = append(out.Criteria, CriterionScore{
					ID: c.ID, Score: 0, Note: "not assessed",
				})
			}
		}
	}

	sort.Slice(out.Criteria, func(i, j int) bool { return out.Criteria[i].ID < out.Criteria[j].ID })

	total := 0
	for _, c := range out.Criteria {
		total += c.Score
	}
	if n := len(out.Criteria); n > 0 {
		out.Score = int(float64(total) / float64(2*n) * 100)
	}
	return out, usage, nil
}

func clamp02(n int) int {
	if n < 0 {
		return 0
	}
	if n > 2 {
		return 2
	}
	return n
}

func firstChoiceText(resp *openrouter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}

// ScoreScript runs the whole rubric over a script.
//
// The five dimension calls run concurrently: they are independent by
// construction, and five sequential round trips would make the panel feel
// broken.
func ScoreScript(ctx context.Context, script, format string, platforms []trendlymodels.Platform,
	model string) (*ViralityScore, *openrouter.Usage, error) {

	clean := strings.TrimSpace(script)
	if clean == "" {
		return nil, nil, fmt.Errorf("nothing to score")
	}

	problems := ValidateScript(script, format)
	fired := map[string]ScriptProblem{}
	for _, p := range problems {
		fired[p.ID] = p
	}

	type result struct {
		idx   int
		dim   DimensionScore
		usage *openrouter.Usage
		err   error
	}
	results := make([]result, len(rubric))
	var wg sync.WaitGroup
	for i, d := range rubric {
		wg.Add(1)
		go func(i int, d dimension) {
			defer wg.Done()
			dim, usage, err := scoreDimension(ctx, d, clean, model, platforms, fired)
			results[i] = result{idx: i, dim: dim, usage: usage, err: err}
		}(i, d)
	}
	wg.Wait()

	total := openrouter.Usage{}
	var firstErr error
	score := &ViralityScore{
		RubricVersion: rubricVersion,
		JudgeModel:    model,
		Problems:      problems,
	}
	for _, r := range results {
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		if r.usage != nil {
			total.PromptTokens += r.usage.PromptTokens
			total.CompletionTokens += r.usage.CompletionTokens
			total.TotalTokens += r.usage.TotalTokens
		}
		score.Dimensions = append(score.Dimensions, r.dim)
	}
	// A single failed dimension is fatal for the run. Reporting four fifths of
	// a rubric as a number out of 100 would understate the piece by exactly
	// the weight of whichever dimension fell over, and nobody looking at the
	// panel could tell.
	if firstErr != nil {
		return nil, &total, firstErr
	}

	weighted := 0
	for _, d := range score.Dimensions {
		weighted += d.Score * d.Weight
	}
	score.Overall = weighted / 100

	applyCaps(score, fired)
	score.Band = bandFor(score.Overall)
	score.Fixes = topFixes(score)
	return score, &total, nil
}

// applyCaps enforces the multiplicative reality the weighted average cannot.
//
// Without these, a fatal hook hides behind a strong body and the number stops
// meaning anything — which is the single most common way a content score ends
// up ignored. Every cap carries its reason.
func applyCaps(s *ViralityScore, fired map[string]ScriptProblem) {
	ceiling := func(at int, reason string) {
		if s.Overall > at {
			s.Overall = at
			s.Cap = &ScoreCap{At: at, Reason: reason}
		}
	}
	byKey := map[string]int{}
	for _, d := range s.Dimensions {
		byKey[d.Key] = d.Score
	}

	if byKey[string(dimHook)] < 40 {
		ceiling(55, "The hook is the gate — nothing downstream gets seen, however good it is.")
	}
	for _, p := range fired {
		if p.Severity == severityCap {
			ceiling(70, "A mechanical problem caps this: "+p.Message)
			break
		}
	}
	if byKey[string(dimShare)] == 0 {
		ceiling(75, "Nothing here gives anyone a reason to send it to someone else.")
	}
}

// topFixes picks the three changes worth making, ranked by the points they
// would actually buy: a criterion's shortfall times its dimension's weight.
//
// A score with no "here is the line to change" is decoration, and ranking by
// raw score alone would send people to fix a 0 in the lightest dimension
// before a 1 in the heaviest.
func topFixes(s *ViralityScore) []Fix {
	type scored struct {
		fix  Fix
		gain int
	}
	var all []scored
	text := map[string]string{}
	for _, d := range rubric {
		for _, c := range d.Criteria {
			text[c.ID] = c.Text
		}
	}
	for _, d := range s.Dimensions {
		for _, c := range d.Criteria {
			if c.Score >= 2 {
				continue
			}
			msg := c.Note
			if msg == "" {
				msg = text[c.ID]
			}
			all = append(all, scored{
				fix:  Fix{Criterion: c.ID, Dimension: d.Label, Message: msg},
				gain: (2 - c.Score) * d.Weight,
			})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].gain > all[j].gain })
	out := make([]Fix, 0, 3)
	for i := 0; i < len(all) && i < 3; i++ {
		out = append(out, all[i].fix)
	}
	return out
}
