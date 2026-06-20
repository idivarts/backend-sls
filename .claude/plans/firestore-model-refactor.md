# Firestore Model Refactor — wrap raw queries behind `trendlymodels`

**Date:** 2026-06-11
**Owner:** backend-sls
**Status:** executing (no review gate — direct execution per request)

## Problem

Several recently-added collections / subcollections are read & written with raw
`firestoredb.Client.Collection(...)...` calls scattered directly inside HTTP
handlers, AI tools and the websocket layer — bypassing the `internal/models/trendlymodels`
layer. This is the same anti-pattern the repo otherwise avoids: every core
collection (users, brands, contracts, …) has a model struct that owns its
Firestore access. The new code drifted from that.

## Goal

Every Firestore collection/subcollection has a **model struct** in
`internal/models/trendlymodels/` that owns ALL its reads/writes/deletes via
wrapped functions. Handlers, AI tools and the websocket layer call those
functions and never touch `firestoredb.Client` directly.

## Scope — collections to fix

| Collection / subcollection | Model file | Action |
|---|---|---|
| `brands/{brandId}/strategies` (+ `…/yupdates`) | `strategy.go` | **new** |
| `websockets` | `websocket.go` | **new** |
| `shareLinks` | `share_link.go` | **new** |
| `brands/{brandId}/contents` | `content.go` | **extend** (create/update/list/delete/range) |
| `brands` (onboarding writes/reads) | `brand.go` | **extend** (`UpdateFields`) |
| `brands/{brandId}/members` (access check) | `brand_member.go` | reuse existing `Get` |

### Intentionally left as-is (already encapsulated — not scattered raw queries)
- `ai_conversations` (+ `messages`) — wrapped in `pkg/openrouter/conversation.go`,
  already backed by `trendlymodels.AIConversation` / `AIMessage`.
- `ai_config` — config registry wrapped in `pkg/openrouter/models.go` (not a
  domain model; openrouter-internal).
- `users`/`managers` raw reads in middlewares (`trendly_mw.go`,
  `crowdychat_mw.go`) — they stash the raw `.Data()` map into the Gin context;
  retyping ripples through every consumer. Out of scope for this pass.
- CrowdyChat collections (`leads`, `sources`, `campaigns`, `conversations`,
  `leadStages`, `collectibles`) — separate app, already have models under
  `internal/models/`.
- `scripts/**` one-off migration/seed tools.

## Call sites to refactor

**strategies** → `strategy.go`
- `internal/trendlyapis/ai/strategy_tools.go` (`strategyDocRef`, set/generate/apply, `resetStrategyCRDT`)
- `internal/trendlyapis/ai/strategy_routes.go` (`runPushToCalendar` read+finalize, `HTTPRecheckDuration`)
- `internal/trendlyapis/ai/context.go` (`loadModuleContext` strategy read)
- `internal/trendlyapis/ai/onboarding_init.go` (create strategy)
- `internal/trendlyapis/ai/tools/strategy.go` (`get_strategy_content`)

**contents** → `content.go`
- `internal/trendlyapis/ai/calendar_tools.go` (`contentsCollection`/`contentDocRef`, create/update/move/remove/list)
- `internal/trendlyapis/ai/strategy_routes.go` (`runPushToCalendar` create/delete-in-range)
- `internal/trendlyapis/ai/context.go` (`loadContentBrief`, `loadCalendarMonth`)
- `internal/trendlyapis/ai/onboarding_init.go` (create content)
- `internal/trendlyapis/ai/tools/calendar.go` (`get_calendar_posts`)
- `internal/trendlyapis/ai/tools/campaigns.go` (`get_campaign_performance`)
- `internal/trendlyapis/unauth_apis/public_share.go` (calendar-month query)

**websockets** → `websocket.go`
- `internal/websocket/connect.go`, `disconnect.go`, `receive.go`, `var.go`
- `pkg/ws_handler/broadcast.go`, `send.go`, `user.go`, `init.go`

**shareLinks** → `share_link.go`
- `internal/trendlyapis/unauth_apis/public_share.go`

**brands** → `brand.go` `UpdateFields` + typed reads
- `internal/trendlyapis/ai/onboarding_tools.go` (`setBrandFields`, `completeOnboarding`)
- `internal/trendlyapis/ai/context.go` (`loadOnboardingState`)
- `internal/trendlyapis/ai/context.go` `verifyBrandAccess` → `BrandMember.Get`

## Conventions to follow (match existing models)
- Model methods take/return typed structs; for partial updates accept
  `[]firestore.Update` (established by `social_v2`/`contract.Update`).
- Map-based `Add` writes that stamp many ad-hoc fields keep a
  `Create…(brandID, map)` wrapper so the model owns the `.Add`.
- `firestore:"-"` for the `ID` field, populated from `doc.Ref.ID` on reads.
- All Firestore access uses `firestoredb.Client` **inside the model file only**.

## Verification
- `go build ./...` after each cluster.
- `go vet ./...` at the end.
- No `firestoredb.Client` / `.Collection(` left in the refactored handler files
  (grep check).

## Standing rule
Add a "Firestore access goes through a model" rule to both `CLAUDE.md` files so
this doesn't regress.
