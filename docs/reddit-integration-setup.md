# Reddit Integration — Setup & Re-enable Guide

> **STATUS: PAUSED / GATED OFF (2026-06-29).** The Reddit integration (connect +
> posting + comments/DM inbox + derived insights) is **fully built end-to-end**
> but **disabled behind a feature flag** because setting up the Reddit app +
> commercial Data API access is involved and likely needs upfront funds. The
> Notion ticket is back in **To Do**. This doc is the single home for everything
> Reddit: the gating flags, the dashboard setup, and the re-enable steps.

---

## 1. Feature flags — how it's gated (currently all `false`)

Reddit is gated in **three places** that must all be flipped to `true` together
to re-enable it:

| Layer | File | Flag |
|---|---|---|
| Backend (Go) | `backend-sls/internal/constants/features.go` | `RedditEnabled` |
| Brand app | `trendly-brands/constants/features.ts` | `REDDIT_ENABLED` |
| Connect portal | `trendly-connect/lib/config.ts` | `REDDIT_ENABLED` |

**While `false`:**
- Backend `RedditInit` returns 404; Reddit is excluded from inbox DM/comment
  channels; publish + analytics dispatch treat it as unsupported; the daily
  snapshot skips it.
- Brand app hides the Reddit connect tile, and excludes Reddit from publishable
  destinations + inbox channels.
- Connect portal hides the Reddit picker tile.

So nothing Reddit-related is reachable. **To re-enable: complete §2–§4 below,
then flip all three flags to `true`.**

---

## 2. Two whole-integration gates (do FIRST — these are the real blockers)
1. **App pre-approval** — every Reddit app now needs approval under the
   **Responsible Builder Policy** (Nov 2025). Submit the app for review.
2. **Commercial use** — a brand scheduler is commercial → you must move off the
   free tier onto a **negotiated paid Data API agreement** (reported ~$12k/yr min
   + per-call fees). Contact Reddit for a commercial license. Go-live is blocked
   on this, not just scale. **This is the cost/fund concern that paused the work.**

## 3. Create the app
- Portal: https://www.reddit.com/prefs/apps (and the Reddit Developer Platform).
- Type: **web app** (confidential client).
- redirect uri: add **both** prod + dev callback URLs:
  - Prod: `https://be.trendly.now/connect/reddit/callback`
  - Dev:  `https://be.trendly.now/dev/connect/reddit/callback`
- Note the **client id** and **secret**.

## 4. OAuth specifics the code relies on
- Authorize host `https://www.reddit.com/api/v1/authorize`, token
  `https://www.reddit.com/api/v1/access_token`, API base `https://oauth.reddit.com`.
- `duration=permanent` (1-hour access tokens + a refresh token).
- **Mandatory unique `User-Agent`** on every request (format
  `web:trendly-ai-social-planner:v1.0 (by /u/<reddit-username>)`). Spoofing /
  generic UAs are bannable.
- Scopes the code requests: `identity submit read privatemessages edit history`.
- Free-tier rate limit: **100 QPM per OAuth client-ID over a 10-min window,
  shared across ALL tenants** (not per-user). The commercial agreement raises this.

## 5. Env vars + GitHub Actions
Add these under the backend repo's **Settings → Secrets and variables → Actions**
(client **ID** + User-Agent = **Variables**; client **secret** = **Secret**):

| Env var | GitHub Actions type |
|---|---|
| `REDDIT_CLIENT_ID` | **Variable** |
| `REDDIT_CLIENT_SECRET` | **Secret** |
| `REDDIT_USER_AGENT` | **Variable** |

They are already wired in `.github/workflows/deploy-trendly.yaml`:
```yaml
REDDIT_CLIENT_ID:     ${{ vars.REDDIT_CLIENT_ID }}
REDDIT_CLIENT_SECRET: ${{ secrets.REDDIT_CLIENT_SECRET }}
REDDIT_USER_AGENT:    ${{ vars.REDDIT_USER_AGENT }}
```
and read in `serverless.trendly.yml` via `${env:REDDIT_CLIENT_ID, ''}` etc.

## 6. Weak / not possible via API (set expectations)
- **Messaging:** legacy PMs are **read-only since Aug 2025**; Reddit Chat (the
  replacement) has **no public API**. The built inbox is read-only for PMs.
- **Insights:** **no organic analytics API** — only derived score/upvote_ratio/
  num_comments/karma. The built analytics show clearly-labeled derived metrics,
  not reach/impressions.

## 7. What's already built (re-enable just flips the flags)
- Backend: `pkg/reddit/*` (OAuth, profile, publish w/ image-asset upload,
  comments, messaging), `social_connect/reddit.go` (connect), `publish.go`
  (`case "reddit"`), inbox `platforms_dm.go`/`platforms_media.go` (Reddit
  branches), `analytics/reddit.go` (derived). All behind `RedditEnabled`.
- Brand app: Reddit provider tile, ScheduleBar subreddit/title/flair option block,
  inbox channel + analytics meta. All behind `REDDIT_ENABLED`.
- Connect portal: Reddit picker tile + icon. Behind `REDDIT_ENABLED`.

## 8. Re-enable checklist
1. Get Reddit app approved (Responsible Builder) + a commercial Data API
   agreement (§2).
2. Create the web app + redirect URLs (§3).
3. Set the three GitHub Actions vars/secrets (§5).
4. Flip the three flags (§1) to `true` and deploy backend + apps.
5. QA: connect a Reddit account → submit a post to a subreddit → load comments →
   reply → check the derived analytics card.
