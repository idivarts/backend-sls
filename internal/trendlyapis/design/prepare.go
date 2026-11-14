package design

// Finishing a design that was EDITED rather than authored.
//
// ── The gap this closes ─────────────────────────────────────────────────
// A Hyperframes project asks for media by declaring it. An element carries
// data-media and data-media-intent and no src:
//
//	<audio id="score" data-media="bgm"
//	       data-media-intent="warm, minimal, no vocals"></audio>
//
// and a later pass fetches the real thing and writes the src in. The
// authoring job runs that pass (markAndPrepare, at the end of
// ExecuteDesignAuthorJob). Nothing ran it after an AI chat edit.
//
// So a user who asked the chat for music got exactly what the model said
// it had done — a correctly declared soundtrack — and no sound. The model
// was not lying and the project was not wrong; the half of the pipeline
// that turns a declaration into a file simply never ran.

import (
	"context"
	"log"
	"path"
	"regexp"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/studiojobs"
	"github.com/idivarts/backend-sls/pkg/mediastore"
)

// ExecuteDesignPrepareJob resolves a design's declared media and recaptures
// its stills.
//
// It takes the WHOLE media ceiling rather than a share of it: unlike
// authoring, which may write a film twice and has to divide the cap between
// its attempts, this runs once per edit.
func ExecuteDesignPrepareJob(ctx context.Context, msg studiojobs.Message) error {
	brandID, designID := msg.BrandID, msg.DesignID

	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		return err
	}
	if d == nil {
		// Deleted between the enqueue and the run. An ordinary outcome.
		log.Printf("studiojobs: design_prepare for a missing design %s/%s", brandID, designID)
		return nil
	}
	if d.ProjectPrefix == "" {
		// Nothing written to prepare. Also ordinary: an authoring job that
		// failed leaves the design empty, and its own error is already on
		// the document.
		return nil
	}

	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	return markAndPrepare(ctx, brandID, designID, MediaCreditCeilingPerAttempt(orgID, 1))
}

// ⚠️ NOT called from PreviewDesign, the HTTP route behind "Try again".
// Each asset gets an 8-second fetch timeout and a brand can have three of
// them, which is 24 seconds against API Gateway's hard 29. That route
// queues this job instead; the worker has 900 seconds.

// assetRef finds every assets/… path a project's source mentions.
var assetRef = regexp.MustCompile(`assets/[A-Za-z0-9._/-]+`)

// restoreBrandAssets puts back the brand files a film references and no
// longer holds, and returns how many it replaced.
//
// ── Why a film loses them in the first place ────────────────────────────
// The brand kit fetches the font files and the logo at AUTHORING time and
// writes them into the project beside the CSS that names them. Every AI
// chat write then replaced the project with the model's files alone, and
// the kit's assets were not among them — the model never sees their bytes.
// backend-sls#584 stopped that, but only for writes made after it: a film
// edited before has a revision that still asks for
// assets/fonts/space-grotesk-700.woff2 and does not contain it.
//
// ── Why the model cannot fix it ─────────────────────────────────────────
// Told the file is missing, its options are to drop the @font-face (which
// leaves the family named and undeclared — the same gate failure one step
// later) or to change the family. Neither works here, because the family
// is named by the kit's own :root block with !important, which the model
// preserves verbatim and must: it is the brand's type, not a mistake. On
// dev it announced a swap to Outfit and wrote a revision with no Outfit
// in it, because there was nothing it could usefully change.
//
// So the repair belongs where the bytes come from. buildBrandKit already
// knows how to fetch them and names them deterministically
// (assets/fonts/<family>-<weight>.<ext>, assets/brand-logo.<ext>), which
// is the same name the stale CSS is asking for.
//
// Best effort throughout: a film that cannot get its logo back is still a
// film, and failing the preparation over it would replace a typography
// warning with a hard stop.
func restoreBrandAssets(ctx context.Context, brandID, designID string) int {
	d, derr := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if derr != nil || d == nil || d.ProjectPrefix == "" {
		return 0
	}
	rels, err := mediastore.ListPrefix(ctx, mediastore.TargetAttachment, d.ProjectPrefix)
	if err != nil {
		log.Printf("design prepare: could not list %s: %v", d.ProjectPrefix, err)
		return 0
	}
	have := map[string]bool{}
	for _, rel := range rels {
		have[rel] = true
	}

	// What the film asks for, across every source file rather than
	// index.html alone — a scene file can carry its own stylesheet.
	missing := map[string]bool{}
	for _, rel := range rels {
		if !isSourceFile(rel) {
			continue
		}
		raw, gerr := mediastore.Get(ctx, mediastore.TargetAttachment, d.ProjectPrefix+"/"+rel)
		if gerr != nil {
			continue
		}
		for _, ref := range assetRefsIn(string(raw)) {
			if !have[ref] {
				missing[ref] = true
			}
		}
	}
	if len(missing) == 0 {
		return 0
	}

	ds, dserr := trendlymodels.GetDesignSystem(ctx, brandID)
	if dserr != nil || ds == nil {
		// Nothing to restore from. The film keeps its warning, which is
		// the honest outcome for a brand with no design system.
		return 0
	}
	kit := buildBrandKit(ctx, ds)
	if kit == nil {
		return 0
	}

	restored := 0
	for _, f := range kit.Files {
		if !missing[f.Path] {
			continue
		}
		if _, perr := mediastore.Put(ctx, mediastore.Spec{
			BrandID:   brandID,
			Target:    mediastore.TargetAttachment,
			ExactKey:  d.ProjectPrefix + "/" + f.Path,
			Filename:  path.Base(f.Path),
			NoLibrary: true,
		}, []byte(f.Content)); perr != nil {
			log.Printf("design prepare: could not restore %s for %s: %v", f.Path, designID, perr)
			continue
		}
		delete(missing, f.Path)
		restored++
	}
	for ref := range missing {
		// Named, so the next person reading the logs knows the gate is
		// about to complain about something the kit could not supply.
		log.Printf("design prepare: %s/%s references %s and the brand kit has no such file",
			brandID, designID, ref)
	}
	return restored
}

// assetRefsIn reads the assets/… paths a source file mentions, with the
// punctuation that follows a url('…') or a src="…" trimmed off.
func assetRefsIn(source string) []string {
	seen := map[string]bool{}
	var out []string
	for _, ref := range assetRef.FindAllString(source, -1) {
		ref = strings.TrimRight(ref, ".,;:)'\"")
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}

// isSourceFile reports whether a project file is worth scanning for asset
// references: the documents and stylesheets, not the media beside them.
func isSourceFile(rel string) bool {
	switch strings.ToLower(path.Ext(rel)) {
	case ".html", ".htm", ".css", ".js", ".json":
		return true
	}
	return false
}
