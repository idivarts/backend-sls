package ai

import (
	"context"
	"strings"
)

// pushToCalendarWSPayload is the per-job payload sent inside a `push_to_calendar`
// WebSocket frame's `payload`. brandId arrives on the envelope (req.BrandID) and
// strategyId on req.ContextID.
type pushToCalendarWSPayload struct {
	JobID            string `json:"jobId"`
	StartDate        string `json:"startDate"` // YYYY-MM-DD
	DurationDays     int    `json:"durationDays"`
	OverrideExisting bool   `json:"overrideExisting"`
}

// handlePushToCalendarWS runs the (potentially slow) strategy→calendar expansion
// over the WebSocket so it never hits the 30s HTTP limit. It streams namespaced
// `calendar_*` frames (carrying the client's jobId so the modal can correlate)
// and ends on `calendar_done` or `calendar_error`. The namespacing keeps these
// frames from colliding with the AI chat's `token`/`done`/`error` frames on the
// shared socket.
func handlePushToCalendarWS(req WSRequest) {
	brandID := strings.TrimSpace(req.BrandID)
	strategyID := strings.TrimSpace(req.ContextID)

	var p pushToCalendarWSPayload
	if err := decodePayload(req.Payload, &p); err != nil {
		wsSend(req.ConnectionID, map[string]any{
			"type": "calendar_error", "message": "invalid payload: " + err.Error(),
		})
		return
	}
	jobID := p.JobID

	fail := func(msg string) {
		wsSend(req.ConnectionID, map[string]any{
			"type": "calendar_error", "jobId": jobID, "message": msg,
		})
	}

	if strategyID == "" {
		fail("strategyId (contextId) is required")
		return
	}
	if !verifyBrandAccess(brandID, req.UserID) {
		fail("forbidden")
		return
	}

	progress := func(phase, message string, extra map[string]any) {
		m := map[string]any{
			"type":    "calendar_status",
			"jobId":   jobID,
			"phase":   phase,
			"message": message,
		}
		for k, v := range extra {
			m[k] = v
		}
		wsSend(req.ConnectionID, m)
	}

	res, err := runPushToCalendar(
		context.Background(), brandID, strategyID, req.UserID,
		p.StartDate, p.DurationDays, p.OverrideExisting, progress,
	)
	if err != nil {
		fail(err.Error())
		return
	}

	wsSend(req.ConnectionID, map[string]any{
		"type":         "calendar_done",
		"jobId":        jobID,
		"createdCount": len(res.CreatedItemIds),
		"removedCount": len(res.RemovedItemIds),
		"startDate":    res.StartDateStr,
		"endDate":      res.EndDateStr,
	})
}
