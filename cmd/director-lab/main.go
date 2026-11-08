// Command director-lab runs the AI director locally and prints the storyboard
// it plans, so the planner can be judged before any of it is deployed.
//
// It deliberately touches no infrastructure: no Firestore, no SQS, no S3, no
// wallet. The manifest comes from director.FixtureManifest, so the only thing
// under test is the thing worth testing — whether, handed what this product can
// actually do, a model plans a video a person would approve.
//
// Nothing here generates media. A plan is text.
//
//	go run ./cmd/director-lab -brief "make it feel like a test, not an ad"
//	go run ./cmd/director-lab -format carousel -seconds 0
//	go run ./cmd/director-lab -seconds 24 -mode cinematic -cap 12
//	go run ./cmd/director-lab -gaps          # what the run would need that we have not built
//	go run ./cmd/director-lab -manifest      # exactly what the planner is handed
//	go run ./cmd/director-lab -plan p.json   # re-validate a stored plan, no model call
//	go run ./cmd/director-lab -title ... -hook ... -script ... -brief ...
//
// A plan call on the default reasoning model runs about $0.15. On a key with a
// small cap, -model google/gemini-3.5-flash is roughly a fiftieth of that and
// is enough to tell whether the prompt and the manifest are working, though it
// plans noticeably more literally.
//
// Provider, in order of preference:
//
//	OPENROUTER_API_KEY in the environment   — the path production uses
//	openrouter.apiKey  in key-secrets.json  — the same place pkg/gemini looks
//	OPENAI_API_KEY     in the environment
//	gemini.apiKey      in key-secrets.json
//
// key-secrets.json is matched by `key-secrets*.json` in .gitignore and is not
// tracked, which is why a key goes there and never into a source file: this
// package is meant to be committed, and a key pasted into it would be
// committed with it.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/trendlyapis/ai/director"
)

func main() {
	brief := flag.String("brief", "", "what the user asked for, in their words")
	format := flag.String("format", "reel", "reel | carousel | video | post")
	seconds := flag.Int("seconds", 30, "target length in seconds (0 = planner decides)")
	mode := flag.String("mode", "balanced", "economy | balanced | cinematic")
	cap_ := flag.Float64("cap", 8.0, "run cap in USD")
	model := flag.String("model", "", "model id; empty = discover a good one")
	showManifest := flag.Bool("manifest", false, "print the manifest the planner is handed and exit")
	showGaps := flag.Bool("gaps", false, "print the tools a run would need that are not built, and exit")
	showJSON := flag.Bool("json", false, "also print the raw plan JSON")
	repair := flag.Bool("repair", true, "on a fatal validation failure, send the problems back once")
	planFile := flag.String("plan", "", "validate and render a plan JSON from a file instead of calling a model")
	// The fixture content is one specific post. Overriding it is how a brief
	// about something else gets a manifest that agrees with it: the planner
	// reads the script and the hook as facts, so a brief that contradicts them
	// produces a plan hedging between the two.
	title := flag.String("title", "", "override the content's title")
	hook := flag.String("hook", "", "override the content's hook")
	script := flag.String("script", "", "override the content's script (the narration is cut from this)")
	pillar := flag.String("pillar", "", "override the content's strategy pillar")
	flag.Parse()

	m := director.FixtureManifest(*format, *seconds, *mode, *cap_)
	if *title != "" {
		m.Content.Title = *title
	}
	if *hook != "" {
		m.Content.Hook = *hook
	}
	if *script != "" {
		m.Content.Script = *script
		// A new script means the fixture's caption no longer describes the
		// post. Leaving it would hand the planner two different briefs.
		m.Content.Caption = ""
	}
	if *pillar != "" {
		m.Content.Pillar = *pillar
	}

	if *showGaps {
		fmt.Println("Tools a director run needs that are not built yet:")
		fmt.Print(director.ToolGaps(m.Tools))
		return
	}
	if *showManifest {
		b, _ := json.MarshalIndent(m, "", "  ")
		fmt.Println(string(b))
		return
	}

	// Replaying a stored plan: no model, no spending. This is how a stored
	// plan is re-examined later ("why did it choose that"), and how the
	// validator is tested against a plan built to break it.
	if *planFile != "" {
		raw, err := os.ReadFile(*planFile)
		if err != nil {
			die(err)
		}
		plan, err := decodePlan(string(raw))
		if err != nil {
			die(err)
		}
		fmt.Print(director.Storyboard(plan, m, plan.Validate(m)))
		return
	}

	user, err := director.BuildUserMessage(m, *brief)
	if err != nil {
		die(err)
	}

	p, err := newProvider(*model)
	if err != nil {
		die(err)
	}
	fmt.Fprintf(os.Stderr, "planner: %s · %s\n", p.name(), p.model())
	fmt.Fprintf(os.Stderr, "manifest: %d enabled models · %d library items · $%.2f cap · %s mode\n\n",
		len(m.Models), len(m.Library), m.Budget.RunCapUSD, m.Budget.Mode)

	start := time.Now()
	raw, err := p.complete(director.SystemPrompt, user)
	if err != nil {
		die(err)
	}

	plan, err := decodePlan(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "the planner did not return a usable plan:")
		fmt.Fprintln(os.Stderr, trim(raw, 2000))
		die(err)
	}

	problems := plan.Validate(m)
	if director.Fatal(problems) && *repair {
		// Name what was wrong before repairing it. A lab that silently fixes
		// the planner's mistakes hides the most useful thing it produces:
		// which rules the model keeps breaking.
		fmt.Fprintf(os.Stderr, "plan failed validation; asking for a repair (%d fatal)\n", countFatal(problems))
		for _, pr := range problems {
			if pr.Fatal {
				fmt.Fprintf(os.Stderr, "  %s\n", pr)
			}
		}
		fmt.Fprintln(os.Stderr)
		raw2, err := p.complete(director.SystemPrompt, user+"\n\nYOUR PREVIOUS PLAN:\n"+raw+"\n\n"+director.RepairMessage(problems))
		if err == nil {
			if fixed, err := decodePlan(raw2); err == nil {
				plan, raw = fixed, raw2
				problems = plan.Validate(m)
			}
		}
	}

	fmt.Print(director.Storyboard(plan, m, problems))
	fmt.Fprintf(os.Stderr, "\nplanned in %s\n", time.Since(start).Round(time.Millisecond))

	if *showJSON {
		b, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Println("\n" + string(b))
	}
}

