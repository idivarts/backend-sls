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

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/studiojobs"
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
