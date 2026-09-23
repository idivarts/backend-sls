package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/socialsync"
	"github.com/idivarts/backend-sls/internal/trendlyapis/analytics"
	"github.com/idivarts/backend-sls/internal/trendlyapis/inbox"
	"github.com/idivarts/backend-sls/internal/trendlyapis/social_connect"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// handler is the single worker for all slow Meta/social Graph-API jobs. It
// dispatches by the message's discriminated type and runs the work off the HTTP
// request path (600s budget). Each op writes its results to Firestore as it goes,
// so the brand's UI fills in live via its Firestore listeners. Errors are logged
// per-record so one brand's failure doesn't replay the whole batch.
func handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, record := range sqsEvent.Records {
		var msg socialsync.Message
		if err := json.Unmarshal([]byte(record.Body), &msg); err != nil {
			log.Println("social_sqs: bad message:", err, record.Body)
			mysentry.Capture(err, map[string]string{"lambda": "social_sqs", "op": "unmarshal"})
			continue
		}
		if msg.BrandID == "" {
			log.Println("social_sqs: missing brandId, skipping:", record.Body)
			continue
		}
		switch msg.Type {
		case socialsync.OpAnalytics:
			if err := analytics.Refresh(msg.BrandID, msg.Range, msg.SocialID); err != nil {
				log.Printf("social_sqs: analytics refresh failed for %s: %v", msg.BrandID, err)
				mysentry.Capture(err, map[string]string{"lambda": "social_sqs", "op": "analytics", "brandId": msg.BrandID})
			}
		case socialsync.OpMedia:
			if err := inbox.RefreshMedia(msg.BrandID); err != nil {
				log.Printf("social_sqs: media refresh failed for %s: %v", msg.BrandID, err)
				mysentry.Capture(err, map[string]string{"lambda": "social_sqs", "op": "media", "brandId": msg.BrandID})
			}
		case socialsync.OpProfileResync:
			if err := inbox.ResyncProfile(msg.BrandID, msg.ConversationID); err != nil {
				log.Printf("social_sqs: profile resync failed for %s/%s: %v", msg.BrandID, msg.ConversationID, err)
				mysentry.Capture(err, map[string]string{"lambda": "social_sqs", "op": "profile_resync", "brandId": msg.BrandID})
			}
		case socialsync.OpThreadResync:
			if err := inbox.ResyncThread(msg.BrandID, msg.ConversationID); err != nil {
				log.Printf("social_sqs: thread resync failed for %s/%s: %v", msg.BrandID, msg.ConversationID, err)
				mysentry.Capture(err, map[string]string{"lambda": "social_sqs", "op": "thread_resync", "brandId": msg.BrandID})
			}
		case socialsync.OpMessageResync:
			if err := inbox.ResyncMessage(msg.BrandID, msg.ConversationID, msg.MessageID); err != nil {
				log.Printf("social_sqs: message resync failed for %s/%s/%s: %v", msg.BrandID, msg.ConversationID, msg.MessageID, err)
				mysentry.Capture(err, map[string]string{"lambda": "social_sqs", "op": "message_resync", "brandId": msg.BrandID})
			}
		case socialsync.OpMediaResync:
			if err := inbox.ResyncMediaItem(msg.BrandID, msg.MediaID, msg.SocialID, msg.Channel); err != nil {
				log.Printf("social_sqs: media item resync failed for %s/%s: %v", msg.BrandID, msg.MediaID, err)
				mysentry.Capture(err, map[string]string{"lambda": "social_sqs", "op": "media_item_resync", "brandId": msg.BrandID})
			}
		case socialsync.OpDisconnectCleanup:
			if msg.SocialID == "" {
				log.Println("social_sqs: disconnect cleanup missing socialId, skipping:", record.Body)
				continue
			}
			social_connect.CleanupBrandSocialData(msg.BrandID, msg.SocialID)
		default: // OpInboxSync (and any unknown type) → DM sync
			if err := inbox.SyncFromMeta(msg.BrandID); err != nil {
				log.Printf("social_sqs: inbox sync failed for %s: %v", msg.BrandID, err)
				mysentry.Capture(err, map[string]string{"lambda": "social_sqs", "op": "inbox_sync", "brandId": msg.BrandID})
			}
		}
	}
	return nil
}

func main() {
	// Non-Gin entry point, so it never passes through the Sentry middleware on
	// the shared Gin engine. Wrap captures errors and panics and — the part
	// that matters — flushes before Lambda freezes the environment.
	mysentry.Init()
	lambda.Start(mysentry.Wrap(handler))
}
