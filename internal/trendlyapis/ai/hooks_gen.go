package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// Hook laddering.
//
// The hook is the one multiplicative gate in the whole funnel: every platform
// ranking signal downstream of it is conditional on someone not scrolling past.
// It is also about fifteen words long, which makes it the cheapest thing in the
// product to iterate on — five variants cost less than one script, and the
// judge only has to read five short lines rather than five whole scripts.
//
// So this does not generate five scripts. It generates five hooks for the body
// the user already has, scores them against the Hook dimension of the same
// rubric the panel uses, and returns them ranked with the archetype each one
// is playing. The user picks; the machine does not pick for them.

// HookCandidate is one hook, scored.
type HookCandidate struct {
	Text string `json:"text"`
	// Which archetype from the pattern library this plays, in the model's own
	// words. Shown so a user can see they are being offered five genuinely
	// different angles rather than five rewrites of one.
	Archetype string `json:"archetype"`
	/// On-screen text to pair with the spoken line, when the two should differ.
	OnScreen string `json:"onScreen,omitempty"`
	Score    int    `json:"score"`
	// Why it scored what it did, only when something was marked down.
	Notes    []string         `json:"notes,omitempty"`
	Criteria []CriterionScore `json:"criteria"`
	Problems []ScriptProblem  `json:"problems,omitempty"`
}

// HookLadder is what the endpoint returns.
type HookLadder struct {
	Hooks         []HookCandidate `json:"hooks"`
	RubricVersion string          `json:"rubricVersion"`
	JudgeModel    string          `json:"judgeModel"`
	GeneratedAt   int64           `json:"generatedAt"`
}

// How many to produce. Five is a judgement about choice, not a technical
// limit: three is too few to contain a surprise, and a list long enough to
// need scrolling turns a decision into a chore.
const hookCount = 5

type hookBrief struct {
	Topic     string
	Title     string
	Body      string // the script the hooks have to open
	Format    string
	Platforms []trendlymodels.Platform
}

func hookGenPrompt(b hookBrief) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		"Write %d DIFFERENT opening hooks for one social video. Each must be able to open the "+
			"same body, which is given below — you are not rewriting the video, only its first "+
			"one to three seconds.\n\n", hookCount))

	// The Phase 0 knowledge layers, which is the whole point of having built
	// them: the craft contract sets the bar, the playbooks say what "right"
	// means on the target platforms, and the pattern library is the menu of
	// archetypes to spread the five across.
	sb.WriteString(viralityCraftInstructions)
	if block := platformPlaybookBlock(b.Platforms); block != "" {
		sb.WriteString(block)
	}
	sb.WriteString(viralityPatternsBlock)

	sb.WriteString("\n--- HOW TO SPREAD THE FIVE ---\n")
	sb.WriteString("Use a DIFFERENT archetype for each. Five variations on one archetype is one " +
		"hook with five haircuts, and gives the user nothing to choose between.\n")
	sb.WriteString("Every hook must be honest about what the body actually delivers. A hook the " +
		"body does not pay off is the single most expensive mistake here: it buys the view and " +
		"loses the watch time, the follow and the reshare in one go.\n")
	sb.WriteString("Keep each spoken line under 20 words. Lead with the specific thing — the " +
		"number, the claim, the stake — not a preamble reaching it.\n")
	sb.WriteString("Where on-screen text should differ from the spoken line, give both. Where the " +
		"same words work, leave onScreen empty.\n")
	if b.Format != "" {
		sb.WriteString("Format: " + b.Format + ".\n")
	}

	sb.WriteString("\nReply with ONLY a JSON array, no prose and no code fence:\n")
	sb.WriteString(`[{"text":"the spoken line","archetype":"which pattern, 2-4 words","onScreen":"optional"}]`)
	return sb.String()
}

