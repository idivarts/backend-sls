package ai

// Scoring a Design Studio v3 film.
//
// ── Why this is a second entry point and not a parameter ──────────────────
// The v2 scorer reaches a design through a CONTENT: content → DesignRef →
// revision → HTML, then renders that HTML to look at it. A v3 design hangs
// off a brand, has no revision and no HTML in Firestore, and — crucially —
// has already been rendered. Threading all of that through one function would
// mean a function that is mostly branches. The RUBRIC is what must not fork,
// and it does not: both paths call assembleDesignScore with the same
// verdicts, the same caps and the same bands.
//
// ── Why it judges the preview frames ──────────────────────────────────────
// This is the part v2 had to work for. A design is a visual artefact, and
// judging it from source rewards markup: a headline that overflows reads
// identically to one that fits, and that is the single most likely way a
// design is bad. v2 renders on demand to get pictures. v3 already HAS
// pictures — the free preview captured real frames off the same machinery
// that renders the film — so the best available evidence is sitting on the
// document, costing nothing to reuse.
//
// It follows that a design with no preview cannot be scored, and this says so
// rather than quietly scoring the text and implying something was seen.

import (
	"context"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// MaxScoreFrames bounds what goes to the judge.
//
// Every frame is an image on a vision call, and the bill scales with them
// while the judgement stops improving — six stills across a ten-second film
// already show every scene. A longer film gets a spread, not a slideshow.
const MaxScoreFrames = 6

// ScoreDesignV3 judges a v3 design from the frames its preview captured.
func ScoreDesignV3(ctx context.Context, brandID, designID, model string) (*ViralityScore, *openrouter.Usage, error) {
	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read the design")
	}
	if d == nil {
		return nil, nil, fmt.Errorf("no such design")
	}
	if len(d.PreviewFrames) == 0 {
		// Deliberately an error, not a text-only score. "We looked at it" and
		// "we read its scene list" are different claims and the second one
		// must not be able to masquerade as the first.
		return nil, nil, fmt.Errorf("preview this design first — the score is taken from its frames")
	}

	images := pickScoreFrames(d.PreviewFrames, MaxScoreFrames)

	var body strings.Builder
	body.WriteString(fmt.Sprintf("A %.1f-second vertical film, %d×%d, in %d scenes.\n\n",
		float64(d.DurationMs)/1000, d.Format.Width, d.Format.Height, len(d.Scenes)))
	for i, s := range d.Scenes {
		body.WriteString(fmt.Sprintf("SCENE %d — %s (%.1fs to %.1fs)\n",
			i+1, s.ID, float64(s.StartMs)/1000, float64(s.StartMs+s.DurationMs)/1000))
	}
	if d.Brief != "" {
		// What the user ASKED for. Without it the judge can only say whether
		// the film is good, not whether it is the film that was wanted.
		body.WriteString("\nWHAT WAS ASKED FOR:\n" + d.Brief + "\n")
	}
	body.WriteString("\nThe images are frames from this film, in order.\n")

	// A v3 film is a reel: one timed sequence, not a carousel of slides. The
	// rubric's hook/body/ask structure maps onto its scenes.
	sys := designJudgePrompt("video", len(d.Scenes), nil, true, nil)

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:       model,
		Temperature: 0,
		MaxTokens:   designScoreTokens,
		Messages: []openrouter.Message{
			{Role: "system", Content: sys},
			openrouter.UserMessageWithImages(body.String(), images),
		},
	})
	if err != nil {
		return nil, nil, err
	}
	usage := resp.Usage
	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason == "length" {
		return nil, usage, fmt.Errorf("the judge's reply hit the token cap before closing its JSON")
	}
	verdicts, err := parseVerdicts(firstChoiceText(resp))
	if err != nil {
		return nil, usage, fmt.Errorf("%w (reply began %q)", err,
			truncate(strings.TrimSpace(firstChoiceText(resp)), 120))
	}

	// No deterministic problems to feed in: v2's come from validating stored
	// HTML, and v3's equivalent gate is Hyperframes' own `check`, which has
	// already refused anything that would fail. Passing none is honest — it
	// means no cap fired, not that nothing was checked.
	return assembleDesignScore(verdicts, nil, model, true), usage, nil
}

// pickScoreFrames spreads the choice across the film rather than taking the
// first n, which on a long film would judge the opening and never see the ask.
func pickScoreFrames(frames []trendlymodels.PreviewFrame, max int) []string {
	if max <= 0 || len(frames) == 0 {
		return nil
	}
	if len(frames) <= max {
		out := make([]string, 0, len(frames))
		for _, f := range frames {
			out = append(out, f.URL)
		}
		return out
	}
	out := make([]string, 0, max)
	// Evenly spaced including both ends: the first frame is the thumbnail
	// social players use and the last carries the ask, so neither may be the
	// one that gets dropped.
	for i := 0; i < max; i++ {
		idx := i * (len(frames) - 1) / (max - 1)
		out = append(out, frames[idx].URL)
	}
	return out
}
