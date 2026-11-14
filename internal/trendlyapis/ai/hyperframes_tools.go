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
	"strings"

	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/mediastore"
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

	pruned, err := trendlymodels.CommitDesignRevision(ctx, brandID, a.DesignID, trendlymodels.DesignRevision{
		N:          rev,
		Prefix:     prefix,
		PreviewURL: previewURL,
		By:         trendlymodels.DesignRevisionAIEdit,
		Note:       "Edited by the AI panel",
		DurationMs: project.DurationMs,
		Scenes:     scenes,
	})
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not save the design"}), err
	}
	for _, p := range pruned {
		if _, derr := mediastore.DeletePrefix(ctx, mediastore.TargetAttachment, p); derr != nil {
			log.Printf("design revision cleanup failed for %s: %v", p, derr)
		}
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
