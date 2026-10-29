package ai

import (
	"strings"
	"testing"
)

func TestRubricWeightsSumTo100(t *testing.T) {
	total := 0
	for _, d := range rubric {
		total += d.Weight
	}
	if total != 100 {
		t.Fatalf("weights sum to %d, not 100 — every overall score would be wrong", total)
	}
}

func TestRubricCriterionIDsAreUnique(t *testing.T) {
	// Ids are the contract with the Phase 2 panel and with the deterministic
	// pre-fill. A duplicate would silently overwrite a judgement.
	seen := map[string]string{}
	for _, d := range rubric {
		for _, c := range d.Criteria {
			if prev, dup := seen[c.ID]; dup {
				t.Errorf("criterion %s appears in both %s and %s", c.ID, prev, d.Label)
			}
			seen[c.ID] = d.Label
		}
	}
}

// Every Prefill must name a check the validator can actually emit, or the
// criterion silently scores a free 2 forever.
func TestPrefillsNameRealChecks(t *testing.T) {
	emitted := map[string]bool{}
	for _, s := range []string{
		"", "## Hook\nhey guys\n## CTA\nlike if you agree",
		strings.Repeat("word ", 600) + "\n## Hook\nx\n## CTA\ny",
	} {
		for _, p := range ValidateScript(s, "reel") {
			emitted[p.ID] = true
		}
	}
	// Ids the validator defines but these samples may not trigger.
	for _, known := range []string{"D1", "D2", "D6", "D7", "D7b", "D8", "D9", "D10", "D13"} {
		emitted[known] = true
	}
	for _, d := range rubric {
		for _, c := range d.Criteria {
			if c.Prefill != "" && !emitted[c.Prefill] {
				t.Errorf("%s pre-fills from %q, which no check emits", c.ID, c.Prefill)
			}
		}
	}
}

func TestBandFor(t *testing.T) {
	cases := map[int]string{
		0: "Rework", 39: "Rework",
		40: "Weak", 59: "Weak",
		60: "Publishable", 74: "Publishable",
		75: "Strong", 89: "Strong",
		90: "Exceptional", 100: "Exceptional",
	}
	for n, want := range cases {
		if got := bandFor(n); got != want {
			t.Errorf("bandFor(%d) = %q, want %q", n, got, want)
		}
	}
}

func dims(hook, body, action, share, fit int) []DimensionScore {
	return []DimensionScore{
		{Key: string(dimHook), Label: "Hook", Weight: 30, Score: hook},
		{Key: string(dimBody), Label: "Body", Weight: 25, Score: body},
		{Key: string(dimAction), Label: "Action", Weight: 15, Score: action},
		{Key: string(dimShare), Label: "Share", Weight: 20, Score: share},
		{Key: string(dimFit), Label: "Fit", Weight: 10, Score: fit},
	}
}

