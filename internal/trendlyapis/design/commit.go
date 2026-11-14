package design

// Turning an edited document into the design's next revision.
//
// Three endpoints produce one — the inspector, a scoped AI edit and a
// structural change — and they do the same six things afterwards:
// validate, allocate, copy the project, write the document, write the
// player, record the revision. Three copies of that would drift, and the
// one that drifted would be the one that left a revision pointing at an
// incomplete project.

import (
	"context"
	"fmt"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/mediastore"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// editOutcome is what the caller answers with.
type editOutcome struct {
	Revision   int
	PreviewURL string
	DurationMs int
}

// commitEditedDocument validates an edited index.html and makes it the
// design's current revision.
//
// Returns a message fit to show the user, and a flag saying whether the
// problem is theirs (a document that will not validate) or ours (a bucket
// that would not answer).
func commitEditedDocument(
	ctx context.Context,
	brandID, designID string,
	from *trendlymodels.Design,
	edited string,
	by, note string,
) (*editOutcome, string, bool, error) {
	// ── The gate, before anything is written ─────────────────────────────
	// An edit that breaks the project is refused with the previous
	// revision untouched, rather than discovered by the next render.
	project, berr := hfproject.Build([]hfproject.File{{Path: "index.html", Content: edited}})
	if berr != nil {
		return nil, berr.Error(), true, nil
	}

	rev, rerr := trendlymodels.AllocateDesignRevision(ctx, brandID, designID)
	if rerr != nil {
		return nil, "could not save the change", false, rerr
	}
	prefix := hfproject.RevisionPrefix(brandID, designID, rev)

	// Everything but the document, copied inside the bucket — a revision
	// has to be complete or restoring it would play a film with no fonts.
	// The player pages are left out because they are rewritten below.
	if _, cerr := mediastore.CopyPrefix(ctx, mediastore.TargetAttachment, from.ProjectPrefix, prefix,
		"index.html", "player.html", "preview.html"); cerr != nil {
		mysentry.Capture(cerr, map[string]string{"op": "design.edit.copy", "designId": designID})
		return nil, "could not save the change", false, cerr
	}
	if _, uerr := hfproject.Upload(ctx, brandID, designID, rev, project); uerr != nil {
		mysentry.Capture(uerr, map[string]string{"op": "design.edit.upload", "designId": designID})
		return nil, "could not save the change", false, uerr
	}
	previewURL, perr := hfproject.UploadPlayer(ctx, brandID, designID, rev, project)
	if perr != nil {
		// Best-effort, as everywhere: a film with no player still renders.
		mysentry.Capture(perr, map[string]string{"op": "design.edit.player", "designId": designID})
	}

	scenes := make([]trendlymodels.DesignScene, 0, len(project.Scenes))
	for _, s := range project.Scenes {
		scenes = append(scenes, trendlymodels.DesignScene{
			ID: s.ID, Src: s.Src, StartMs: s.StartMs,
			DurationMs: s.DurationMs, TrackIndex: s.TrackIndex, TrackKind: s.TrackKind,
		})
	}
	pruned, comerr := trendlymodels.CommitDesignRevision(ctx, brandID, designID, trendlymodels.DesignRevision{
		N:          rev,
		Prefix:     prefix,
		PreviewURL: previewURL,
		By:         by,
		Note:       note,
		DurationMs: project.DurationMs,
		Scenes:     scenes,
	})
	if comerr != nil {
		mysentry.Capture(comerr, map[string]string{"op": "design.edit.commit", "designId": designID})
		return nil, "could not save the change", false, comerr
	}
	dropRevisionFiles(ctx, pruned)

	return &editOutcome{Revision: rev, PreviewURL: previewURL, DurationMs: project.DurationMs}, "", false, nil
}

// readCurrentDocument reads the design's current index.html.
func readCurrentDocument(ctx context.Context, d *trendlymodels.Design) (string, error) {
	if d.ProjectPrefix == "" {
		return "", fmt.Errorf("This design has nothing in it yet.")
	}
	raw, err := mediastore.Get(ctx, mediastore.TargetAttachment, d.ProjectPrefix+"/index.html")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