// ── Providers ───────────────────────────────────────────────────────────────
//
// Two, because production goes through OpenRouter and this machine has a
// Gemini key. The prompt and the plan schema are identical either way, which
// is the only thing that has to be true for the lab to be worth running.

type provider interface {
	name() string
	model() string
	complete(system, user string) (string, error)
}

func newProvider(requested string) (provider, error) {
	orKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if orKey == "" {
		// Falls back to the untracked secrets file, so the lab needs no
		// exported variable and no key in source.
		orKey, _ = secretKey("openrouter")
	}
	if orKey != "" {
		mdl := requested
		if mdl == "" {
			// The reasoning-grade end of the chat registry — planning a video
			// is the same kind of work as planning a strategy, and this is the
			// model Resolve(TaskStrategy, ...) picks for a paid plan.
			mdl = "anthropic/claude-opus-4.8"
		}
		return openRouter{key: orKey, mdl: mdl}, nil
	}
	if k := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); k != "" {
		mdl := requested
		if mdl == "" {
			mdl = "gpt-5"
		}
		return openAI{key: k, mdl: mdl}, nil
	}
	key, err := secretKey("gemini")
	if err != nil {
		return nil, fmt.Errorf("no planner available: set OPENROUTER_API_KEY or OPENAI_API_KEY, or add openrouter.apiKey / gemini.apiKey to key-secrets.json (%w)", err)
	}
	g := gemini{key: key, mdl: requested}
	if g.mdl == "" {
		mdl, err := g.discover()
		if err != nil {
			return nil, err
		}
		g.mdl = mdl
	}
	return g, nil
}

type openRouter struct{ key, mdl string }

func (o openRouter) name() string  { return "openrouter" }
func (o openRouter) model() string { return o.mdl }

