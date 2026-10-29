package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/designpreview"
	"github.com/idivarts/backend-sls/pkg/openrouter"
	"golang.org/x/net/html"
)

// Scoring a design rather than a script.
//
// The rubric generalises: a carousel's first slide is its hook, the middle
// slides are its body, the last one carries the ask. What does not generalise
// is the input. A design is a VISUAL artefact, and judging it from its HTML
// source would reward markup — a headline that overflows its slide reads
// identically to one that fits, and the single thing most likely to kill a
// design is exactly that kind of defect.
//
// So this renders the design and looks at it, reusing the preview renderer the
// design loop already uses to check its own work. When the renderer is not
// available the score is still produced from the slide text, and the result
// says so rather than quietly implying something was seen.
//
// One judge call, not five. For a script, splitting per dimension bought
// reproducibility cheaply. Here the frames dominate the bill, and sending the
// same images five times to ask five questions about them is the wasteful part.

// designScoreTokens is generous: one call is carrying all five dimensions.
const designScoreTokens = 3000

// slideText is the visible copy of one slide, in reading order.
type slideText struct {
	Index int
	Lines []string
}

// extractSlideText pulls the rendered copy out of the design, slide by slide.
// Script and style contents are skipped — they are not read by anyone.
func extractSlideText(doc string) []slideText {
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		return nil
	}

	var slides []*html.Node
	var findSlides func(*html.Node)
	findSlides = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "data-slide" {
					slides = append(slides, n)
					return // nested slides are not a thing; stop descending
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findSlides(c)
		}
	}
	findSlides(root)
	// A single-slide design may carry no data-slide at all; treat the document
	// as one slide rather than returning nothing.
	if len(slides) == 0 {
		slides = []*html.Node{root}
	}

	out := make([]slideText, 0, len(slides))
	for i, s := range slides {
		st := slideText{Index: i + 1}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
				return
			}
			if n.Type == html.TextNode {
				if t := strings.Join(strings.Fields(n.Data), " "); t != "" {
					st.Lines = append(st.Lines, t)
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(s)
		out = append(out, st)
	}
	return out
}

