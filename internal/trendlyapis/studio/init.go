// Package studio is the Design Studio's API.
//
// ⚠️ SCENES ARE BACKEND-WRITTEN AND CLIENT-READ, which is a deliberate reversal
// of how designs work (that subcollection is client-writable and the app writes
// revisions straight to it).
//
// The reason is reference counts. Every scene write moves them: placing media
// increments, clearing decrements, swapping moves only the difference. A client
// that wrote scenes directly would have to maintain those counts itself, on
// every one of a dozen edit paths, and the first one that forgot would either
// strand media as permanently un-archivable or let a sweep delete something
// still on screen. Neither is recoverable by hand.
//
// The UI does not pay for this. It subscribes to the scenes in Firestore and
// applies its own edit optimistically, so a drag still lands instantly — the
// write goes out in the background and the subscription confirms it. What the
// user loses is nothing; what we gain is a library whose counts are true.
package studio

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
)

// RegisterRoutes wires /api/studio onto the engine. Manager session throughout:
// a studio belongs to a brand, and only a brand's team authors in it.
func RegisterRoutes(engine *gin.Engine) {
	g := engine.Group("/api/studio",
		middlewares.ValidateSessionMiddleware(),
		middlewares.TrendlyMiddleware("managers"),
	)

	base := "/brands/:brandId/contents/:contentId"

	// Open the studio for a content, making one if it has none. The blank
	// canvas is the product decision: a studio that opens with a scene in it
	// has guessed what the user is making, and the first thing they do is
	// delete it.
	g.POST(base+"/open", OpenStudio)
	g.GET(base+"/studios/:studioId", GetStudio)
	g.PATCH(base+"/studios/:studioId", PatchStudio)
	g.DELETE(base+"/studios/:studioId", DeleteStudio)

	scenes := base + "/studios/:studioId/scenes"
	g.POST(scenes, AddScene)
	// Order is replaced wholesale, never spliced: it is ONE fact, and a partial
	// update to it is how two clients end up believing different sequences.
	g.POST(scenes+"/order", ReorderScenes)
	g.PATCH(scenes+"/:sceneId", PatchScene)
	g.PUT(scenes+"/:sceneId/layers", SetSceneLayers)
	g.DELETE(scenes+"/:sceneId", DeleteScene)
	g.POST(scenes+"/:sceneId/duplicate", DuplicateScene)
	g.POST(scenes+"/:sceneId/split", SplitScene)

	// Motion graphics. Two calls rather than one: plan-scenes writes every
	// scene empty in about a second so the shape is on the rail immediately,
	// then the client fills them one at a time — which is what makes the rail
	// fill visibly and a failure cost one scene instead of the run.
	g.POST(base+"/studios/:studioId/plan-scenes", PlanScenes)
	g.POST(scenes+"/:sceneId/generate", GenerateScene)

	// AI video: one beat per request, because every model this product can
	// reach tops out at 8 seconds. A longer sequence is several clips on the
	// timeline, not a longer generation.
	g.GET(base+"/studios/:studioId/video-quote", VideoQuote)
	g.POST(scenes+"/:sceneId/generate-video", GenerateSceneVideo)
	g.POST(scenes+"/:sceneId/prepare-chain", PrepareChain)

	// The AI director. The plan is STORED rather than returned and forgotten,
	// because a person reads a storyboard, thinks about it, and approves it —
	// possibly on another device. Approval materialises the plan into scenes;
	// the per-scene endpoints above then fill them.
	g.POST(base+"/studios/:studioId/plan", PlanRun)
	g.GET(base+"/studios/:studioId/runs", ListRuns)
	g.GET(base+"/studios/:studioId/runs/:runId", GetRun)
	g.POST(base+"/studios/:studioId/runs/:runId/approve", ApproveRun)

	// Rendering. The preflight answers "can this render, and what would it
	// cost" without spending anything — a Render button that only finds out by
	// trying is a button that charges for the discovery.
	g.GET(base+"/studios/:studioId/render-preflight", RenderPreflight)
	g.POST(base+"/studios/:studioId/render", RenderStudio)

	// Cast and locations: the mechanism that keeps one face and one room
	// across separate generated clips.
	g.PUT(base+"/studios/:studioId/cast", SetCast)
	g.PUT(base+"/studios/:studioId/locations", SetLocations)
}
