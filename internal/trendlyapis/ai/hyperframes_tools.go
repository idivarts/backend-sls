package ai

// Authoring a Design Studio v3 project.
//
// v1 and v2 had the model write ONE self-contained HTML document with
// [data-slide] sections, against a contract we invented and maintained. v3
// has it write a Hyperframes project — several files, against a contract
// Hyperframes owns and enforces with its own `check`.
//
// ── Where the contract lives ───────────────────────────────────────────────
// pkg/hfproject.AuthoringContract, not here. Two things author projects —
// this tool, and the async design_author job — and a contract written twice
// is a contract that drifts: a design written one way would render and the
// same design written the other way would not, with no way to tell from the
// result which path produced the broken one.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/studiojobs"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/mediastore"
	"github.com/idivarts/backend-sls/pkg/mysentry"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

const toolWriteHyperframesProject = "write_hyperframes_project"

func hyperframesServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolWriteHyperframesProject,
			"Write a Design Studio v3 film as a Hyperframes project: several HTML files "+
				"rendered to MP4 by seeking a paused timeline frame by frame.\n\n"+
				hfproject.AuthoringContract,
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

	// A revision, not an overwrite. An AI edit is exactly the writer this
	// mechanism exists for: it rewrites a film the user may have liked, and
	// the way back has to be written at the same moment the change is.
	rev, err := trendlymodels.AllocateDesignRevision(ctx, brandID, a.DesignID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not save the design"}), err
	}

	prefix, err := hfproject.Upload(ctx, brandID, a.DesignID, rev, project)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not store the project"}), err
	}

	// ── Everything the model did not write comes with it ────────────────
	// A write replaces the PROJECT, not the revision: the brand's fonts,
	// its logo and every resolved media file live in the same prefix and
	// the model never sees their bytes — read_hyperframes_project reports
	// them as `asset: true` precisely so a 24KB woff2 does not go through
	// a frontier model.
	//
	// Uploading only the model's files therefore left the new revision
	// with index.html and nothing else, while that index.html still asked
	// for assets/brand-logo.png and assets/fonts/inter-400.woff2. Seen for
	// real: from the first chat edit onwards a film lost its brand font
	// and logo to 404s and kept rendering, in fallback type, with nobody
	// told. element.go has always done this copy; this is the same one.
	kept := carryForward(ctx, d.ProjectPrefix, prefix, project)

	// The player pages go beside the project, or the app plays the previous
	// revision while the renderer renders this one. Best-effort, as
	// everywhere else: a film with no player still renders.
	previewURL, perr := hfproject.UploadPlayer(ctx, brandID, a.DesignID, rev, project)
	if perr != nil {
		log.Printf("ai design player upload failed for %s/%s: %v", brandID, a.DesignID, perr)
	}

	// Pointer, duration and the revision entry in one transaction.
	// projectPrefix without durationMs would be a renderable design the
	// wallet cannot price, and durationMs without the prefix would price a
	// design with nothing to render.
	// The scenes travel with the revision. Without them a restore puts the
	// pointer back and leaves the timeline describing a different film —
	// three clips drawn over a two-scene project, and a scene panel listing
	// something that is not there.
	scenes := make([]trendlymodels.DesignScene, 0, len(project.Scenes))
	for _, sc := range project.Scenes {
		scenes = append(scenes, trendlymodels.DesignScene{
			ID:         sc.ID,
			Src:        sc.Src,
			StartMs:    sc.StartMs,
			DurationMs: sc.DurationMs,
			TrackIndex: sc.TrackIndex,
			TrackKind:  sc.TrackKind,
		})
	}
	// And the timeline, for the same reason: a restore that put the scenes
	// back and left the clips describing the previous film would draw rows
	// for media that is no longer in it.
	clips := make([]trendlymodels.DesignClip, 0, len(project.Clips))
	for _, c := range project.Clips {
		clips = append(clips, trendlymodels.DesignClip{
			ElementID: c.ElementID, Kind: c.Kind, TrackIndex: c.TrackIndex,
			Authored: c.Authored, StartMs: c.StartMs, DurationMs: c.DurationMs,
			Label: c.Label, Src: c.Src,
		})
	}

	pruned, err := trendlymodels.CommitDesignRevision(ctx, brandID, a.DesignID, trendlymodels.DesignRevision{
		N:          rev,
		Prefix:     prefix,
		PreviewURL: previewURL,
		By:         trendlymodels.DesignRevisionAIEdit,
		Note:       "Edited by the AI panel",
		DurationMs: project.DurationMs,
		Scenes:     scenes,
		Clips:      clips,
	})
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not save the design"}), err
	}
	for _, p := range pruned {
		if _, derr := mediastore.DeletePrefix(ctx, mediastore.TargetAttachment, p); derr != nil {
			log.Printf("design revision cleanup failed for %s: %v", p, derr)
		}
	}

	// ── Media the project ASKS for is not media it HAS ──────────────────
	// A declaration is an <audio>/<img> with data-media-intent and no src,
	// and something has to fetch the real file and fill the src in.
	// Authoring does that at the end of its job; an edit did not. So "add
	// music" produced a correctly declared soundtrack, no file, and a film
	// that played in silence — with the model reporting, truthfully as far
	// as it could tell, that it had added the track.
	declared := unresolvedMedia(project)

	// ── A film that was failing has to be told it is not any more ───────
	// The gate's verdict lives on the document: previewStatus "failed" and
	// the sentence the user reads. Nothing clears it but another run. So a
	// write that fixes the fault left the banner saying "Something went
	// wrong — 404 loading assets/fonts/inter-400.woff2" over a film with
	// no such reference in it, and the user had no way to tell a repair
	// from a no-op. Seen exactly that way on dev: the model found both
	// dangling fonts, removed them, said "the film should render now",
	// and the error stayed on screen.
	wasFailing := d.PreviewStatus == trendlymodels.DesignPreviewFailed
	queued := false
	if (declared > 0 || wasFailing) && studiojobs.Configured() {
		if eerr := studiojobs.Enqueue(studiojobs.Message{
			Type:     studiojobs.JobDesignPrepare,
			BrandID:  brandID,
			DesignID: a.DesignID,
		}); eerr != nil {
			mysentry.Capture(eerr, map[string]string{
				"op": "design.chat.write.prepare", "designId": a.DesignID,
			})
		} else {
			queued = true
		}
	}

	note := fmt.Sprintf(
		"Saved %d files. The film is %.1fs. It is checked by Hyperframes when it renders; "+
			"a structural mistake fails there with a specific fix.",
		len(project.Files), float64(project.DurationMs)/1000)
	if fonts := undeclaredFonts(project); len(fonts) > 0 {
		note += " ⚠️ This project names font families the renderer cannot supply: " +
			strings.Join(fonts, ", ") +
			". The gate FAILS on that. Either add an @font-face pointing at a font" +
			" file the project actually holds, or switch to one it resolves by" +
			" itself: " + RendererFontList + "." +
			" Deleting an @font-face is NOT the fix — it leaves the family named" +
			" and undeclared, which is the same error one step later."
	}

	if missing := danglingAssets(project, kept); len(missing) > 0 {
		note += " ⚠️ This project references files this film does NOT have: " +
			strings.Join(missing, ", ") +
			". They will 404 and finishing the film will fail. You cannot upload" +
			" binary files — rewrite those references away (a system font, no logo)" +
			" and write the project again, then tell the user what you changed."
	}

	switch {
	case declared > 0 && queued:
		note += fmt.Sprintf(" %d piece(s) of media are DECLARED but not fetched yet;"+
			" that is running now. Say the film is being finished and that the"+
			" media will arrive shortly — do NOT say it is already playing.", declared)
	case wasFailing && queued:
		note += " This film was in a failed state; it is being finished again now," +
			" which is what clears the error the user can see. Say it is being" +
			" re-checked rather than that it is fixed."
	case declared > 0:
		note += fmt.Sprintf(" ⚠️ %d piece(s) of media are declared with no file and"+
			" nothing is fetching them. Tell the user the media could not be"+
			" added, rather than that it was.", declared)
	}

	return jsonResult(map[string]any{
		"ok":           true,
		"designId":     a.DesignID,
		"files":        len(project.Files),
		"durationMs":   project.DurationMs,
		"mediaPending": declared,
		"note":         note,
	}), nil
}

