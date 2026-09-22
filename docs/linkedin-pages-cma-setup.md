# LinkedIn Company/Showcase Pages — Dedicated CMA App Setup

> **Audience:** Rahul (admin). This is the manual LinkedIn-dashboard setup for the
> **`linkedin_page`** provider (Company/Showcase Page connect, posting, comments
> inbox, page insights). It is **separate from the personal LinkedIn app** and is
> the single biggest lead-time item in the whole social expansion — **start the
> CMA review first.** Created 2026-06-29. Supersedes §3 of
> `social-expansion-dashboard-setup.md` for everything org/page-related.

## 0. The hard rule (why a NEW app is required)
LinkedIn requires the **Community Management API (CMA) to be the ONLY product on
its app.** Our existing app **`249980194`** already has **Sign In with LinkedIn
(OIDC)** + Share-on-LinkedIn for the *personal* `linkedin` provider, so it **cannot**
also carry CMA. You must **create a brand-new, dedicated LinkedIn app** whose only
product is CMA. Mixing them is what the first implementation got wrong.

```
Personal profiles  →  app 249980194  →  provider "linkedin"       (unchanged)
Company Pages      →  NEW CMA app     →  provider "linkedin_page"  (this doc)
```

## 1. Create the dedicated CMA app
1. https://www.linkedin.com/developers → **Create app**. Name it clearly, e.g.
   "Trendly – Community Management (Pages)".
2. Associate it with a **Company Page** you control and **verify** the
   association (LinkedIn requires a verified page).
3. **Products:** request **Community Management API** and *nothing else* — do **not**
   add Sign In with LinkedIn (OIDC) or Share on LinkedIn on this app.
4. **Submit the CMA access review** (the form under Products). This is a **manual
   review (days–weeks)** and gates all end-to-end testing — do it first.

## 2. Auth tab
- **Authorized redirect URLs** — add both:
  - Prod: `https://be.trendly.now/connect/linkedin_page/callback`
  - Dev:  `https://be.trendly.now/dev/connect/linkedin_page/callback`
- Note the app's **Client ID** and **Client Secret** (distinct from the personal
  app's).
- Confirm **refresh tokens are enabled** for the app (so page tokens auto-renew).

## 3. Scopes the code requests
On the `linkedin_page` connect, the backend requests these CMA scopes:
```
r_basicprofile            # member identity (the connecting admin)
rw_organization_admin     # page + follower + share STATISTICS (Insights)
r_organization_social     # read org posts + comments (Media inbox)
w_organization_social     # comment/reply/react + create org posts (posting)
r_organization_followers  # follower statistics
```
The connecting LinkedIn member must be a **page ADMINISTRATOR**; otherwise org
calls return 403 and no pages are offered.
> ⚠️ LinkedIn is migrating some of these to `*_social_feed` variants. When CMA is
> granted, confirm the exact scope strings shown in the portal and, if they
> differ, update `pkg/linkedin/var.go` (Scope consts) + `social_connect/linkedin_page.go`.

## 4. LinkedIn-Version header
- The REST calls send a `LinkedIn-Version` month (YYYYMM). Default in code is
  `LINKEDIN_API_VERSION` (shared with the personal app, currently `202606`).
  Verify the CMA app supports the configured month; bump the env if LinkedIn has
  sunset it.

## 5. Env vars to set (GitHub Actions → CI → serverless)
Add these under the backend repo's **Settings → Secrets and variables → Actions**.
Client **ID** = a **Variable**; client **secret** = a **Secret** (mirrors the
existing `LINKEDIN_CLIENT_ID`/`LINKEDIN_CLIENT_SECRET` split). Single repo-level
values (shared dev+prod — no `_PROD`/`_DEV` suffix).

| Env var | GitHub Actions type | Value |
|---|---|---|
| `LINKEDIN_CM_CLIENT_ID` | **Variable** | the NEW CMA app's client id |
| `LINKEDIN_CM_CLIENT_SECRET` | **Secret** | the NEW CMA app's client secret |
| `LINKEDIN_API_VERSION` | **Variable** (optional) | override the `LinkedIn-Version` month |

The deploy workflow (`.github/workflows/deploy-trendly.yaml`) already wires them
into the `sls deploy` step:
```yaml
LINKEDIN_CM_CLIENT_ID: ${{ vars.LINKEDIN_CM_CLIENT_ID }}
LINKEDIN_CM_CLIENT_SECRET: ${{ secrets.LINKEDIN_CM_CLIENT_SECRET }}
```
and `serverless.trendly.yml` reads them via `${env:LINKEDIN_CM_CLIENT_ID, ''}`.
The personal app's `LINKEDIN_CLIENT_ID` / `LINKEDIN_CLIENT_SECRET` stay as they
are — do not point them at the CMA app.

## 6. Connect flow the user will see (built)
1. In the brand app → Connected Accounts → **"LinkedIn Page"** tile.
2. Consents on the **CMA app** (the admin's LinkedIn login).
3. The backend lists the Company/Showcase Pages that member administers and shows
   a **page picker** (multi-select) in the connect portal
   (`connect.trendly.now/connect/select-pages`).
4. Selected Pages each become a connected account (badged "Page"), all sharing one
   member token.

## 7. What works once CMA is approved
- **Posting** to a Company/Showcase Page (org feed), incl. scheduled.
- **Comments inbox** (Media tab) on the Page's posts: read + reply + delete.
- **Insights** (Analytics): follower stats (+ industry/seniority/geo
  demographics) and share statistics (impressions/reach/engagement).
- **Not possible (LinkedIn has no API):** Page **DMs/messaging**.

## 8. Lead-time / ordering
1. **Create CMA app + submit CMA review** (longest pole — do now).
2. Add redirect URLs + capture client id/secret while waiting.
3. Set the env vars in CI.
4. Once approved: connect a Page and run the QA in the correction plan.

Until CMA is approved, the "LinkedIn Page" tile will fail at consent
(`unauthorized_scope_error`) — expected; the personal "LinkedIn" tile is
unaffected.