func TestCapsFireWithTheirReason(t *testing.T) {
	t.Run("a weak hook ceilings the overall", func(t *testing.T) {
		// Everything else excellent: the weighted average would be 79.
		s := &ViralityScore{Overall: 79, Dimensions: dims(30, 100, 100, 100, 100)}
		applyCaps(s, nil)
		if s.Overall != 55 {
			t.Errorf("overall = %d, want 55", s.Overall)
		}
		if s.Cap == nil || s.Cap.Reason == "" {
			t.Fatal("a cap must carry its reason; a silent ceiling looks like a bug")
		}
		if !strings.Contains(s.Cap.Reason, "gate") {
			t.Errorf("reason should explain the gate, got %q", s.Cap.Reason)
		}
	})

	t.Run("no share trigger ceilings at 75", func(t *testing.T) {
		s := &ViralityScore{Overall: 80, Dimensions: dims(100, 100, 100, 0, 100)}
		applyCaps(s, nil)
		if s.Overall != 75 {
			t.Errorf("overall = %d, want 75", s.Overall)
		}
	})

	t.Run("a mechanical problem ceilings at 70", func(t *testing.T) {
		s := &ViralityScore{Overall: 95, Dimensions: dims(100, 100, 100, 100, 100)}
		applyCaps(s, map[string]ScriptProblem{
			"D8": {ID: "D8", Severity: severityCap, Message: "bait"},
		})
		if s.Overall != 70 {
			t.Errorf("overall = %d, want 70", s.Overall)
		}
	})

	t.Run("warnings do not cap", func(t *testing.T) {
		s := &ViralityScore{Overall: 95, Dimensions: dims(100, 100, 100, 100, 100)}
		applyCaps(s, map[string]ScriptProblem{
			"D10": {ID: "D10", Severity: severityWarn, Message: "no cues"},
		})
		if s.Overall != 95 {
			t.Errorf("a warning must not cap: got %d", s.Overall)
		}
	})

	t.Run("a good score is left alone", func(t *testing.T) {
		s := &ViralityScore{Overall: 88, Dimensions: dims(90, 85, 90, 85, 90)}
		applyCaps(s, nil)
		if s.Overall != 88 || s.Cap != nil {
			t.Errorf("unexpected cap: %d %+v", s.Overall, s.Cap)
		}
	})

	t.Run("the lowest applicable ceiling wins", func(t *testing.T) {
		// Both a weak hook (55) and a mechanical problem (70) apply.
		s := &ViralityScore{Overall: 90, Dimensions: dims(10, 100, 100, 100, 100)}
		applyCaps(s, map[string]ScriptProblem{
			"D8": {ID: "D8", Severity: severityCap, Message: "bait"},
		})
		if s.Overall != 55 {
			t.Errorf("overall = %d, want the strictest ceiling 55", s.Overall)
		}
	})
}

// A fix list ranked by raw score would send someone to repair a 0 in the
// 10-point dimension before a 1 in the 30-point one.
func TestTopFixesRankByPointsTheyWouldBuy(t *testing.T) {
	s := &ViralityScore{Dimensions: []DimensionScore{
		{Key: "fit", Label: "Fit", Weight: 10, Criteria: []CriterionScore{{ID: "F1", Score: 0, Note: "fit zero"}}},
		{Key: "hook", Label: "Hook", Weight: 30, Criteria: []CriterionScore{{ID: "H1", Score: 1, Note: "hook partial"}}},
		{Key: "body", Label: "Body", Weight: 25, Criteria: []CriterionScore{{ID: "B1", Score: 0, Note: "body zero"}}},
	}}
	fixes := topFixes(s)
	if len(fixes) != 3 {
		t.Fatalf("want 3 fixes, got %d", len(fixes))
	}
	// body zero = 2*25 = 50, fit zero = 2*10 = 20, hook partial = 1*30 = 30.
	if fixes[0].Criterion != "B1" {
		t.Errorf("first fix should be the one worth most (B1, 50 pts), got %s", fixes[0].Criterion)
	}
	if fixes[1].Criterion != "H1" {
		t.Errorf("second fix should be H1 (30 pts), got %s", fixes[1].Criterion)
	}
}

func TestTopFixesSkipsSatisfiedCriteria(t *testing.T) {
	s := &ViralityScore{Dimensions: []DimensionScore{
		{Key: "hook", Label: "Hook", Weight: 30, Criteria: []CriterionScore{
			{ID: "H1", Score: 2}, {ID: "H2", Score: 2},
		}},
	}}
	if got := topFixes(s); len(got) != 0 {
		t.Errorf("a clean dimension should produce no fixes, got %v", got)
	}
}

func TestTopFixesFallsBackToTheCriterionText(t *testing.T) {
	s := &ViralityScore{Dimensions: []DimensionScore{
		{Key: "hook", Label: "Hook", Weight: 30, Criteria: []CriterionScore{{ID: "H1", Score: 0}}},
	}}
	fixes := topFixes(s)
	if len(fixes) != 1 || fixes[0].Message == "" {
		t.Fatalf("a fix with no judge note must still say something: %+v", fixes)
	}
}

