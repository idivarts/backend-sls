# Inbox Webhooks — Meta Setup & Testing Runbook

Hands-on guide to **configure the Meta webhooks** the Inbox depends on and
**test all four delivery paths** end-to-end:

1. Instagram **message** (DM)
2. Instagram **comment**
3. Facebook **message** (Messenger DM)
4. Facebook **comment**

> Companion docs:
> - [`inbox-meta-permissions.md`](./inbox-meta-permissions.md) — scopes & App Review.
> - [`social-oauth-setup.md`](./social-oauth-setup.md) — base OAuth/connect.
>
> This runbook is the practical "wire it up and prove it works" checklist.

---

## 0. How the backend receives webhooks

All Meta events hit a single Lambda (`functions/message_webhook`):

| Method / Path | Handler | Purpose |
|---|---|---|
| `GET  /webhooks/instagram` | `Validation` | Subscription verification (`hub.challenge`) |
| `POST /webhooks/instagram` | `Receive` | IG events (DMs + comments) |
| `GET  /webhooks/facebook`  | `Validation` | Subscription verification |
| `POST /webhooks/facebook`  | `Receive` | FB events (DMs + comments) |
| `POST /webhooks/data-deletion` | `DataDeletion` | Meta data-deletion callback |

Inside `Receive` (`internal/message_webhook/receive.go`):
- `entry[].messaging[]` → `inbox.IngestMessaging()` (DMs).
- `entry[].changes[]` → `inbox.IngestComment()` (comments/feed/mentions).
- Events are routed to a brand via `socialAccountIndex/{platformAccountId}`
  (doc id = the IG Business Account id or FB Page id, i.e. `entry.id`). Events
  for accounts not in that index are ignored.

**Prod base:** `https://be.trendly.now` — **Dev base:** `https://be.trendly.now/dev`

---

## 1. Environment variables

| Var | Used for | Default | Notes |
|---|---|---|---|
| `WEBHOOK_VERIFY_TOKEN` | `GET` subscription verify | `mytoken` | Must match the **Verify Token** typed into the Meta dashboard. |
| `WEBHOOK_STRICT_SIGNATURE` | `POST` signature gate | unset (lenient) | When `true`, a bad `X-Hub-Signature-256` is rejected (403). Leave **off** during initial testing, flip **on** once app secrets are confirmed. |
| `FB_CLIENT_SECRET` / `INSTA_CLIENT_SECRET` | HMAC signature key | — | Required for signature verification to pass in strict mode. |

---

## 2. One-time Meta App dashboard setup

> Do this on the **dev** app first, then repeat on prod.

### 2.1 Products
- Add **Messenger**, **Instagram**, and **Webhooks** products to the app.
- Business Verification must be complete before Advanced Access is granted
  (start early — see permissions doc).

### 2.2 Webhook subscriptions (callback + verify token)
In **App Dashboard → Webhooks**, configure each object:

**Instagram object:**
- Callback URL: `https://be.trendly.now/dev/webhooks/instagram`
- Verify Token: value of `WEBHOOK_VERIFY_TOKEN`
- Subscribe fields: **`messages`**, **`comments`**, **`mentions`**

**Page object (Facebook):**
- Callback URL: `https://be.trendly.now/dev/webhooks/facebook`
- Verify Token: value of `WEBHOOK_VERIFY_TOKEN`
- Subscribe fields: **`messages`**, **`message_echoes`**, **`messaging_postbacks`** (optional), **`feed`**

Clicking **Verify and Save** triggers `GET /webhooks/...`; a green check means
the challenge round-trip succeeded (token matched).

### 2.3 Per-Page / per-IG subscription
App-level field subscriptions are not enough — each Page (and its linked IG
Business Account) must be subscribed to the app. On connect, the Facebook OAuth
callback calls `SubscribeApp()` per Page (`pkg/messenger/subscribe_app.go`),
which subscribes the extended field list. For a manually-connected test Page,
confirm the subscription via:

```
GET https://graph.facebook.com/v19.0/{page-id}/subscribed_apps?access_token={page-token}
```

### 2.4 Confirm the account is in the routing index
For an event to land in a brand's inbox, `socialAccountIndex/{platformAccountId}`
must exist with `app: "brands"` and the owning `brandId`/`socialId`. This is
written when a brand connects the account. Verify in Firestore before testing.

---

## 3. Quick verification (subscription handshake)

```bash
# Should echo back the challenge value (200, plain text "test123")
curl "https://be.trendly.now/dev/webhooks/instagram?hub.mode=subscribe&hub.verify_token=$WEBHOOK_VERIFY_TOKEN&hub.challenge=test123"
curl "https://be.trendly.now/dev/webhooks/facebook?hub.mode=subscribe&hub.verify_token=$WEBHOOK_VERIFY_TOKEN&hub.challenge=test123"
```

If you get an empty/empty-string body, the token didn't match.

---

## 4. The 4-path test matrix

For each path: trigger the real event, watch the Lambda logs, confirm the
Firestore doc, confirm it appears in the brand app Inbox, then reply and confirm
the reply lands back on the platform.