// carryForward copies the previous revision's non-source files into the new
// one: the brand's fonts and logo, and every media file already resolved.
//
// Best effort, like every other copy in this flow — a film that loses its
// logo is worse off than one that does not, but it is still a film, and
// failing the write would throw away an edit the user has paid for.
func carryForward(ctx context.Context, from, to string, project *hfproject.Project) map[string]bool {
	kept := map[string]bool{}
	if from == "" || from == to {
		return kept
	}
	// Skip anything the new revision owns. The player pages are written
	// separately just below, so they must not be overwritten by the old
	// ones; the project's own files are already there.
	skip := []string{"player.html", "preview.html"}
	for _, f := range project.Files {
		skip = append(skip, f.Path)
	}
	// And the OLD project's source, which the new project replaces. Without
	// this a scene file the model deleted would be copied back in.
	if olds, lerr := mediastore.ListPrefix(ctx, mediastore.TargetAttachment, from); lerr == nil {
		for _, rel := range olds {
			if isProjectSource(rel) {
				skip = append(skip, rel)
				continue
			}
			kept[rel] = true
		}
	}
	if _, cerr := mediastore.CopyPrefix(ctx, mediastore.TargetAttachment, from, to, skip...); cerr != nil {
		log.Printf("design chat write: carrying files forward failed %s → %s: %v", from, to, cerr)
	}
	return kept
}

