# CLAUDE.md — backend-sls

> **⚠️ Read the monorepo root before going further.**
> This file covers only `backend-sls`-specific detail. For the complete picture
> of the entire Trendly platform — every micro-repo, shared architecture,
> domain model, auth flow, contract lifecycle, Notion preferences, and more —
> read the parent first:
>
> - **Full monorepo context**: `../CLAUDE.md`
> - **Knowledge graph** (keyword → exact file path, token-efficient lookups): `../.claude/knowledge-graph.json`
>
> When working across repos (e.g. backend + mobile app), always load `../CLAUDE.md`
> so you have the full picture before touching any code.

**Read this before touching any code in this project.**
This file was migrated from `.cursor/rules/` and is the authoritative context for all AI-assisted work in this repo.

Reference plans for complex features: `.claude/plans/`

---

## Project Overview

This repository is the **Go serverless backend** for Trendly — a platform connecting brands, influencers, and collaborations. It powers:

1. **Trendly**: Matchmaking, discovery, monetization (Razorpay), social media integration, collaborations.
2. **Crowdy Chat (CC)**: Chat and messaging (websockets, SQS, Step Functions) — secondary app, separate serverless config.

---

## Technology Stack

- **Language**: Go 1.23 (primary), minor Node.js for specific scripts
- **Framework**: Gin HTTP router, deployed via `aws-lambda-go-api-proxy` inside Lambda
- **Infra**: AWS Lambda (arm64, `provided.al2`), API Gateway, S3, SQS, CloudFront
- **Serverless**: Framework v3 — `serverless.trendly.yml` (Trendly), `serverless.cc.yml` (CC)
- **Databases**:
  - **Firestore** — primary store for all core models
  - **PostgreSQL/RDS** — analytics and discovery (via GORM)
  - **BigQuery** — social data warehouse
  - DynamoDB — legacy only, not actively used for core models
- **External Services**: Stream Chat, Razorpay, SendGrid, Firebase Auth/FCM, OpenAI, Gemini, HubSpot, Apify, n8n

---

## Key Directory Structure

```
backend-sls/
├── functions/          # Lambda entry points — one main.go per function
├── internal/           # Business logic (not exported)
│   ├── trendlyapis/    # HTTP handlers for Trendly APIs
│   ├── ccapis/         # HTTP handlers for Crowdy Chat
│   ├── models/
│   │   ├── trendlymodels/  # Core Firestore models
│   │   ├── trendlyrdb/     # PostgreSQL models (discovery)
│   │   └── trendlybq/      # BigQuery models
│   ├── middlewares/        # ValidateSessionMiddleware, TrendlyMiddleware
│   ├── constants/          # App-wide Go constants
│   ├── matchmaking/        # AI matchmaking logic
│   ├── trendly_discovery/  # Influencer search engine
│   ├── openai/             # OpenAI integration
│   ├── s3/                 # S3 upload handlers
│   ├── websocket/          # WebSocket handlers
│   └── stream_sqs/         # Stream Chat SQS hooks
├── pkg/                # Shared utility packages (exported)
│   ├── api_handler/    # GinEngine singleton + StartLambda()
│   ├── firebase/       # Firebase Admin SDK (auth, firestore, messaging)
│   ├── instagram/      # Instagram Graph API client
│   ├── messenger/      # Facebook Messenger client
│   ├── payments/       # Razorpay wrappers
│   ├── myemail/        # SendGrid helpers
│   ├── gemini/         # Google Gemini AI
│   ├── myopenai/       # OpenAI assistant wrappers
│   ├── mys3/           # S3 upload/get
│   ├── hubspot/        # HubSpot CRM
│   ├── n8n/            # n8n automation webhooks
│   ├── apify/          # Apify Instagram scraper
│   └── delayed_sqs/    # SQS delayed message helpers
├── templates/          # SendGrid HTML email templates (30+)
├── scripts/            # Standalone scripts / cron handlers
├── postman/            # Postman collection JSONs
└── serverless.trendly.yml  # Primary infra config
```

---

## Lambda Functions Reference

