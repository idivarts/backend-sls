# Inbox Feature — Meta Permissions & App Review Walkthrough

End-to-end guide for the permissions Trendly must request, and the App Review
process it must pass, to **read and reply to DMs and comments** on connected
Instagram and Facebook accounts from inside the Trendly Inbox.

> **Scope (v1):** Meta only — Instagram (DMs + comments) and Facebook Pages
> (Messenger + Page comments). WhatsApp is deferred to v2; YouTube/LinkedIn/Twitter
> have no usable inbox API and are out of scope. See the channel reality table below.
>
> **This doc builds on** [`social-oauth-setup.md`](./social-oauth-setup.md), which
> covers base OAuth (login + read-only profile/insights). Here we add **only** the
> extra scopes, webhooks, and review steps the Inbox needs.

---

## 0. Channel reality table (why v1 is Meta-only)

| Channel | DMs | Comments | Permission(s) | App Review |
|---|---|---|---|---|
| **Instagram** | ✅ | ✅ | `instagram_manage_messages`, `instagram_manage_comments` | ✅ Required + screencast |
| **Facebook Page** | ✅ | ✅ | `pages_messaging`, `pages_manage_engagement`, `pages_read_engagement` | ✅ Required + screencast |
| WhatsApp | ⚠️ separate WABA | — | `whatsapp_business_messaging` | ✅ + phone/number onboarding → **v2** |
| YouTube | ❌ no DM API | ⚠️ comments only (YouTube Data API, separate Google OAuth) | — | Out of inbox v1 |
| LinkedIn | ❌ | ❌ | no usable messaging API | Cut |
| Twitter/X | ⚠️ paid API tier only | ❌ | — | Cut |

---

## 1. Permissions to request

### Already granted today (from `social-oauth-setup.md`, read-only)
- Instagram (via separate Instagram OAuth flow): `instagram_business_basic`, `instagram_business_manage_insights`, `instagram_business_manage_messages`, `instagram_business_manage_comments`, `instagram_business_content_publish`
- Facebook (via Facebook OAuth flow): `pages_show_list`, `pages_read_engagement`, `pages_messaging`, `pages_manage_engagement`, `pages_manage_metadata`, `pages_manage_posts`

> **Note:** Instagram permissions (`instagram_basic`, `instagram_manage_insights`,
> `instagram_manage_messages`, `instagram_manage_comments`) are **not** requested
> through the Facebook OAuth flow. Instagram has its own separate OAuth flow
> (`internal/trendlyapis/social_connect/instagram.go` or `unauth_apis/instagram.go`).
> Do **not** add Instagram scopes to the Facebook connect URL.

### NEW scopes the Inbox requires

| Scope | Channel | What it unlocks | Access level |
|---|---|---|---|
| `instagram_manage_messages` | IG | Read IG DMs on the connected IG Business account; send replies within Meta's messaging window | Advanced (review) |
| `instagram_manage_comments` | IG | Read comments on the account's posts/reels; reply to / hide / delete comments | Advanced (review) |
| `pages_messaging` | FB | Read & send Messenger messages for the connected Page | Advanced (review) |
| `pages_manage_engagement` | FB | Reply to / hide / delete comments on Page posts | Advanced (review) |
| `pages_read_engagement` | FB | Read Page posts & comments (already requested — confirm Advanced Access) | Advanced (review) |
| `pages_manage_metadata` | FB | Subscribe the Page to webhooks (required for real-time events) | Advanced (review) |
| `business_management` | both | Only if managing Pages/IG accounts owned by external businesses via Business Manager | Advanced (review) |

> **Note vs. the existing chatbot review** (`facebookReview.md`): that review framed
> the app as an **automated chatbot** on Trendly's *own* pages. The Inbox is a
> **custom inbox solution** operating on **users' connected** Pages/IG accounts.
> This is a different (broader) use case and **must be re-justified** in App Review —
> when asked "automated experience, custom inbox, or both?", answer **custom inbox
> solution** (and "both" if AI-assisted replies ship).

---

## 2. Messaging windows & policy constraints (design implications)

These are hard API limits — the UI must account for them, not work around them.

- **Instagram & Messenger 24-hour window**: you may send a reply only within **24
  hours** of the user's last message. Outside it, sends fail unless using an
  approved message tag / template. → *UI: disable the composer and show "You can
  only reply within 24h of the last message" when the window has closed.*