func TestParseVerdictsIsLenient(t *testing.T) {
	cases := []string{
		`[{"id":"H1","score":2,"note":"fine"}]`,
		"```json\n[{\"id\":\"H1\",\"score\":2,\"note\":\"fine\"}]\n```",
		"Here is my assessment:\n[{\"id\":\"H1\",\"score\":2,\"note\":\"fine\"}]\nHope that helps.",
	}
	for _, raw := range cases {
		got, err := parseVerdicts(raw)
		if err != nil {
			t.Errorf("%q: %v", raw, err)
			continue
		}
		if len(got) != 1 || got[0].ID != "H1" || got[0].Score != 2 {
			t.Errorf("%q parsed to %+v", raw, got)
		}
	}
	if _, err := parseVerdicts("I cannot score this."); err == nil {
		t.Error("a reply with no array should error rather than score zero silently")
	}
}

func TestClamp02(t *testing.T) {
	for in, want := range map[int]int{-5: 0, 0: 0, 1: 1, 2: 2, 7: 2} {
		if got := clamp02(in); got != want {
			t.Errorf("clamp02(%d) = %d, want %d", in, got, want)
		}
	}
}

// A prefill answers a criterion outright, so it is only safe where the check
// is EQUIVALENT to the criterion rather than merely related.
//
// This started wrong. P3 ("exactly one ask, AND it is the action this platform
// rewards") pre-filled from the ask COUNT, so every script with one ask scored
// a free 2 on the half that actually matters — whether it was the right ask.
// Four criteria were inflated that way. Narrower checks now reach the judge as
// evidence instead.
func TestOnlyEquivalentChecksPrefill(t *testing.T) {
	allowed := map[string]string{
		"H4": "D2",  // "no throat-clearing" is exactly what D2 detects
		"P4": "D8",  // "no engagement bait" is exactly what D8 detects
		"F2": "D13", // "respects the length band" is exactly what D13 measures
		"F3": "D10", // "sound-off cues present" is exactly what D10 looks for
	}
	for _, d := range rubric {
		for _, c := range d.Criteria {
			if c.Prefill == "" {
				continue
			}
			want, ok := allowed[c.ID]
			if !ok {
				t.Errorf("%s (%q) pre-fills from %s, but it asks more than any single check answers — "+
					"pre-filling it would score it for free", c.ID, c.Text, c.Prefill)
				continue
			}
			if want != c.Prefill {
				t.Errorf("%s pre-fills from %s, expected %s", c.ID, c.Prefill, want)
			}
		}
	}
}

// The criteria carrying the real judgement must reach the judge.
func TestJudgementCriteriaAreNotPrefilled(t *testing.T) {
	mustJudge := map[string]bool{"H2": true, "P1": true, "P3": true, "B3": true, "B5": true, "B6": true}
	for _, d := range rubric {
		for _, c := range d.Criteria {
			if mustJudge[c.ID] && c.Prefill != "" {
				t.Errorf("%s must be judged, not pre-filled from %s", c.ID, c.Prefill)
			}
		}
	}
}

// The scale anchors are load-bearing and easy to soften by accident.
//
// The first version said "2 = clearly met", and competent work cleared that
// without effort: Hook and Body saturated at 100 on every real dev script, so
// the score separated bad from good and told someone iterating on a good
// script nothing at all. Reserving 2 for exemplary execution is what put the
// useful range back — measured on dev, a decent script moved from 91
// "Exceptional" to 85 "Strong", and the weak control from 17 to 5.
func TestJudgePromptKeepsADemandingBar(t *testing.T) {
	d := rubric[0]
	p := judgePrompt(d, d.Criteria, nil, nil)
	for _, must := range []string{"EXEMPLARY", "ADEQUATE", "NOT MET"} {
		if !strings.Contains(p, must) {
			t.Errorf("the scale anchor %q is gone", must)
		}
	}
	if !strings.Contains(p, "absence of a mistake") {
		t.Error("the 'do not award 2 for the absence of a mistake' instruction is gone")
	}
	if strings.Contains(p, "2 = clearly met") {
		t.Error("the soft anchor is back; it saturates Hook and Body at 100")
	}
}