// assetRef finds every assets/… path a project's source mentions.
var assetRef = regexp.MustCompile(`assets/[A-Za-z0-9._/-]+`)

// danglingAssets are the files the project asks for and the film does not
// have.
//
// ── Why this is checked at WRITE time ───────────────────────────────────
// Because the alternative is finding out from the gate, minutes later, as
// "404 loading assets/fonts/space-grotesk-700.woff2" in a red banner over
// a film that will not finish. The model cannot upload a woff2, so a
// reference it cannot satisfy is a reference it must not write — and the
// one moment it can still choose otherwise is while it is being told what
// it just saved.
func danglingAssets(project *hfproject.Project, kept map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range project.Files {
		for _, ref := range assetRef.FindAllString(f.Content, -1) {
			ref = strings.TrimRight(ref, ".,;:)'\"")
			if seen[ref] || kept[ref] {
				continue
			}
			seen[ref] = true
			out = append(out, ref)
			if len(out) >= 10 {
				return out
			}
		}
	}
	return out
}

// ── Fonts the renderer can supply on its own ────────────────────────────
// Hyperframes' check refuses a family it cannot resolve, as a hard error:
// "used without @font-face declaration … not in the auto-resolved font
// list". What it accepts is a family DECLARED by an @font-face in the
// document, a family the document links from Google Fonts, or one of
// these — the canonical faces bundled with the renderer, read out of its
// own FONT_ALIAS_MAP (hyperframes/dist, checked 2026-11-14).
//
// The list matters because the obvious repair for a missing font file is
// to delete the @font-face, and that leaves the family NAMED and
// undeclared — which is the same error one step later. Seen exactly that
// way on dev: the model removed two dangling woff2 references, said the
// film should render, and the gate came back with "space grotesk".
var rendererFonts = map[string]bool{
	"inter": true, "montserrat": true, "outfit": true, "nunito": true,
	"oswald": true, "league gothic": true, "archivo black": true,
	"space mono": true, "ibm plex mono": true, "jetbrains mono": true,
	"eb garamond": true, "playfair display": true, "source code pro": true,
	"noto sans jp": true, "roboto": true, "open sans": true, "lato": true,
	"poppins": true,
}

