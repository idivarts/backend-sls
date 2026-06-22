// Package socialsync is the shared async pipe for slow Meta/social Graph-API
// work. Every social module (inbox DM sync, analytics, media, …) enqueues onto a
// SINGLE queue with a discriminated `type` + the params that op needs; the
// social_sqs worker dispatches by type. There is intentionally NO per-module
// queue — add a new OpType and a worker case instead.
package socialsync

import (
	"encoding/json"
	"os"

	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

// QueueEnv is the env var (set on trendly_v2 in serverless.trendly.yml) holding
// the shared social-sync queue URL.
const QueueEnv = "SOCIAL_SYNC_QUEUE_URL"

// OpType discriminates what the worker should run for a message.
type OpType = string

const (
	// Brand-wide bulk ops.
	OpInboxSync OpType = "inbox_sync" // pull DM conversations from Meta → Firestore
	OpAnalytics OpType = "analytics"  // build the analytics overview → Firestore (per-account when SocialID set)
	OpMedia     OpType = "media"      // pull published media → Firestore

	// Unit-level resyncs (refresh exactly one item that looks stale).
	OpProfileResync OpType = "profile_resync" // re-fetch a conversation contact's name/avatar
	OpThreadResync  OpType = "thread_resync"  // re-pull one DM thread's messages
	OpMessageResync OpType = "message_resync" // re-fetch one message (e.g. expired attachment)
	OpMediaResync   OpType = "media_resync"   // re-fetch one media item (counts + image)
)

// Message is the single payload shape for the shared queue. Only the fields a
// given op needs are populated (BrandID is always required).
type Message struct {
	Type    OpType `json:"type"`
	BrandID string `json:"brandId"`
	Range   string `json:"range,omitempty"` // analytics range ("7d" | "28d" | "90d")

	// Unit-level params (set per op; empty otherwise).
	SocialID       string `json:"socialId,omitempty"`
	Channel        string `json:"channel,omitempty"` // "instagram" | "facebook"
	ConversationID string `json:"conversationId,omitempty"`
	MessageID      string `json:"messageId,omitempty"`
	MediaID        string `json:"mediaId,omitempty"`
}

// Enqueue sends a job to the shared social queue. It returns queued=false (with a
// nil error) when no queue is configured (e.g. local dev) so the caller can fall
// back to running the work inline — the worker logic always writes to Firestore,
// so the inline path is behaviourally identical, just synchronous.
func Enqueue(msg Message) (queued bool, err error) {
	url := os.Getenv(QueueEnv)
	if url == "" {
		return false, nil
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return false, err
	}
	if err := sqshandler.SendToQueue(url, string(body), 0); err != nil {
		return false, err
	}
	return true, nil
}