// Evidence must not leak across dimensions: that is the contamination the
// one-call-per-dimension split exists to prevent.
func TestJudgePromptEvidenceIsScoped(t *testing.T) {
	for _, d := range rubric {
		for _, id := range d.Evidence {
			var ownedElsewhere bool
			for _, other := range rubric {
				if other.Key == d.Key {
					continue
				}
				for _, c := range other.Criteria {
					if c.Prefill == id {
						ownedElsewhere = true
					}
				}
			}
			if ownedElsewhere {
				// Not fatal — D10 and D13 legitimately inform both Body and
				// Fit — but a check that another dimension PRE-FILLS should
				// not also be loose evidence here.
				t.Logf("note: %s lists evidence %s which another dimension pre-fills", d.Label, id)
			}
		}
	}
}

// The stored document must be the API shape, key for key. Firestore marshals by
// `firestore:` tag and falls back to the Go field name, never the json tag, so
// persisting the struct directly wrote Band / Overall / ScoredAt and every
// reader saw undefined.
func TestFirestoreMapMatchesTheAPIShape(t *testing.T) {
	s := &ViralityScore{
		Overall:       84,
		Band:          "Strong",
		Cap:           &ScoreCap{At: 70, Reason: "a check fired"},
		RubricVersion: rubricVersion,
		JudgeModel:    "google/gemini-3.5-flash",
		ScoredAt:      1_793_000_000_000,
		Dimensions: []DimensionScore{{
			Key: "hook", Label: "Hook", Weight: 30, Score: 100,
			Criteria: []CriterionScore{{ID: "H1", Score: 2}, {ID: "H4", Score: 0, Note: "n", Prefills: true}},
		}},
		Fixes:    []Fix{{Criterion: "S2", Dimension: "Share", Message: "m"}},
		Problems: []ScriptProblem{{ID: "D2", Severity: severityCap, Message: "msg"}},
	}

	doc, err := s.FirestoreMap()
	if err != nil {
		t.Fatalf("FirestoreMap: %v", err)
	}

	for _, k := range []string{"overall", "band", "cap", "dimensions", "fixes", "problems", "rubricVersion", "judgeModel", "scoredAt"} {
		if _, ok := doc[k]; !ok {
			t.Errorf("missing %q — the panel reads this key", k)
		}
	}
	// A capitalised key means the struct leaked through rather than its JSON.
	for k := range doc {
		if k != "" && k[0] >= 'A' && k[0] <= 'Z' {
			t.Errorf("key %q is a Go field name, not the json tag", k)
		}
	}
	if doc["scoredAt"] == nil {
		t.Error("scoredAt must be present: an undefined timestamp crashes the panel")
	}

	// The severity has to survive as its name. Firestore tags could not do
	// this — MarshalJSON is the only thing that renders the enum.
	probs, ok := doc["problems"].([]interface{})
	if !ok || len(probs) != 1 {
		t.Fatalf("problems did not round-trip: %#v", doc["problems"])
	}
	p, _ := probs[0].(map[string]interface{})
	if p["severity"] != "cap" {
		t.Errorf("severity = %#v, want \"cap\"", p["severity"])
	}

	// Nested shapes matter just as much: the panel maps over dimensions and
	// reads each criterion.
	dims, _ := doc["dimensions"].([]interface{})
	if len(dims) != 1 {
		t.Fatalf("dimensions did not round-trip: %#v", doc["dimensions"])
	}
	d, _ := dims[0].(map[string]interface{})
	for _, k := range []string{"key", "label", "weight", "score", "criteria"} {
		if _, ok := d[k]; !ok {
			t.Errorf("dimension missing %q", k)
		}
	}
	crits, _ := d["criteria"].([]interface{})
	c0, _ := crits[1].(map[string]interface{})
	if c0["prefilled"] != true {
		t.Errorf("prefilled did not survive: %#v", c0)
	}
}
