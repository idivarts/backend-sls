package publishing

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	delayedsqs "github.com/idivarts/backend-sls/pkg/delayed_sqs"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

// ScheduleMessage is the payload placed on the content-publish queue (for both
// scheduled publishes and instant publish-now / retry jobs).
type ScheduleMessage struct {
	BrandID   string `json:"brandId"`
	ContentID string `json:"contentId"`
	Action    string `json:"action"`
	// OnlyDestinations, when set, limits the run to these destination
	// socialAccountIds (Retry re-publishes just the failed socials). Empty =
	// publish to every destination.
	OnlyDestinations []string `json:"onlyDestinations,omitempty"`
}

// enqueuePublish hands a publish job to the shared content-publish queue with no
// delay — consumed by the scheduled_publish_sqs worker, which runs PublishContent
// off the HTTP path (no 30s API Gateway limit). When no queue is configured
// (local dev), it falls back to publishing inline so behaviour is identical, just
// synchronous.
func enqueuePublish(msg ScheduleMessage) error {
	if os.Getenv("SEND_MESSAGE_QUEUE_ARN") == "" {
		return PublishContent(msg.BrandID, msg.ContentID, msg.OnlyDestinations...)
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return sqshandler.SendToMessageQueue(string(body), 0)
}

// PublishNow starts publishing the content to all of its destinations. The work
// is queued (not run inline) so slow multi-platform / video uploads never hit the
// API Gateway timeout; the doc is flipped to "publishing" immediately and the
// per-social results fill in live via the app's Firestore listener.
// POST /api/v2/brands/:brandId/contents/:contentId/publish
func PublishNow(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")

	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarPublish); !ok {
		return
	}

	// Reflect the in-flight state the instant we return, before the worker starts.
	if err := SeedPublishing(brandID, contentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Nothing to publish"})
		return
	}
	if err := enqueuePublish(ScheduleMessage{BrandID: brandID, ContentID: contentID, Action: "PUBLISH"}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "message": "Could not start publishing"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Publishing"})
}

// RetryPublish re-publishes ONLY the destinations that failed on the last run,
// leaving already-published socials untouched.
// POST /api/v2/brands/:brandId/contents/:contentId/publish/retry
func RetryPublish(c *gin.Context) {
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
	failed := []string{}
	for _, r := range ct.PublishResults {
		if r.Status == pubStatusFailed && r.SocialAccountID != "" {
			failed = append(failed, r.SocialAccountID)
		}
	}
	if len(failed) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "No failed destinations to retry"})
		return
	}

	if err := SeedPublishing(brandID, contentID, failed...); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := enqueuePublish(ScheduleMessage{BrandID: brandID, ContentID: contentID, Action: "PUBLISH", OnlyDestinations: failed}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "message": "Could not start publishing"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Retrying", "destinations": failed})
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
