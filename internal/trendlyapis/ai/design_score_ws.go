package ai

import (
	"context"
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type designScorePayload struct {
	BrandID   string `json:"brandId"`
	ContextID string `json:"contextId"`
}

// handleDesignScoreWS scores the content's current design.
//
// Websocket rather than HTTP, for two reasons that both came out of trying it
// the other way. Scoring a design means RENDERING it first, and the renderer is
// reached by invoking another lambda whose name is only on this function's
// environment — the HTTP AI lambda would have reported "not configured" and
// quietly scored the copy instead. And render-plus-vision comfortably exceeds
// the 29-second API Gateway integration timeout that no lambda timeout can
// raise.
func handleDesignScoreWS(req WSRequest) {
	var p designScorePayload
	if err := decodePayload(req.Payload, &p); err != nil {
		wsErrorTo(req.ConnectionID, "invalid payload: "+err.Error())
		return
	}
	if req.BrandID != "" {
		p.BrandID = req.BrandID
	}
	if !verifyBrandAccess(p.BrandID, req.UserID) {
		wsErrorTo(req.ConnectionID, "forbidden")
		return
	}
	if p.ContextID == "" {
		wsErrorTo(req.ConnectionID, "no content to score")
		return
	}

	// TaskDesign, not TaskScore: this judge has to SEE the render, and every
	// model on the design task is vision-capable for exactly that reason. The
	// score task's list is picked for cheap reproducibility on text and would
	// quietly drop the images.
	model, locked := pickModel(context.Background(), p.BrandID, openrouter.TaskDesign, req.Model)
	if locked {
		wsSend(req.ConnectionID, map[string]any{"type": "upgrade_required", "task": string(openrouter.TaskDesign)})
		return
	}
	orgID, _ := orgIDForBrand(p.BrandID)
	if aiTokensExhausted(orgID) {
		wsSend(req.ConnectionID, map[string]any{"type": "upgrade_required", "reason": "tokens_exhausted", "task": string(openrouter.TaskDesign)})
		return
	}

	score, usage, err := ScoreDesign(context.Background(), p.BrandID, p.ContextID, model)
	// Meter whatever was spent even on the failure path: the tokens left the
	// wallet either way.
	meterAIUsage(orgID, usage)
	if err != nil {
		wsErrorTo(req.ConnectionID, err.Error())
		return
	}

	// Persist through FirestoreMap, never the struct: Firestore ignores `json:`
	// tags and would store the Go field names.
	if doc, derr := score.FirestoreMap(); derr != nil {
		log.Printf("ai design score encode brand=%s content=%s: %v", p.BrandID, p.ContextID, derr)
	} else if derr := trendlymodels.UpdateContentFields(p.BrandID, p.ContextID,
		map[string]any{"designScore": doc}); derr != nil {
		log.Printf("ai design score persist brand=%s content=%s: %v", p.BrandID, p.ContextID, derr)
	}

	wsSend(req.ConnectionID, map[string]any{
		"type":  "design_score",
		"score": score,
		"usage": usage,
	})
}
