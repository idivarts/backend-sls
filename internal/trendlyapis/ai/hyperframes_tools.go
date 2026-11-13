package ai

// Authoring a Design Studio v3 project.
//
// v1 and v2 had the model write ONE self-contained HTML document with
// [data-slide] sections, against a contract we invented and maintained. v3
// has it write a Hyperframes project — several files, against a contract
// Hyperframes owns and enforces with its own `check`.
//
// ── Why the tool description is this long ──────────────────────────────────
// It is the contract. The model cannot be expected to satisfy rules it was
// never told, and every rule below was either taken verbatim from the
// Hyperframes skill shipped in the npm package, or learned by rendering a
// real reel in Phase 0 and reading what `check` complained about. The
// expensive ones to discover are marked.
//
// ── What is deliberately NOT here ──────────────────────────────────────────
// No GSAP <script> tag. Their own scaffold writes a jsdelivr CDN tag, which
// would make every render depend on that CDN being up; the worker vendors
// GSAP into the project and rewrites any CDN tag it finds. Telling the model
// to emit one would be telling it to write something we then undo.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

const toolWriteHyperframesProject = "write_hyperframes_project"

func hyperframesServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolWriteHyperframesProject,
			"Write a Design Studio v3 film as a Hyperframes project: several HTML files that "+
				"are rendered to MP4 by seeking a paused timeline frame by frame.\n\n"+

				"FILES. `index.html` is the root composition: it declares the film's variables and "+
				"mounts one host div per scene. Each scene is its own file under `compositions/`.\n\n"+

				"ROOT (index.html):\n"+
				"<html data-composition-variables='[{\"id\":\"headline\",\"type\":\"string\",\"label\":\"Headline\",\"default\":\"…\"}]'>\n"+
				"  <body style=\"margin:0\">\n"+
				"    <div id=\"root\" data-composition-id=\"main\" data-start=\"0\" data-duration=\"12\"\n"+
				"         data-width=\"1080\" data-height=\"1920\">\n"+
				"      <div id=\"hook\" data-composition-id=\"hook\" data-composition-src=\"compositions/hook.html\"\n"+
				"           data-start=\"0\" data-duration=\"4\" data-track-index=\"1\"\n"+
				"           data-track-kind=\"graphics\" data-width=\"1080\" data-height=\"1920\"></div>\n"+
				"    </div>\n"+
				"    <script>window.__timelines[\"main\"] = gsap.timeline({ paused: true });</script>\n"+
				"  </body>\n"+
				"</html>\n\n"+

				"SCENE (compositions/hook.html) — everything inside <template>, styles and scripts "+
				"outside it are DROPPED:\n"+
				"<template>\n"+
				"  <style>#hook { position:absolute; inset:0; background:#0E0B09; }</style>\n"+
				"  <div id=\"hook\" data-composition-id=\"hook\" data-width=\"1080\" data-height=\"1920\">\n"+
				"    <h1 id=\"hook-title\" class=\"clip\" data-start=\"0\" data-duration=\"4\" data-var-text=\"headline\">…</h1>\n"+
				"  </div>\n"+
				"  <script>\n"+
				"    const tl = gsap.timeline({ paused: true });\n"+
				"    tl.fromTo(\"#hook-title\", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.5, ease: \"power4.out\" }, 0);\n"+
				"    window.__timelines[\"hook\"] = tl;\n"+
				"  </script>\n"+
				"</template>\n\n"+

				"RULES — all of these are enforced, and breaking one fails the render:\n"+
				"1. The host id, the scene root id and the timeline key must be THE SAME STRING. "+
				"A mismatch makes every render wait 45s per scene and then show static frames.\n"+
				"2. Every timed element carries class=\"clip\", data-start and data-duration in "+
				"SECONDS, and is shown over [start, start+duration). A clip with data-start=\"0.15\" "+
				"leaves frame 0 empty — and frame 0 is the thumbnail on social players, so the "+
				"first scene's main element starts at 0.\n"+
				"3. The film's length is the ROOT's data-duration. Overrun is cut; an early end "+
				"holds the last frame.\n"+
				"4. Each composition builds ONE paused timeline and registers it AFTER its tweens: "+
				"window.__timelines[\"<data-composition-id>\"] = tl.\n"+
				"5. Use fromTo, never from: the host re-seeks a scene each time it shows, and `from` "+
				"records its start state once, so it drifts on seek-back.\n"+
				"6. Nothing may depend on real time: no Date.now(), no unseeded Math.random(), no "+
				"fetch, no timers, no repeat:-1. Everything visible is a function of timeline time.\n"+
				"7. Never tween display, visibility or autoAlpha on a .clip — animate a child. "+
				"Never put a CSS transform on an element GSAP transforms.\n"+
				"8. Size a scene root with `inset: 0`, never hard-coded pixels.\n"+
				"9. Ids must be unique across the whole assembled page — prefix them with the "+
				"scene (#hook-title, not #title). A scene's timeline cannot reach outside its file.\n"+
				"10. Never add a <script src> for GSAP. `gsap` is already there.\n"+
				"11. Fonts: only 'Noto Sans', 'Liberation Sans', 'DejaVu Sans' and the generic "+
				"families resolve. Any other family needs an @font-face to a file in assets/.\n"+
				"12. Something must keep moving: a stretch over 3s with nothing changing fails the "+
				"check. If a hold is intended, mark the element data-hold.\n"+
				"13. Keep content inside the middle 90% of the canvas, and text inside the middle "+
				"80%. Phones crop the edges.\n\n"+

				"MOTION. Give a scene one clear action rather than several small ones. A move "+
				"should land on the thing that carries the meaning, not on a container sliding in. "+
				"0.4-0.6s with ease \"power3.out\" or \"power4.out\" reads as deliberate; under 0.2s "+
				"reads as a glitch.\n\n"+

				"SIZING. Font sizes are in px against the canvas you were given, so they do NOT "+
				"scale if the canvas changes. For 1080x1920 a headline is roughly 100-140px and "+
				"body text 56-84px; for 1920x1080, 110-150px and 60-90px.\n\n"+

				"VARIABLES. Anything the user may want to change later — copy, a brand colour, a "+
				"swappable image — is declared in data-composition-variables and bound with "+
				"data-var-text=\"id\", data-var-src=\"id\" or var(--id) in CSS. This is the only part "+
				"of a film that can be edited on a phone, so put the copy there.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"designId": map[string]any{
						"type":        "string",
						"description": "The design to write this project into.",
					},
					"files": map[string]any{
						"type":        "array",
						"description": "Every file of the project. Must include index.html.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"path": map[string]any{
									"type":        "string",
									"description": "Relative path, e.g. index.html or compositions/hook.html.",
								},
								"content": map[string]any{"type": "string"},
							},
							"required": []string{"path", "content"},
						},
					},
				},
				"required": []string{"designId", "files"},
			},
		),
	}
}

