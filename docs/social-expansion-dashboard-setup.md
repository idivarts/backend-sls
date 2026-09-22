# Social Expansion — Developer Dashboard Setup Guide

> **Audience:** Rahul (admin). This is the **manual, out-of-band setup** you must
> complete on each platform's developer dashboard for the LinkedIn / Twitter(X) /
> YouTube / Reddit features to work in production. The code assumes all of this is
> done — it reads the resulting client IDs/secrets from env vars and requests the
> scopes listed below.
>
> Companion to: `docs/social-oauth-setup.md` (the existing FB/IG setup) and
> `docs/inbox-meta-permissions.md`. Created 2026-06-29 alongside the social
> expansion build.

Every platform follows the same shape we already use for FB/IG:
**redirect/callback URLs → OAuth app → scopes → app review/approval → set
`*_CLIENT_ID` / `*_CLIENT_SECRET` env (+ GitHub Actions vars).**

OAuth callback base (same as today):
- Prod: `https://be.trendly.now/connect/<platform>/callback`
- Dev: `https://be.trendly.now/dev/connect/<platform>/callback`
- The connect front-end portal is `https://connect.trendly.now` (prod) /
  `https://dev.connect.trendly.now` (dev).

Set secrets in **GitHub Actions org/repo variables** (mirroring
`FB_CLIENT_ID`/`FB_CLIENT_SECRET`) so CI injects them into
`serverless.trendly.yml` at deploy. The env var names the code reads are listed
per platform.

---

## 1. Twitter / X

**Portal:** https://developer.x.com (X Developer Portal) → Projects & Apps.

### 1.1 Pricing tier (DO THIS FIRST — it gates everything)
- New apps are on **pay-per-use**; legacy Free/Basic/Pro only persist for existing
  subscribers. The **Free tier (~17 writes/24h/app, ~100 reads/month) is not
  viable** for a multi-tenant scheduler.
- **Action:** choose a paid plan / top-up credits sufficient for expected post +
  read volume. Note the **$0.20 per post that contains a URL** (vs $0.015 plain) —
  this affects unit economics for a link-heavy scheduler. Budget reads for the
  DM inbox and replies/mentions inbox (reads dominate cost).

### 1.2 App config
- App type: **Web App, Automated App or Bot** (confidential client).
- **User authentication settings → set up:**
  - Type of App: **Confidential client**.
  - App permissions: **Read and write and Direct message**.
  - Callback URI / Redirect URL: add **both** prod and dev callback URLs above.
  - Website URL: `https://trendly.now`.
- OAuth 2.0 must be enabled (we use OAuth 2.0 Authorization Code w/ **PKCE**).

### 1.3 Scopes requested by the code
`tweet.read users.read offline.access tweet.write media.write dm.read dm.write`
(account-level OAuth2 user-context). Make sure the app permission level (Read +
write + DM) covers these or consent will fail.

### 1.4 Env vars
`TWITTER_CLIENT_ID`, `TWITTER_CLIENT_SECRET` (already wired in
`serverless.trendly.yml`). Confirm they point at the paid app.

### 1.5 Notes / limits to be aware of
- DM read polling is capped ~15 calls/15min **per account**; no free webhooks
  (we poll). Mentions/replies search (`/2/users/:id/mentions`,
  `search/recent`) ≈ 450/15min app.
- `non_public_metrics`/`organic_metrics` only for **own tweets < 30 days old** →
  our daily analytics snapshot captures them before they expire.
- v1.1 media upload is sunset; we use **v2 chunked media upload**.

---

## 2. YouTube (Google Cloud)

**Portal:** https://console.cloud.google.com → APIs & Services. Use the existing
Trendly Google Cloud project (the one already used for Firebase) or a dedicated
one — be consistent with whatever `YOUTUBE_CLIENT_ID` currently points at.

### 2.1 Enable APIs
- **YouTube Data API v3** (uploads, comments, channel info).
- **YouTube Analytics API** (`yt-analytics`).
- **YouTube Reporting API** (optional, bulk CSV — not required for v1).

### 2.2 OAuth consent screen
- User type: **External**, Publishing status: **In production** (not Testing) for
  real users.
- Add the scopes below as **sensitive/restricted** scopes; they require **Google
  OAuth verification** (brand verification + possibly a security assessment for
  restricted scopes). Start this early — it has the longest lead time.

### 2.3 OAuth client
- Create an **OAuth 2.0 Client ID → Web application**.
- Authorized redirect URIs: add **both** prod + dev callback URLs above.
- `access_type=offline` + `prompt=consent` are sent by the code to obtain refresh
  tokens (refresh token only issued on first consent).

### 2.4 Scopes requested by the code
- Read/analytics (already used today): `https://www.googleapis.com/auth/youtube.readonly`,
  `https://www.googleapis.com/auth/yt-analytics.readonly`,
  `https://www.googleapis.com/auth/userinfo.profile`,
  `https://www.googleapis.com/auth/userinfo.email`.
- **NEW for posting:** `https://www.googleapis.com/auth/youtube.upload` and
  `https://www.googleapis.com/auth/youtube.force-ssl` (needed for thumbnails +
  comment moderation if enabled).

### 2.5 Quota
- Default 10,000 units/day + **100 uploads/day** + per-method caps. `videos.insert`
  is ~1 unit now but the 100/day cap is the limiter; comment writes are 50 units.
- **Action (for scale):** request a quota increase via the **YouTube API
  Compliance Audit** (separate from OAuth verification).

### 2.6 Env vars
`YOUTUBE_CLIENT_ID`, `YOUTUBE_CLIENT_SECRET` (already wired). Confirm correct
project/client.

### 2.7 Not possible via API (do not expect)
- YouTube **Community posts** (text/image/poll) — no API. **DMs** — no API.