// RendererFontList is those names, for a prompt to read.
var RendererFontList = "Inter, Montserrat, Outfit, Nunito, Oswald, League Gothic, " +
	"Archivo Black, Space Mono, IBM Plex Mono, JetBrains Mono, EB Garamond, " +
	"Playfair Display, Source Code Pro, Noto Sans JP, Roboto, Open Sans, Lato, Poppins"

// genericFamilies are CSS keywords, not faces.
var genericFamilies = map[string]bool{
	"serif": true, "sans-serif": true, "monospace": true, "cursive": true,
	"fantasy": true, "system-ui": true, "ui-sans-serif": true,
	"ui-serif": true, "ui-monospace": true, "ui-rounded": true,
	"inherit": true, "initial": true, "unset": true, "revert": true,
	"emoji": true, "math": true, "fangsong": true,
}

var (
	fontFaceBlock = regexp.MustCompile(`(?is)@font-face\s*{[^}]*}`)
	fontFamilyUse = regexp.MustCompile(`(?i)font-family\s*:\s*([^;}]+)`)
	googleFamily  = regexp.MustCompile(`(?i)family=([A-Za-z0-9+%20_-]+)`)
)

// undeclaredFonts are the families the project names and the renderer
// cannot supply: not declared by an @font-face here, not linked from
// Google Fonts, and not one of its own.
//
// The same shape as danglingAssets, and for the same reason: the model
// cannot upload a font file either, so the only moment this is cheap to
// fix is while it is being told what it just wrote.
func undeclaredFonts(project *hfproject.Project) []string {
	declared := map[string]bool{}
	seen := map[string]bool{}
	var out []string

	// Pass one: what the project declares, and what it links.
	for _, f := range project.Files {
		for _, block := range fontFaceBlock.FindAllString(f.Content, -1) {
			for _, m := range fontFamilyUse.FindAllStringSubmatch(block, -1) {
				for _, name := range splitFamilies(m[1]) {
					declared[name] = true
				}
			}
		}
		for _, m := range googleFamily.FindAllStringSubmatch(f.Content, -1) {
			fam := strings.ToLower(strings.NewReplacer("+", " ", "%20", " ").Replace(m[1]))
			declared[strings.TrimSpace(fam)] = true
		}
	}

	// Pass two: what it uses, with the @font-face blocks taken out so a
	// declaration does not count as a use of itself.
	for _, f := range project.Files {
		body := fontFaceBlock.ReplaceAllString(f.Content, "")
		for _, m := range fontFamilyUse.FindAllStringSubmatch(body, -1) {
			for _, name := range splitFamilies(m[1]) {
				if declared[name] || rendererFonts[name] || seen[name] {
					continue
				}
				seen[name] = true
				out = append(out, name)
				if len(out) >= 10 {
					return out
				}
			}
		}
	}
	return out
}

// splitFamilies reads a font-family value into comparable names: unquoted,
// lowercased, with the generics and var() references dropped.
func splitFamilies(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		name = strings.Trim(name, "'\"")
		name = strings.TrimSpace(name)
		// A var() is a reference to a token defined elsewhere in the same
		// stylesheet; the token's own value is scanned on its own line.
		if name == "" || genericFamilies[name] || strings.HasPrefix(name, "var(") {
			continue
		}
		out = append(out, name)
	}
	return out
}

// unresolvedMedia counts the elements that ask for media and have none.
//
// A clip's Src is the file behind it. Empty, on a kind that is media,
// means the project declared it and nothing has fetched it yet.
func unresolvedMedia(project *hfproject.Project) int {
	n := 0
	for _, c := range project.Clips {
		switch c.Kind {
		case hfproject.ClipMusic, hfproject.ClipVoice, hfproject.ClipSound,
			hfproject.ClipImage, hfproject.ClipVideo:
			if strings.TrimSpace(c.Src) == "" {
				n++
			}
		}
	}
	return n
}
