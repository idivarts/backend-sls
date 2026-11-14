// Package design is the Design Studio v3 API.
//
// ⚠️ A DESIGN IS NOT SCOPED TO A CONTENT. Every route here is
// /brands/:brandId/designs/… with no :contentId, which is the whole point of
// v3: a design lives at brands/{brandId}/designs/{designId} and carries the
// link to a content as an optional field. That is what lets Design Studio sit
// in the Playground section of the drawer as something you open, rather than
// a mode you can only enter from a post.
//
// Compare internal/trendlyapis/studio, whose every route is
// /brands/:brandId/contents/:contentId/… — that package is v1/v2 and stays
// alive until Phase 6.
//
// Designs are BACKEND-WRITTEN AND CLIENT-READ. The document carries the render
// lifecycle and the pointer to the project in S3; a client able to write
// renderStatus could mark an unrendered design done, and one able to write
// projectPrefix could point it at another brand's files. firestore.rules says
// `allow write: if false` for exactly that reason, so every mutation comes
// through here.
package design

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
)

// RegisterRoutes wires /api/design onto the engine. Manager session
// throughout: a design belongs to a brand, and only a brand's team authors in
// one.
func RegisterRoutes(engine *gin.Engine) {
	g := engine.Group("/api/design",
		middlewares.ValidateSessionMiddleware(),
		middlewares.TrendlyMiddleware("managers"),
	)

	base := "/brands/:brandId/designs"

	g.POST(base, CreateDesign)
	g.GET(base, ListDesigns)
	g.GET(base+"/:designId", GetDesign)

	// Preview comes BEFORE render, and charges nothing. It captures stills
	// across the film so the user can approve what they are about to pay for
	// — Hyperframes' own loop snapshots at step 3 and renders at step 5
	// because "passing checks alone is not a reason to render".
	g.POST(base+"/:designId/preview", PreviewDesign)

	// Scoring is a frontier vision call over the preview's frames, so it is
	// its own POST rather than something previewing does for free.
	g.POST(base+"/:designId/score", ScoreDesign)

	// Rendering is a POST because it spends money: it charges the wallet and
	// queues real compute. The response says what state the design is in, not
	// what the render produced — the result arrives on the document, which is
	// what lets a render survive a page reload.
	g.POST(base+"/:designId/render", RenderDesign)

	// Authoring is a 202, not a 200 with the result. Writing a project is a
	// frontier model emitting several HTML documents and does not fit in API
	// Gateway's 29-second ceiling — the first attempt at doing it inline
	// returned 504 with nothing stored. The job runs on the studio queue and
	// reports on the design document, which the app already watches.
	g.POST(base+"/:designId/author", AuthorDesign)

	// Going back to a revision. A POST because it changes which bytes the
	// renderer, the preview and the player all read — and the list it acts
	// on needs no GET of its own, because revisions ride on the design
	// document the app already watches.
	g.POST(base+"/:designId/revisions/:n/restore", RestoreDesignRevision)
}