// GenerateHooks produces the candidates, unscored.
func GenerateHooks(ctx context.Context, b hookBrief, model string) ([]HookCandidate, *openrouter.Usage, error) {
	if strings.TrimSpace(b.Body) == "" && strings.TrimSpace(b.Topic) == "" {
		return nil, nil, fmt.Errorf("nothing to write hooks for")
	}

	var user strings.Builder
	if b.Title != "" {
		user.WriteString("TITLE: " + b.Title + "\n")
	}
	if b.Topic != "" {
		user.WriteString("TOPIC: " + b.Topic + "\n")
	}
	if body := plainScript(b.Body); strings.TrimSpace(body) != "" {
		user.WriteString("\nTHE BODY THESE HOOKS MUST OPEN:\n" + body)
	}

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model: model,
		// Unlike the judge, this one WANTS spread — five hooks at temperature 0
		// would be five attempts at the same idea.
		Temperature: 0.9,
		MaxTokens:   1200,
		Messages: []openrouter.Message{
			{Role: "system", Content: hookGenPrompt(b)},
			{Role: "user", Content: user.String()},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason == "length" {
		return nil, resp.Usage, fmt.Errorf("hook reply hit the token cap before closing its JSON")
	}

	raw := reJSONArray.FindString(firstChoiceText(resp))
	if raw == "" {
		return nil, resp.Usage, fmt.Errorf("no JSON array in hook reply (began %q)",
			truncate(strings.TrimSpace(firstChoiceText(resp)), 120))
	}
	var parsed []struct {
		Text      string `json:"text"`
		Archetype string `json:"archetype"`
		OnScreen  string `json:"onScreen"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, resp.Usage, err
	}

	out := make([]HookCandidate, 0, len(parsed))
	for _, p := range parsed {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			continue
		}
		out = append(out, HookCandidate{
			Text:      text,
			Archetype: strings.TrimSpace(p.Archetype),
			OnScreen:  strings.TrimSpace(p.OnScreen),
		})
	}
	if len(out) == 0 {
		return nil, resp.Usage, fmt.Errorf("hook reply contained no usable hooks")
	}
	return out, resp.Usage, nil
}

// hookRankPrompt scores every candidate in ONE call.
//
// One call rather than one per hook, for two reasons. It is five times cheaper,
// and — the part that actually matters — a judge holding all five at once ranks
// them against each other instead of against its own drifting sense of a good
// hook. Ranking is the output here, so relative judgement is the right kind.
func hookRankPrompt(d dimension, cands []HookCandidate, platforms []trendlymodels.Platform) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		"You are scoring %d candidate OPENING HOOKS for the same social video against a fixed "+
			"rubric. Score each one independently, but you are seeing them together so that "+
			"'exemplary' means exemplary among what is achievable here.\n\n", len(cands)))
	sb.WriteString("THE QUESTION: " + d.Question + "\n\n")
	if block := platformPlaybookBlock(platforms); block != "" {
		sb.WriteString("The piece targets these platforms, whose rules decide what 'right' means here:\n")
		sb.WriteString(block)
		sb.WriteString("\n")
	}
	sb.WriteString("Score EACH criterion for EACH hook as exactly 0, 1 or 2:\n")
	sb.WriteString("  2 = EXEMPLARY. Not merely correct — distinctly good.\n")
	sb.WriteString("  1 = ADEQUATE. Met, but unremarkably. Most criteria on a decent hook land here.\n")
	sb.WriteString("  0 = NOT MET, or not addressed.\n\n")
	sb.WriteString("Do not award 2 for the absence of a mistake. A hook that merely avoids being " +
		"bad is a 1.\n\n")
	for _, c := range d.Criteria {
		sb.WriteString("- " + c.ID + ": " + c.Text + "\n")
	}
	sb.WriteString("\nA hook is one to three seconds of a video. Judge it as an opening, not as a " +
		"complete piece: it is not required to deliver the payoff, only to earn the next seconds " +
		"honestly.\n\n")
	sb.WriteString("Write a note ONLY when the score is 0 or 1, under 12 words, naming what is " +
		"missing. For a 2 leave it empty.\n\n")
	sb.WriteString("Reply with ONLY a JSON array, one object per hook, in the order given, no prose " +
		"and no code fence:\n")
	sb.WriteString(`[{"hook":1,"criteria":[{"id":"H1","score":1,"note":"under 12 words"}]}]`)
	return sb.String()
}

type hookVerdict struct {
	Hook     int            `json:"hook"`
	Criteria []judgeVerdict `json:"criteria"`
}

func hookDimension() dimension {
	for _, d := range rubric {
		if d.Key == dimHook {
			return d
		}
	}
	// rubric is a package-level literal; a missing Hook dimension is a
	// programming error, not a runtime condition.
	panic("rubric has no hook dimension")
}

// rankHooks scores the candidates in place and sorts them best first.
func rankHooks(ctx context.Context, cands []HookCandidate, format string,
	platforms []trendlymodels.Platform, model string) (*openrouter.Usage, error) {

	d := hookDimension()

	// The deterministic checks run per hook, on the hook alone. Only the ones
	// that are about an opening apply — D1 would fire on every candidate (a
	// hook has no body or CTA) and would say nothing.
	for i := range cands {
		cands[i].Problems = hookProblems(cands[i].Text)
	}

	var user strings.Builder
	for i, c := range cands {
		user.WriteString(fmt.Sprintf("HOOK %d", i+1))
		if c.Archetype != "" {
			user.WriteString(" [" + c.Archetype + "]")
		}
		user.WriteString("\nSpoken: " + c.Text + "\n")
		if c.OnScreen != "" {
			user.WriteString("On-screen: " + c.OnScreen + "\n")
		}
		user.WriteString("\n")
	}

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:       model,
		Temperature: 0,
		// Six criteria across five hooks. Generous on purpose: this is the one
		// call in the feature, and a truncated reply loses the whole ladder.
		MaxTokens: 600 + 300*len(cands)*len(d.Criteria)/2,
		Messages: []openrouter.Message{
			{Role: "system", Content: hookRankPrompt(d, cands, platforms)},
			{Role: "user", Content: user.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason == "length" {
		return resp.Usage, fmt.Errorf("hook ranking hit the token cap before closing its JSON")
	}

	raw := reJSONArray.FindString(firstChoiceText(resp))
	if raw == "" {
		return resp.Usage, fmt.Errorf("no JSON array in ranking reply (began %q)",
			truncate(strings.TrimSpace(firstChoiceText(resp)), 120))
	}
	var verdicts []hookVerdict
	if err := json.Unmarshal([]byte(raw), &verdicts); err != nil {
		return resp.Usage, err
	}

	for _, v := range verdicts {
		idx := v.Hook - 1
		if idx < 0 || idx >= len(cands) {
			continue
		}
		applyHookVerdict(&cands[idx], d, v.Criteria)
	}
	// A hook the judge skipped keeps score 0 and says so, rather than being
	// dropped: five hooks were generated and paid for, and silently returning
	// four would look like the model's choice rather than a failed parse.
	for i := range cands {
		if len(cands[i].Criteria) == 0 {
			cands[i].Notes = append(cands[i].Notes, "not assessed")
		}
	}

	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	return resp.Usage, nil
}

func applyHookVerdict(c *HookCandidate, d dimension, verdicts []judgeVerdict) {
	seen := map[string]bool{}
	for _, v := range verdicts {
		seen[v.ID] = true
		score := clamp02(v.Score)
		note := strings.TrimSpace(v.Note)
		c.Criteria = append(c.Criteria, CriterionScore{ID: v.ID, Score: score, Note: note})
		if score < 2 && note != "" {
			c.Notes = append(c.Notes, note)
		}
	}
	for _, cr := range d.Criteria {
		if !seen[cr.ID] {
			c.Criteria = append(c.Criteria, CriterionScore{ID: cr.ID, Score: 0, Note: "not assessed"})
		}
	}
	sort.Slice(c.Criteria, func(i, j int) bool { return c.Criteria[i].ID < c.Criteria[j].ID })

	total := 0
	for _, cr := range c.Criteria {
		total += cr.Score
	}
	if n := len(c.Criteria); n > 0 {
		c.Score = int(float64(total) / float64(2*n) * 100)
	}
	// A deterministic failure is not an opinion, so it caps rather than
	// nudges: a hook that opens with throat-clearing cannot be the best of
	// five however well it reads otherwise.
	for _, p := range c.Problems {
		if p.Severity != severityWarn && c.Score > 60 {
			c.Score = 60
		}
	}
}

// BuildHookLadder generates and ranks in one go.
func BuildHookLadder(ctx context.Context, b hookBrief, model string) (*HookLadder, *openrouter.Usage, error) {
	cands, genUsage, err := GenerateHooks(ctx, b, model)
	total := genUsage
	if err != nil {
		return nil, total, err
	}

	rankUsage, err := rankHooks(ctx, cands, b.Format, b.Platforms, model)
	total = sumUsage(total, rankUsage)
	if err != nil {
		// The hooks themselves are still worth having: an unranked ladder is a
		// worse product than a ranked one, not a failure. The caller decides,
		// and the error says what was lost.
		return &HookLadder{Hooks: cands, RubricVersion: rubricVersion, JudgeModel: model}, total, err
	}

	return &HookLadder{
		Hooks:         cands,
		RubricVersion: rubricVersion,
		JudgeModel:    model,
	}, total, nil
}

// sumUsage folds two optional usages together so the wallet is metered for
// everything that was actually spent, including on the failure path.
func sumUsage(a, b *openrouter.Usage) *openrouter.Usage {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := *a
	out.PromptTokens += b.PromptTokens
	out.CompletionTokens += b.CompletionTokens
	out.TotalTokens += b.TotalTokens
	return &out
}