> **Prereq:** a brand in the dev app with a connected IG Business Account and/or
> Facebook Page (its `socialAccountIndex` entry present), and — for live data —
> the messaging/comment scopes granted (Advanced Access or the connected account
> added as a **tester/role** on the dev app so dev-mode permissions apply).

| # | Path | Trigger | Webhook field | Ingest fn | Firestore doc |
|---|---|---|---|---|---|
| 1 | IG message | From a test IG account, DM the connected IG business account | `messages` | `IngestMessaging` | `brands/{b}/inbox/dmwh_{socialId}_{contactId}` |
| 2 | IG comment | Comment on one of the IG account's posts | `comments` | `IngestComment` | `brands/{b}/inbox/cmt_{commentId}` |
| 3 | FB message | Message the connected Page from another FB user | `messages` | `IngestMessaging` | `brands/{b}/inbox/dmwh_{socialId}_{contactId}` |
| 4 | FB comment | Comment on a Page post | `feed` (item=`comment`) | `IngestComment` | `brands/{b}/inbox/cmt_{commentId}` |

### Per-path checklist
1. **Trigger** the event from a *second* account (Meta won't deliver `messages`
   for your own outbound unless via `message_echoes`).
2. **Logs:** `sls logs -f message_webhook --stage dev -t` (or CloudWatch). Look
   for the POST and absence of `signature verification failed` (or set
   `WEBHOOK_STRICT_SIGNATURE` off while testing).
3. **Firestore:** the doc above is created/updated; `unread:true`, `preview` set,
   `lastActivityAt` recent. DMs carry `replyWindowExpiresAt` (~24h out).
4. **Inbox UI:** the conversation/comment shows in the brand app
   (Messages tab for DMs + new comments; Media tab to browse a post's comments).
5. **Reply:** reply from the app →
   `POST /api/v2/brands/:brandId/inbox/conversations/:id/reply` (or, from the
   Media tab, `POST .../inbox/comments/:commentId/reply`). Confirm the reply
   appears on Instagram/Facebook.
6. **Edge cases:**
   - **Public reply ingestion:** reply to a stored comment from another account →
     it appends to `comment.replies[]` on the parent `cmt_*` doc (handled by
     `ingestCommentReply`). IG threads are one level deep (reply `parent_id` is
     the top-level comment).
   - **DM attachment:** send an image DM → the message stores `attachmentUrl` and
     the list preview shows `📎 Attachment`.
   - **Unsend (DM)** / **comment delete**: the stored copy is removed.
   - **Hide/unhide (comment):** `comment.hidden` flips.

---

## 5. Replaying a payload locally (no real account needed)

You can POST a sample payload straight at the dev endpoint to exercise ingestion
(set `WEBHOOK_STRICT_SIGNATURE` off, and use a `platformAccountId` that exists in
`socialAccountIndex`):

```bash
# IG comment (entry.changes[].field = comments)
curl -X POST "https://be.trendly.now/dev/webhooks/instagram" \
  -H "Content-Type: application/json" \
  -d '{"object":"instagram","entry":[{"id":"<IG_BUSINESS_ACCOUNT_ID>","time":1700000000,
       "changes":[{"field":"comments","value":{"id":"<COMMENT_ID>","text":"love this!",
       "from":{"id":"<COMMENTER_ID>","username":"tester"},"media":{"id":"<MEDIA_ID>"}}}]}]}'

# IG DM (entry.messaging[])
curl -X POST "https://be.trendly.now/dev/webhooks/instagram" \
  -H "Content-Type: application/json" \
  -d '{"object":"instagram","entry":[{"id":"<IG_BUSINESS_ACCOUNT_ID>","time":1700000000,
       "messaging":[{"sender":{"id":"<CONTACT_ID>"},"recipient":{"id":"<IG_BUSINESS_ACCOUNT_ID>"},
       "timestamp":1700000000000,"message":{"mid":"m_1","text":"hi there"}}]}]}'
```

Then check Firestore for the `cmt_<COMMENT_ID>` / `dmwh_..._<CONTACT_ID>` doc.

> **Note:** replays exercise ingestion + UI, but **reply** still calls the real
> Graph API, so a reply to a fake comment id will fail at the platform — use a
> real event to test the reply leg.

---

## 6. Sign-off checklist

- [ ] `GET` handshake green for IG + FB (token matches)
- [ ] Page(s) + linked IG subscribed (`subscribed_apps` confirms fields)
- [ ] `socialAccountIndex` entry present for each connected account
- [ ] Path 1 — IG message: received → ingested → shown → reply lands
- [ ] Path 2 — IG comment: received → ingested → shown → reply lands
- [ ] Path 3 — FB message: received → ingested → shown → reply lands
- [ ] Path 4 — FB comment: received → ingested → shown → reply lands
- [ ] Public reply ingestion appends to the parent thread
- [ ] DM attachment stored + previewed
- [ ] Unsend / delete / hide sync verified
- [ ] `WEBHOOK_STRICT_SIGNATURE=true` re-tested (signatures pass) before prod