---

## 3. LinkedIn — TWO separate apps/providers

> ⚠️ **Important correction.** LinkedIn is split into **two distinct providers**
> on two **separate** OAuth apps, because LinkedIn requires the Community
> Management API (CMA) to be the **only** product on its app — it cannot share an
> app with Sign-In/OIDC.
>
> | Provider | What | App | Setup |
> |---|---|---|---|
> | `linkedin` | Personal profile connect + member posting | existing app `249980194` (OIDC + Share-on-LinkedIn) | §3 below |
> | `linkedin_page` | Company/Showcase Pages: posting + comments + insights | **NEW dedicated CMA app** | **see `docs/linkedin-pages-cma-setup.md`** |

### 3.1 Personal app (provider `linkedin`) — unchanged
- **Portal:** https://www.linkedin.com/developers → existing app `249980194`.
- Products: **Sign In with LinkedIn (OIDC)** + **Share on LinkedIn**.
- Redirect URLs: prod/dev `…/connect/linkedin/callback`.
- Scopes requested by the code: `openid profile email w_member_social` (personal
  posting only — **no org scopes here**).
- Env: `LINKEDIN_CLIENT_ID`, `LINKEDIN_CLIENT_SECRET` (already wired).

### 3.2 Company Pages (provider `linkedin_page`) — NEW dedicated CMA app
All org features (page posting, comments inbox, page insights) require a **brand
new, dedicated CMA app** with Community Management API as its only product, its
own client id/secret (`LINKEDIN_CM_CLIENT_ID` / `LINKEDIN_CM_CLIENT_SECRET`),
and a separate redirect (`…/connect/linkedin_page/callback`). The full checklist
— app creation, CMA review submission (longest lead time), scopes, env — is in
**`docs/linkedin-pages-cma-setup.md`**. Do that doc for anything Pages-related.

### 3.3 Not possible via API (do not expect)
- **Company-page DMs / personal-profile analytics** — no API. The comments inbox
  + page insights cover the org side; personal `linkedin` is posting-only.

---

## 4. Reddit — PAUSED / gated off → moved to its own doc

The Reddit integration is **built but disabled behind a feature flag** (setting
up the Reddit app + commercial Data API access is involved and likely needs
upfront funds). Everything Reddit — the gating flags, dashboard setup, env vars,
and the re-enable checklist — now lives in **`docs/reddit-integration-setup.md`**.
Nothing here for Reddit until that's revisited; the rows below are kept in §5 only
for reference (also gated).

---

## 5. GitHub Actions setup — Variables vs Secrets (REQUIRED)

The deploy workflow `.github/workflows/deploy-trendly.yaml` injects these into the
`sls deploy` step's `env:` block, and `serverless.trendly.yml` reads them via
`${env:NAME, ''}`. So you must add each to the **GitHub repository (or org)**
under **Settings → Secrets and variables → Actions**.

**Convention (mirror the existing FB/IG/Twitter ones):**
- Client **IDs** + the Reddit User-Agent → **Variables** (non-sensitive).
- Client **secrets** → **Secrets** (sensitive).
- These OAuth creds are **single repo-level values shared by dev + prod** — there
  is **no `_PROD`/`_DEV` suffix** for them (only `WEBSOCKET_*` and
  `FIRESTORE_DATABASE_*` are stage-split).

| Env var | GitHub Actions type | New? |
|---|---|---|
| `TWITTER_CLIENT_ID` | **Variable** | existing |
| `TWITTER_CLIENT_SECRET` | **Secret** | existing |
| `YOUTUBE_CLIENT_ID` | **Variable** | existing |
| `YOUTUBE_CLIENT_SECRET` | **Secret** | existing |
| `LINKEDIN_CLIENT_ID` | **Variable** | existing |
| `LINKEDIN_CLIENT_SECRET` | **Secret** | existing |
| `LINKEDIN_CM_CLIENT_ID` | **Variable** | **new** (dedicated CMA app) |
| `LINKEDIN_CM_CLIENT_SECRET` | **Secret** | **new** (dedicated CMA app) |
| `REDDIT_CLIENT_ID` | **Variable** | **new** — ⏸ gated, see `reddit-integration-setup.md` |
| `REDDIT_CLIENT_SECRET` | **Secret** | **new** — ⏸ gated |
| `REDDIT_USER_AGENT` | **Variable** | **new** — ⏸ gated |

The workflow lines that consume them (already added):
```yaml
LINKEDIN_CM_CLIENT_ID: ${{ vars.LINKEDIN_CM_CLIENT_ID }}
LINKEDIN_CM_CLIENT_SECRET: ${{ secrets.LINKEDIN_CM_CLIENT_SECRET }}
REDDIT_CLIENT_ID: ${{ vars.REDDIT_CLIENT_ID }}
REDDIT_CLIENT_SECRET: ${{ secrets.REDDIT_CLIENT_SECRET }}
REDDIT_USER_AGENT: ${{ vars.REDDIT_USER_AGENT }}
```
If a Variable/Secret is left unset, its env resolves to `''` and that platform's
connect fails gracefully at consent time (no deploy break).

## 6. Critical-path / lead-time ranking
1. **LinkedIn CMA Standard** (legal entity + screencast review) — longest.
2. **YouTube** OAuth verification + compliance audit (for quota).
3. **Reddit** — ⏸ **PAUSED / gated off** (Responsible-Builder approval + commercial
   agreement needed). See `reddit-integration-setup.md` to re-enable.
4. **Twitter/X** paid-tier selection (fast, but a spend decision).

The code ships behind these; until each approval lands, that platform's feature
will fail at the API with a clear error (and the frontend shows the connected
account but the capability returns empty / an upgrade-needed state).