type writeHyperframesProjectArgs struct {
	DesignID string `json:"designId"`
	Files    []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"files"`
}

// runWriteHyperframesProject validates the project, uploads it and points the
// design at it.
//
// Every failure returns ok:false with a sentence rather than an error, because
// the next turn is the model's attempt to fix it — an error would end the run
// where a message lets it try again. The one exception is a storage failure,
// which the model cannot do anything about.
func runWriteHyperframesProject(ctx context.Context, brandID, arguments string) (string, error) {
	if brandID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no brand in context"}), nil
	}
	var a writeHyperframesProjectArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil
	}
	if strings.TrimSpace(a.DesignID) == "" {
		return jsonResult(map[string]any{"ok": false, "error": "designId is required"}), nil
	}

	d, err := trendlymodels.GetBrandDesign(ctx, brandID, a.DesignID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not read the design"}), nil
	}
	if d == nil {
		return jsonResult(map[string]any{"ok": false, "error": "no such design"}), nil
	}

	files := make([]hfproject.File, 0, len(a.Files))
	for _, f := range a.Files {
		files = append(files, hfproject.File{Path: f.Path, Content: f.Content})
	}

	project, err := hfproject.Build(files)
	if err != nil {
		// The validation messages are written for the model. Handing the
		// error straight back is what lets it correct the project on the
		// next turn instead of guessing.
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), nil
	}

	prefix, err := hfproject.Upload(ctx, brandID, a.DesignID, project)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not store the project"}), err
	}

	// Both fields in one write. projectPrefix without durationMs would be a
	// renderable design the wallet cannot price, and durationMs without the
	// prefix would price a design with nothing to render.
	if err := trendlymodels.UpdateBrandDesign(ctx, brandID, a.DesignID, []firestore.Update{
		{Path: "projectPrefix", Value: prefix},
		{Path: "durationMs", Value: project.DurationMs},
	}); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not save the design"}), err
	}

	return jsonResult(map[string]any{
		"ok":         true,
		"designId":   a.DesignID,
		"files":      len(project.Files),
		"durationMs": project.DurationMs,
		"note": fmt.Sprintf(
			"Saved %d files. The film is %.1fs. It is checked by Hyperframes when it renders; "+
				"a structural mistake fails there with a specific fix.",
			len(project.Files), float64(project.DurationMs)/1000),
	}), nil
}