func (o openRouter) complete(system, user string) (string, error) {
	body := map[string]any{
		"model": o.mdl,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      32000,
	}
	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := postJSON("https://openrouter.ai/api/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + o.key}, body, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", fmt.Errorf("openrouter: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("openrouter returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}

// openAI is the stand-in while this machine has an OpenAI key and no
// OpenRouter one. Same prompt, same plan schema — only the transport differs,
// which is the point of keeping the director free of its provider.
type openAI struct{ key, mdl string }

func (o openAI) name() string  { return "openai (direct)" }
func (o openAI) model() string { return o.mdl }

func (o openAI) complete(system, user string) (string, error) {
	body := map[string]any{
		"model": o.mdl,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]string{"type": "json_object"},
		// The gpt-5 family takes max_completion_tokens and refuses a
		// temperature other than its default, so neither is set here.
		"max_completion_tokens": 32000,
	}
	var out struct {
		Choices []struct {
			Message      struct{ Content string } `json:"message"`
			FinishReason string                   `json:"finish_reason"`
		} `json:"choices"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := postJSON("https://api.openai.com/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + o.key}, body, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", fmt.Errorf("openai: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}
	if out.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("openai returned an empty plan (finish: %s)", out.Choices[0].FinishReason)
	}
	return out.Choices[0].Message.Content, nil
}

type gemini struct{ key, mdl string }

func (g gemini) name() string  { return "gemini (direct)" }
func (g gemini) model() string { return g.mdl }

// discover asks the API which models this key can use, rather than hardcoding
// an id that may have been retired. A wrong model id is a 404 at the worst
// possible moment; the list endpoint is free.
func (g gemini) discover() (string, error) {
	var out struct {
		Models []struct {
			Name                       string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := getJSON("https://generativelanguage.googleapis.com/v1beta/models?key="+g.key+"&pageSize=200", &out); err != nil {
		return "", err
	}
	var pro, flash string
	for _, m := range out.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if !hasString(m.SupportedGenerationMethods, "generateContent") {
			continue
		}
		low := strings.ToLower(id)
		// Exclude everything that is not a general text model: image, audio
		// and embedding models, and the research agents, which are a different
		// product behind the same endpoint and are billed like one.
		if containsAny(low, "image", "embedding", "tts", "vision", "live",
			"thinking-exp", "deep-research", "lyria", "nano-banana", "omni") {
			continue
		}
		// Prefer the newest pro, then the newest flash. Later entries in the
		// list are generally newer, so last match wins.
		if strings.Contains(low, "pro") {
			pro = id
		} else if strings.Contains(low, "flash") && !strings.Contains(low, "lite") {
			flash = id
		}
	}
	switch {
	case pro != "":
		return pro, nil
	case flash != "":
		return flash, nil
	}
	return "", fmt.Errorf("no gemini model on this key supports generateContent")
}

func (g gemini) complete(system, user string) (string, error) {
	body := map[string]any{
		"systemInstruction": map[string]any{"parts": []map[string]string{{"text": system}}},
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": user}}},
		},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"temperature":      0.6,
			"maxOutputTokens":  32768,
		},
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	url := "https://generativelanguage.googleapis.com/v1beta/models/" + g.mdl + ":generateContent?key=" + g.key
	if err := postJSON(url, nil, body, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", fmt.Errorf("gemini: %s", out.Error.Message)
	}
	if len(out.Candidates) == 0 {
		return "", fmt.Errorf("gemini returned no candidates")
	}
	// Concatenate every part: a thinking model can split its answer, and
	// taking parts[0] silently loses most of a long plan.
	var b strings.Builder
	for _, p := range out.Candidates[0].Content.Parts {
		b.WriteString(p.Text)
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("gemini returned an empty plan (finish: %s)", out.Candidates[0].FinishReason)
	}
	return b.String(), nil
}

// secretKey reads <section>.apiKey out of the key-secrets.json the repo
// already keeps for pkg/gemini.
//
// That file is gitignored and untracked, which is the whole reason the lab
// reads it: a key belongs somewhere git cannot pick it up, not in a source
// file that is going to be committed. Nothing here prints the value.
func secretKey(section string) (string, error) {
	for _, p := range []string{"key-secrets.json", filepath.Join("..", "key-secrets.json")} {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var doc map[string]struct {
			APIKey string `json:"apiKey"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return "", fmt.Errorf("%s: %w", p, err)
		}
		k := strings.TrimSpace(doc[section].APIKey)
		if k == "" {
			return "", fmt.Errorf("%s has no %s.apiKey", p, section)
		}
		return k, nil
	}
	return "", fmt.Errorf("key-secrets.json not found — run from the backend-sls directory")
}

// ── Plumbing ────────────────────────────────────────────────────────────────

func postJSON(url string, headers map[string]string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// A planning call on a reasoning model is slow. This is a lab, so it waits.
	cl := &http.Client{Timeout: 5 * time.Minute}
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("%s: %s", res.Status, trim(string(b), 600))
	}
	return json.Unmarshal(b, out)
}

func getJSON(url string, out any) error {
	cl := &http.Client{Timeout: 30 * time.Second}
	res, err := cl.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("%s: %s", res.Status, trim(string(b), 400))
	}
	return json.Unmarshal(b, out)
}

// decodePlan is forgiving about the wrapper and strict about the contents: a
// model that fences its JSON or prefixes a sentence has still done the work.
func decodePlan(raw string) (director.Plan, error) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimPrefix(s, "json")
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 && j < len(s)-1 {
		s = s[:j+1]
	}
	var p director.Plan
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &p); err != nil {
		return director.Plan{}, err
	}
	return p, nil
}

func countFatal(ps []director.Problem) int {
	n := 0
	for _, p := range ps {
		if p.Fatal {
			n++
		}
	}
	return n
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func hasString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "director-lab:", err)
	os.Exit(1)
}