| Function | Entry | Route Prefix | Purpose |
|---|---|---|---|
| `trendly_v2` | `functions/trendly_v2/main.go` | `/api/v2` | Socials, brand creation, chat auth |
| `trendly_collabs` | `functions/trendly_collabs/main.go` | `/api/collabs` | Collaborations, applications, contracts |
| `trendly_monetize` | `functions/trendly_monetize/main.go` | `/monetize` | Payments, shipments, deliverables, KYC |
| `trendly_discovery` | `functions/trendly_discovery/main.go` | `/discovery` | Influencer search |
| `trendly_influencers` | `functions/trendly_influencers/main.go` | `/api/influencers` | Direct brand→influencer invites |
| `trendly_matchmaking` | `functions/trendly_matchmaking/main.go` | (cron) | AI matchmaking |
| `razorpay/apis` | `functions/razorpay/apis/main.go` | `/razorpay` | Subscription/plan APIs |
| `razorpay/webhook` | `functions/razorpay/webhook/main.go` | `/razorpay/webhook` | Payment webhooks |
| `stream_sqs_hook` | `functions/stream_sqs_hook/main.go` | (SQS) | Stream Chat event hook |
| `unauth_apis` | `functions/unauth_apis/main.go` | `/onboard`, `/instagram`, `/firebase` | Public: signup, IG OAuth |
| `websocket` | `functions/websocket/main.go` | (WS) | WebSocket connect/disconnect |
| `s3` | `functions/s3/main.go` | `/s3` | S3 pre-signed uploads |

---

## Go Coding Conventions

- **Struct tags**: Always include both `json:"..."` and `firestore:"..."` tags on model structs.
- **Error handling**: Explicit checks everywhere. Return `c.JSON(http.StatusBadRequest, ...)` on errors — no panics.
- **Lambda entry**: Every `functions/*/main.go` calls `apihandler.StartLambda()` as the last line.
- **Build tags**: `go build -tags lambda.norpc` (arm64, `provided.al2` runtime).
- **Route groups**: Use `apihandler.GinEngine.Group(...)` with `middlewares.ValidateSessionMiddleware()` and `middlewares.TrendlyMiddleware("users"|"managers")`.
- **Dependency injection**: Clients are initialized per-package (e.g., `pkg/firebase/init.go`) and referenced globally within that package. No constructor injection.
- **No DynamoDB for new code**: Use Firestore for new models. DynamoDB references in the codebase are legacy.

### ⭐ STANDING RULE — All Firestore access goes through a model (NEVER raw queries in handlers)

**Never call `firestoredb.Client.Collection(...)` (or `.Doc/.Where/.Set/.Update/
.Delete/.Add/.CollectionGroup`) directly from a handler, AI tool, websocket
handler, middleware, or any non-model package.** Every Firestore
collection/subcollection MUST have a model struct in
`internal/models/trendlymodels/` that owns all of its reads, writes, and deletes
behind wrapped functions. Callers use those functions only.

When you add or touch a collection:
1. **Model first.** Create/extend the struct in `internal/models/trendlymodels/`
   with `json:"..."` + `firestore:"..."` tags on every field, and an
   `ID string `firestore:"-"`` populated from `doc.Ref.ID` on reads.
2. **Wrap every operation** as a function/method on that file:
   - typed read → returns the struct (`Get…`, `List…`),
   - create → `Create…(parentID, …)` (a `map[string]any` payload is fine when
     many ad-hoc fields are stamped — the `.Add`/`.Set` call still lives in the model),
   - partial update → accept `[]firestore.Update` (matches `contract.Update`,
     `social_v2`), and the `.Update` call lives in the model,
   - delete / range-delete → `Delete…` in the model.
3. **`firestoredb.Client` may only appear inside `internal/models/trendlymodels/`
   files** (plus the already-encapsulated `pkg/openrouter` wrapper). If you find
   yourself reaching for it elsewhere, add the missing model function instead.
4. Keep the Firestore rules & indexes in sync (see §3 in the root CLAUDE.md).

Existing examples to copy: `content.go`, `strategy.go`, `websocket.go`,
`share_link.go`, `social_account_index.go`.

### Route Pattern
```go
// functions/<name>/main.go
handler := apihandler.GinEngine.Group("/basePath", middlewares.ValidateSessionMiddleware())
userGroup := handler.Group("/", middlewares.TrendlyMiddleware("users"))
userGroup.POST("/action", handlerPackage.HandlerFunc)
// → POST {{baseUrl}}/basePath/action  (requires user session)

managerGroup := handler.Group("/", middlewares.TrendlyMiddleware("managers"))
managerGroup.POST("/action", handlerPackage.HandlerFunc)
// → POST {{baseUrl}}/basePath/action  (requires manager/brand session)
```

### Request Binding Pattern
```go
type MyReq struct {
    Name  string `json:"name" binding:"required"`
    Notes string `json:"notes"`  // optional — no binding tag
}
var req MyReq
if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
}
```

---

## Data Models

### Core Firestore Models (`internal/models/trendlymodels/`)
| File | Struct | Firestore Collection |
|---|---|---|
| `user.go` | `User` | `users` |
| `brand.go` | `Brand` | `brands` |
| `brand_member.go` | `BrandMember` | (subcollection) |
| `manager.go` | `Manager` | `managers` |
| `collaboration.go` | `Collaboration` | `collaborations` |
| `application.go` | `Application` | (subcollection) |
| `contract.go` | `Contract` | `contracts` |
| `invite.go` | `Invite` | (subcollection) |
| `influencer_invite.go` | `InfluencerInvite` | (subcollection) |
| `social.go` | `Social` | (subcollection) |
| `notification.go` | `Notification` | (subcollection) |
| `monetization_enums.go` | enums | — |