- **Comment replies** are **public** and have no time window (but are subject to
  the account's own moderation rules).
- **Rate limits**: per-Page and per-app call limits apply; batch/poll accordingly.
- **No proactive outreach**: you cannot DM a user who hasn't messaged the account
  first (no cold outreach) — comments can be replied to anytime.

---

## 3. Webhooks (real-time inbound)

The backend already subscribes Pages to `messages` and `message_echoes`
(`pkg/messenger/subscribe_app.go`). For the Inbox add comment events.

**Webhook fields to subscribe (per Page/IG account):**
- `messages` — inbound DMs (already on)
- `message_echoes` — outbound echoes / sent confirmations (already on)
- `messaging_postbacks` — button taps (optional)
- `feed` (Facebook Page) — new comments/posts on the Page
- `comments` / `mentions` (Instagram) — new comments & @-mentions

**Setup steps:**
1. In the Meta App dashboard → **Webhooks**, configure the callback URLs
   (handled by the `message_webhook` lambda):
   - Instagram: `https://be.trendly.now/webhooks/instagram` (`…/dev/…` for dev)
   - Facebook:  `https://be.trendly.now/webhooks/facebook`
   - Data deletion: `https://be.trendly.now/webhooks/data-deletion`
2. Set the **Verify Token** to match `WEBHOOK_VERIFY_TOKEN` (defaults to the
   historical value if unset). Used by `GET /webhooks/{instagram,facebook}`.
3. Subscribe the app to the fields above at the **app** level.
4. Connected Pages are auto-subscribed on connect (the FB connect callback calls
   `SubscribeApp()` per Page with the extended field list).
5. Signature validation (`X-Hub-Signature-256`) is implemented in the receiver;
   set `WEBHOOK_STRICT_SIGNATURE=true` to reject bad signatures (default logs +
   continues to avoid disrupting the existing chatbot during rollout).

---

## 4. End-to-end walkthrough (what to do, in order)

### A. Meta app prerequisites
1. Use the **prod** Meta app (Instagram + Facebook share one app — see setup doc).
2. Complete **Business Verification** (Meta Business Manager → Security Center).
   *Required before Advanced Access on any messaging scope.* This can take days —
   **start first**.
3. Confirm the app has a **Privacy Policy URL** and **Data Deletion** URL
   (Trendly website already serves `/privacy` and `/data-deletion`).

### B. Add scopes & products
4. App dashboard → add the new scopes (§1) under **App Review → Permissions and
   Features** (request **Advanced Access** for each).
5. Ensure both **Messenger** and **Instagram** products are added to the app, and
   the **Webhooks** product is configured (§3).

### C. Configure webhooks (§3) and verify they deliver to the dev app first.

### D. App Review submission (per scope)
For **each** requested scope, Meta requires:
6. **A detailed use-case description** — explain that Trendly is a **custom inbox**
   that lets a business owner read and reply to their own connected accounts' DMs
   and comments in one place.
7. **A screencast** showing the real flow end-to-end:
   - User connects their IG/FB account in Trendly (consent screen visible).
   - A test user sends a DM / leaves a comment.
   - The message/comment appears in the Trendly Inbox.
   - The Trendly user replies; the reply is shown landing back on IG/FB.
   - Show comment hide/delete if `*_manage_*` comment scopes are requested.
8. **Test credentials** — a working Trendly login + a connected test Page/IG
   account Meta's reviewer can use (add reviewers as test users / roles).
9. **Step-by-step reviewer instructions** mirroring the screencast.

### E. Go live
10. Once approved, switch the scopes to Advanced Access in **prod**.
11. Roll out the connect flow updating the requested scope list (see §5).

---

## 5. Code touchpoints (where scopes/webhooks are wired)

| Concern | File |
|---|---|
| OAuth scope list (Meta connect) | `internal/trendlyapis/social_connect/facebook.go`, `internal/trendlyapis/unauth_apis/instagram.go` |
| Long-lived token exchange | `pkg/messenger/token.go` |
| Page webhook subscription | `pkg/messenger/subscribe_app.go` (extend field list) |
| Conversations / messages fetch | `pkg/messenger/conversation.go`, `pkg/messenger/message.go` |
| Instagram profile/Graph calls | `pkg/instagram/instagram.go` |
| Webhook receiver | `functions/unauth_apis/main.go` (+ handler) |
| Stored tokens (`AccessToken`, `GraphType`) | `internal/models/trendlymodels/social.go` (`SocialsPrivate`) |

> **Standing rule:** any new Firestore collection for the inbox (e.g.
> `brands/{brandId}/inbox/...`) or any new multi-field query must update
> `firestore/trendly/firestore.rules` and `firestore.indexes.json` in the same change.

---

## 6. Pre-submission checklist

- [ ] Business Verification complete (Meta Business Manager)
- [ ] Privacy Policy + Data Deletion URLs live and set in app
- [ ] Advanced Access requested: `instagram_manage_messages`, `instagram_manage_comments`
- [ ] Advanced Access requested: `pages_messaging`, `pages_manage_engagement`, `pages_read_engagement`, `pages_manage_metadata`
- [ ] Webhooks configured (callback URL + verify token) for `messages`, `message_echoes`, `feed`, `comments`/`mentions`
- [ ] Webhook signature (`X-Hub-Signature-256`) validation implemented
- [ ] Screencast recorded for each scope (connect → receive → reply)
- [ ] Reviewer test credentials + connected test account provided
- [ ] Use case stated as **custom inbox solution** (re-justified vs. the older chatbot review)
- [ ] 24-hour messaging window handled in UI
