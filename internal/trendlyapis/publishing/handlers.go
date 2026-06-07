package publishing

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	delayedsqs "github.com/idivarts/backend-sls/pkg/delayed_sqs"
)

// ScheduleMessage is the payload placed on the delayed-publish queue.
type ScheduleMessage struct {
	BrandID   string `json:"brandId"`
	ContentID string `json:"contentId"`
	Action    string `json:"action"`
}

// PublishNow publishes the content immediately to all of its destinations.
// POST /api/v2/brands/:brandId/contents/:contentId/publish
func PublishNow(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")

	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarPublish); !ok {
		return
	}

	if err := PublishContent(brandID, contentID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "message": "Publish failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Published"})
}

// SchedulePublish enqueues a delayed publish via the delayed_sqs state machine.
// POST /api/v2/brands/:brandId/contents/:contentId/schedule  { "scheduledAt": <epoch ms> }
func SchedulePublish(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")

	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarPublish); !ok {
		return
	}

	var req struct {
		ScheduledAt int64 `json:"scheduledAt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delaySeconds := (req.ScheduledAt - time.Now().UnixMilli()) / 1000
	if delaySeconds < 0 {
		delaySeconds = 0
	}

	msg := ScheduleMessage{BrandID: brandID, ContentID: contentID, Action: "PUBLISH"}
	jData, err := json.Marshal(msg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	exec, err := delayedsqs.Send(string(jData), delaySeconds)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "message": "Could not schedule"})
		return
	}

	fields := map[string]interface{}{
		"status":               "scheduled",
		"scheduleMode":         "scheduled",
		"scheduledAt":          req.ScheduledAt,
		"scheduleExecutionArn": "",
	}
	if exec != nil && exec.ExecutionArn != nil {
		fields["scheduleExecutionArn"] = *exec.ExecutionArn
	}
	if err := trendlymodels.UpdateContentFields(brandID, contentID, fields); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Scheduled", "scheduledAt": req.ScheduledAt})
}

// CancelSchedule stops a pending scheduled publish and reverts the content to approved.
// DELETE /api/v2/brands/:brandId/contents/:contentId/schedule
func CancelSchedule(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")

	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarPublish); !ok {
		return
	}

	ct, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if ct.ScheduleExecutionArn != "" {
		arn := ct.ScheduleExecutionArn
		if serr := delayedsqs.StopExecutions(&arn); serr != nil {
			// The execution may already have fired/expired — log and continue.
			log.Println("CancelSchedule: stop execution error:", serr)
		}
	}
	fields := map[string]interface{}{
		"status":               "approved",
		"scheduleExecutionArn": "",
	}
	if err := trendlymodels.UpdateContentFields(brandID, contentID, fields); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Schedule cancelled"})
}