func designJudgePrompt(docType string, slides int, platforms []trendlymodels.Platform, saw bool, evidence []string) string {
	var sb strings.Builder
	surface := "a social image post"
	switch {
	case docType == "video":
		surface = "a short social video"
	case slides > 1:
		surface = fmt.Sprintf("a %d-slide social carousel", slides)
	}

	sb.WriteString("You are scoring " + surface + " against a fixed rubric, across five dimensions.\n\n")
	if saw {
		sb.WriteString("You can SEE it: the images are the rendered frames, in order. Judge what is " +
			"actually on them. Copy that overflows its edge, is unreadable at thumbnail size, or " +
			"collides with something else has failed at the thing it was for, however well it reads " +
			"as text.\n\n")
	} else {
		sb.WriteString("The render was unavailable, so you are judging the slide COPY alone. Score " +
			"what the words do and say nothing about the visuals you cannot see.\n\n")
	}

	if block := platformPlaybookBlock(platforms); block != "" {
		sb.WriteString("It targets these platforms, whose rules decide what 'right' means here:\n")
		sb.WriteString(block)
		sb.WriteString("\n")
	}

	// How the script-shaped rubric maps onto a design. Said once, plainly,
	// because a judge left to guess will score a carousel as if it were prose.
	sb.WriteString("HOW THE DIMENSIONS APPLY HERE:\n")
	if slides > 1 {
		sb.WriteString("- Hook: the FIRST slide alone. It is the only one most people will see.\n")
		sb.WriteString("- Body: the middle slides — whether they deliver what the first promised.\n")
		sb.WriteString("- Action: the final slide, and whether it asks for one clear thing.\n")
	} else if docType == "video" {
		sb.WriteString("- Hook: the opening frame and first seconds.\n")
		sb.WriteString("- Body: the middle — whether it keeps the promise the opening made.\n")
		sb.WriteString("- Action: how it closes, and whether it asks for one clear thing.\n")
	} else {
		sb.WriteString("- Hook: what stops the scroll — the dominant line or image.\n")
		sb.WriteString("- Body: the supporting copy. A single image carries little, so score what " +
			"is there rather than penalising it for not being a carousel.\n")
		sb.WriteString("- Action: whether it asks for one clear thing, on the image itself.\n")
	}
	sb.WriteString("- Share: whether anyone would send this to someone.\n")
	sb.WriteString("- Fit: whether it suits the platform and the format it is going to, including " +
		"whether it is legible at the size it will actually be seen.\n\n")

	if len(evidence) > 0 {
		sb.WriteString("Mechanical checks already found the following. Treat these as evidence, not " +
			"as the answer:\n")
		for _, e := range evidence {
			sb.WriteString("- " + e + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Score EACH criterion as exactly 0, 1 or 2, against a demanding bar:\n")
	sb.WriteString("  2 = EXEMPLARY. Not merely correct — distinctly good.\n")
	sb.WriteString("  1 = ADEQUATE. Met, but unremarkably. Most criteria on competent work land here.\n")
	sb.WriteString("  0 = NOT MET, or not addressed at all.\n\n")
	sb.WriteString("Do not award 2 for the absence of a mistake.\n\n")

	for _, d := range rubric {
		sb.WriteString(d.Label + " — " + d.Question + "\n")
		for _, c := range d.Criteria {
			sb.WriteString("  - " + c.ID + ": " + c.Text + "\n")
		}
	}
	sb.WriteString("\nWhere a criterion names something only a script has (a spoken line, a scene), " +
		"read it as its visual equivalent. A criterion the piece genuinely does not address scores " +
		"0, not 1.\n\n")
	sb.WriteString("Write a note ONLY when the score is 0 or 1, under 12 words, naming what is " +
		"missing. For a 2 leave it empty.\n\n")
	sb.WriteString("Reply with ONLY a JSON array of every criterion, no prose and no code fence:\n")
	sb.WriteString(`[{"id":"H1","score":1,"note":"under 12 words"}]`)
	return sb.String()
}

// ScoreDesign scores the content's current design against the virality rubric.
// `saw` in the result's judge note records whether the model actually looked.
func ScoreDesign(ctx context.Context, brandID, contentID, model string) (*ViralityScore, *openrouter.Usage, error) {
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return nil, nil, fmt.Errorf("content not found")
	}
	if content.DesignRef == nil || content.DesignRef.RevisionID == "" {
		return nil, nil, fmt.Errorf("this content has no design yet")
	}
	rev, err := trendlymodels.GetDesignRevision(brandID, contentID, content.DesignRef.RevisionID)
	if err != nil || rev == nil {
		return nil, nil, fmt.Errorf("current design not found")
	}

	// Deterministic first, exactly as for a script: these cost nothing and they
	// are facts rather than opinions, so they cap the score and pre-load the
	// judge's evidence.
	var problems []ScriptProblem
	for _, p := range validateDesignHTML(rev.HTML, rev.DocType, 0) {
		problems = append(problems, ScriptProblem{
			ID: "G1", Severity: severityCap, Message: string(p),
		})
	}
	evidence := make([]string, 0, len(problems))
	for _, p := range problems {
		evidence = append(evidence, p.Message)
	}

	// Look at it. A failure here degrades the score rather than failing it, but
	// it is recorded — a score taken without seeing the design is a weaker
	// claim and the caller is told which one they got.
	var images []string
	saw := false
	if res, rerr := designpreview.Render(brandID, contentID, rev.ID, nil, previewMaxEdge); rerr == nil && res != nil && res.OK {
		images = res.Images
		saw = len(images) > 0
	} else if rerr != nil && !errors.Is(rerr, designpreview.ErrNotConfigured) {
		// Worth knowing about, but not worth refusing to score over.
		fmt.Printf("design score: preview failed brand=%s content=%s: %v\n", brandID, contentID, rerr)
	}

	slides := extractSlideText(rev.HTML)
	var body strings.Builder
	for _, s := range slides {
		body.WriteString(fmt.Sprintf("SLIDE %d\n", s.Index))
		if len(s.Lines) == 0 {
			body.WriteString("(no text)\n")
		}
		for _, l := range s.Lines {
			body.WriteString("  " + l + "\n")
		}
		body.WriteString("\n")
	}
	if content.Caption != "" {
		body.WriteString("CAPTION (published alongside it):\n" + content.Caption + "\n")
	}

	platforms := content.Platforms
	sys := designJudgePrompt(rev.DocType, len(slides), platforms, saw, evidence)

	user := openrouter.Message{Role: "user", Content: body.String()}
	if saw {
		user = openrouter.UserMessageWithImages(body.String(), images)
	}

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:       model,
		Temperature: 0,
		MaxTokens:   designScoreTokens,
		Messages: []openrouter.Message{
			{Role: "system", Content: sys},
			user,
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

	byID := map[string]judgeVerdict{}
	for _, v := range verdicts {
		byID[v.ID] = v
	}

	score := &ViralityScore{
		RubricVersion: rubricVersion,
		JudgeModel:    model,
		ScoredAt:      time.Now().UnixMilli(),
		Problems:      problems,
		Dimensions:    make([]DimensionScore, 0, len(rubric)),
		Fixes:         []Fix{},
	}
	for _, d := range rubric {
		ds := DimensionScore{Key: string(d.Key), Label: d.Label, Weight: d.Weight}
		total := 0
		for _, c := range d.Criteria {
			v, ok := byID[c.ID]
			cs := CriterionScore{ID: c.ID, Score: clamp02(v.Score), Note: strings.TrimSpace(v.Note)}
			if !ok {
				// A criterion the judge skipped scores 0 rather than vanishing:
				// dropping it would quietly raise the dimension's average.
				cs = CriterionScore{ID: c.ID, Score: 0, Note: "not assessed"}
			}
			ds.Criteria = append(ds.Criteria, cs)
			total += cs.Score
		}
		if n := len(ds.Criteria); n > 0 {
			ds.Score = int(float64(total) / float64(2*n) * 100)
		}
		score.Dimensions = append(score.Dimensions, ds)
	}

	weighted, weight := 0, 0
	for _, d := range score.Dimensions {
		weighted += d.Score * d.Weight
		weight += d.Weight
	}
	if weight > 0 {
		score.Overall = weighted / weight
	}

	fired := map[string]ScriptProblem{}
	for _, p := range problems {
		fired[p.ID] = p
	}
	applyCaps(score, fired)
	score.Band = bandFor(score.Overall)
	score.Fixes = topFixes(score)

	if !saw {
		score.JudgeModel = model + " (copy only — the design was not rendered)"
	}
	return score, usage, nil
}