### PostgreSQL Models (`internal/models/trendlyrdb/`) — Discovery only
`influencers.go`, `socials.go`, `insta_posts.go`, `niche_counts.go`

---

## Email System

### Directory
- `templates/` — HTML files (one per email event)
- `templates/init.go` — `TemplatePath` constants for each template file
- `templates/subject.go` — Subject line string constants
- `pkg/myemail/main.go` — SendGrid sending helpers

### Template Format Rules
Every template HTML file **must** start with a comment block listing all dynamic variables:
```html
<!--
  Dynamic Variables:
    {{.RecipientName}} => Name of the recipient
    {{.CollabTitle}}   => Collaboration title
    {{.Link}}          => Action URL
-->
```
Use `{{.VariableName}}` throughout. Match styles from an existing template (e.g. `influencer_accepted.html`) for brand consistency.

### Sending Pattern
```go
data := map[string]interface{}{
    "BrandMemberName": brand.Name,
    "InfluencerName":  user.Name,
    "CollabTitle":     collab.Name,
    "Link":            fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
}
// Single recipient
err = myemail.SendCustomHTMLEmail(recipientEmail, templates.MyTemplate, templates.SubjectMyTemplate, data)

// Multiple recipients
err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.MyTemplate, templates.SubjectMyTemplate, data)
```

### Adding a New Email — Checklist
1. Create `templates/<event_name>.html` with the variable comment block
2. Add `MyTemplate myemail.TemplatePath = "templates/<event_name>.html"` to `templates/init.go`
3. Add `SubjectMyTemplate = "Your subject line"` to `templates/subject.go`
4. Call `myemail.SendCustomHTMLEmail(...)` from the relevant handler

### Available Sending Functions (`pkg/myemail/main.go`)
- `SendCustomHTMLEmail(toEmail string, templatePath TemplatePath, subject string, data map[string]interface{}) error`
- `SendCustomHTMLEmailToMultipleRecipients(toEmails []string, templatePath TemplatePath, subject string, data map[string]interface{}) error`
- `SendEmailUsingTemplate(toEmail, templateID string, dynamicData map[string]interface{}) error` — SendGrid dynamic templates (less common)

---

## Postman Collection Generation

**Apply when**: Asked to create or update a Postman collection for any set of APIs.

### Output Rules
- Format: **Postman Collection v2.1 JSON** (schema `https://schema.getpostman.com/json/collection/v2.1.0/collection.json`)
- Save to: `postman/<name>_collection.json`
- Collection name: `"Trendly Backend"` (so it merges with existing collection on re-import)
- Base URL: `{{baseUrl}}` variable — user already has this in their Postman environment
- Auth: **Do NOT configure** auth at collection or folder level — user has Bearer token configured globally

### Folder Structure
- One top-level named folder per domain (e.g., "Monetization", "Collaborations")
- Sub-folders grouped by actor + domain (e.g., "Brand - Orders", "Influencer - Shipment", "Webhooks")
- Each sub-folder has a short `description`

### Extracting API Info
1. Read `functions/<name>/main.go` → base route group + sub-groups + all routes
2. Read handler in `internal/trendlyapis/<domain>/<file>.go` → extract:
   - Request body: look for `c.ShouldBindJSON(&req)` → struct fields with `json:"..."` tags
   - Path params: `c.Param("name")` → `:name` in URL
   - Query params: `c.Query("name")` → query params
   - Required vs optional: `binding:"required"` tag = required
   - Response: `c.JSON(http.StatusOK, gin.H{...})` at end of handler

### Sample Data Guidelines
- Use realistic Indian data: PAN `ABCDE1234F`, IFSC `HDFC0001234`, cities Mumbai/Bangalore
- Date fields: epoch milliseconds int64 (e.g. `1740000000000`)
- File/image fields: `https://example.com/proof.jpg`
- No-body endpoints: `"{}"`; GET requests: no body section

### Existing Collections
- `postman/monetization_collection.json` — Monetization APIs
- `postman/trendly_collabs_collection.json` — Collaborations APIs
- `postman/discovery_collection.json` — Discovery APIs

---

## Deploy

```bash
# Deploy prod
sls deploy --stage prod --config serverless.trendly.yml

# Deploy dev
sls deploy --stage dev --config serverless.trendly.yml
```

---

## Key Env Vars
`FB_CLIENT_SECRET`, `INSTA_CLIENT_SECRET`, `STREAM_SECRET`, `JWT_ENCODE_KEY`,
`OPENAI_API_KEY`, `SENDGRID_API_KEY`, `HUBSPOT_API_KEY`
