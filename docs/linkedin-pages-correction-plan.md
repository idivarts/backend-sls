# LinkedIn Pages (CMA) — Correction & Build Plan

> **Why this exists.** The first social-expansion build wrongly folded LinkedIn
> **Company-Page** features (Community Management API: comments + insights) into
> the **existing personal LinkedIn app** (`249980194`, which has OIDC/Sign-In).
> LinkedIn requires **CMA to be the ONLY product on its app**, so that is
> impossible — and shipping it would have broken personal LinkedIn login
> (`unauthorized_scope_error`). This plan removes that mistake and rebuilds
> Company-Page support as a **distinct provider (`linkedin_page`)** on a
> **separate, dedicated CMA app**, then re-points the Media + Insights features
> onto it. Created 2026-06-29. Aligns with the existing Notion ticket
> "🏢 LinkedIn (Brands): connect Company/Showcase Pages + page posting (CMA)".

## Modeling decision (locked)
`linkedin` (personal profile) and `linkedin_page` (Company/Showcase Page) are
**two distinct platforms / source entries**, NOT one platform with an
`accountType` flag. Rationale: the whole codebase dispatches on `platform`
(`publish.go`, inbox `isCommentChannel`, analytics `fetchAccount`, frontend
`platformMeta`/`Socials`), so a separate platform slots cleanly into every switch
and keeps the two genuinely-different things visibly separate (the user's stated
intent). `accountType:"organization"` is still stored on the page account as
metadata (+ `tokenRef`, `vanityName`), per the original ticket.

| | `linkedin` (personal) | `linkedin_page` (company) |
|---|---|---|
| OAuth app | existing `249980194` (OIDC + Share-on-LinkedIn) | **new dedicated CMA app** |
| Scopes | `openid profile email w_member_social` | `r_basicprofile rw_organization_admin r_organization_social w_organization_social r_organization_followers` |
| Account | one member account | **one per administered Page** |
| Token | per-account | **shared member token doc**, pages reference via `tokenRef` |
| Posting | member feed (`w_member_social`) | org feed (`w_organization_social`) |
| Comments inbox | ❌ (no API) | ✅ |
| Insights | ❌ (no personal analytics API) → `supported:false` | ✅ org page/follower/share stats |
| Messaging | ❌ (no API) | ❌ (no API) |

## What gets removed (the "mess")
- `social_connect/linkedin.go`: drop the three org scopes from
  `linkedinScopesRequired`; drop the `GetAdministeredOrg` call + `orgUrn`/`orgName`
  capture into the personal account's `rawProfile`. Personal connect returns to
  OIDC + `w_member_social` only.
- `docs/social-expansion-dashboard-setup.md` §3: corrected to say "personal app =
  existing; Company Pages = a NEW dedicated CMA app" and point at the new CMA
  setup doc.
- Media + Insights dispatch: stop keying off `PlatformLinkedIn` + personal
  `rawProfile.orgUrn`; key off `PlatformLinkedInPage` (page account) instead.

## Token model — shared member token via `tokenRef`
A member may admin many Pages; all Pages connected in one OAuth carry the **same
member access token**. We store it **once** at `socialTokens/lipage_{memberId}`
and set each page account's `TokenRef` to that doc id. Reads go through a new
`GetBrandSocialTokenForAccount(brandID, acc)` that follows `TokenRef` when set
(else falls back to `acc.ID` — so every other platform is unchanged). This avoids
LinkedIn refresh-token rotation on one Page invalidating its siblings.

---

## Build waves

### LP1 — Revert (backend) + doc fix
- `social_connect/linkedin.go` → personal-only scopes; remove org-URN capture.
- Fix dashboard doc §3.

### LP2 — Model (`trendlymodels`)
- `social_v2.go`: `PlatformLinkedInPage = "linkedin_page"`; add `AccountType`,
  `TokenRef`, `VanityName` to `SocialAccount`.
- `GetBrandSocialTokenForAccount(brandID, *SocialAccount)` — follows `TokenRef`.
- `SaveBrandPageAccounts(brandID, accounts []SocialAccount, sharedToken, tokenDocID)`
  — one shared token doc + N page accounts in a batch.
- New `linkedin_page_session.go`: `LinkedInPageSession{ID, BrandID, App,
  CallbackScheme, UserID, MemberID, AccessToken, RefreshToken, TokenExpiry,
  Scopes, Orgs[], CreatedAt}` in `linkedinPageSessions/{id}` (server-only) with
  `Create/Get/Delete`; 10-min TTL enforced on read.

### LP3 — `pkg/linkedin`
- `var.go`: `CMClientID`/`CMClientSecret` (env `LINKEDIN_CM_CLIENT_ID/SECRET`);
  `ScopeOrgFollowers`. (Org scope consts already exist.)
- `token.go`: `ExchangeCodeCM(code, redirectURI)` + `RefreshAccessTokenCM` using
  the CM creds (factor a `exchangeWithClient(...)`).
- `org.go`: `ListAdministeredOrgs(token) []Organization` (paginate
  `organizationAcls`); keep `GetAdministeredOrg` (used nowhere personal now).
- `posts.go`: `CreateOrgPost(token, orgURN, text, imageURLs)` (author = org URN),
  reusing the existing post/image-upload logic.

### LP4 — Connect flow (`social_connect/linkedin_page.go`) + routes + env
- `LinkedInPageInit` → OAuth to CM app with org scopes.
- `LinkedInPageCallback` → `ExchangeCodeCM` → member profile (`sub`) →
  `ListAdministeredOrgs` → create a `LinkedInPageSession` → redirect to
  `{portal}/connect/select-pages?session={id}`.
- `LinkedInPageSession` (GET, public): returns `{orgs, app, callbackScheme}` for
  the picker (NO token leaked).
- `LinkedInPageSelect` (POST, public, guarded by the random session id): body
  `{session, orgIds[]}` → for each org create a page `SocialAccount`
  (`platform:linkedin_page`, `accountType:organization`, `platformAccountId:orgId`,
  `tokenRef:lipage_{memberId}`, org URN in rawProfile) via `SaveBrandPageAccounts`;
  delete the session; return `{ok, app, callbackScheme, count}`.
- Routes in `functions/unauth_apis/main.go`: init (token-guarded), callback +
  session + select (public). Env `LINKEDIN_CM_CLIENT_ID/SECRET` in
  `serverless.trendly.yml` **and** in `.github/workflows/deploy-trendly.yaml`
  (`vars.LINKEDIN_CM_CLIENT_ID` / `secrets.LINKEDIN_CM_CLIENT_SECRET`), plus the
  matching GitHub Actions Variable/Secret — see `docs/linkedin-pages-cma-setup.md` §5.

### LP5 — Re-point + token resolution
- `inbox/service.go` `isCommentChannel`: `linkedin` → `linkedin_page`.
- `inbox/platforms_media.go`: `PlatformLinkedIn` branch → `PlatformLinkedInPage`
  (org URN from `rawProfile.orgUrn` or `urn:li:organization:{platformAccountId}`).
- `analytics/cache.go` `fetchAccount`: `PlatformLinkedInPage → fetchLinkedIn`;
  `PlatformLinkedIn → Supported:false` (no personal analytics API).
- `analytics/linkedin.go`: org URN from the page account.
- `content_format.go`: add `PlatformLinkedInPage` to the same formats as
  `linkedin` (post/text/carousel/reel/video/live).
- `publishing/publish.go`: `publishToLinkedInPage` + `case "linkedin_page"`
  (org URN author, `CreateOrgPost`).
- Swap the account-aware token loads to `GetBrandSocialTokenForAccount`:
  `inbox/service.go` (loadServingAccount, SyncFromMeta), `inbox/media.go`
  (ListMedia), `analytics/cache.go` (fetchAccount), `publishing/publish.go`.
- `social_account_index`/cleanup: page disconnect should also clean the shared
  token doc only when it's the last page referencing it (best-effort; documented).

### LP6 — Brand app
- `platform.ts`: `LinkedInPage = "linkedin_page"`.
- `ISocialAccount`: add `linkedin_page` to the union + `accountType?`,
  `vanityName?`, `platformAccountId?`.
- `constants/Socials.ts`: keep "LinkedIn" (personal — "Personal profile & posting")
  and add "LinkedIn Page" (company — "Company Page: post, comments & insights",
  `faLinkedin`, `socialLinkedin`).
- `ScheduleBar.tsx`: add `linkedin_page` to `PUBLISHABLE` + dot color.
- Inbox `types.ts`: media `InboxChannel` uses `linkedin_page` (drop personal
  `linkedin`); update `use-inbox.api` INBOX_CHANNELS + `utils` icon/label.
- `analytics.tsx` `platformMeta`: add `linkedin_page` → "LinkedIn Page".
- `connected-accounts/index.tsx`: show a "Page" badge when
  `accountType === "organization"`; allow multiple LinkedIn accounts.

### LP7 — Connect portal (`trendly-connect`)
- `lib/platforms.ts`: add `linkedin_page` to `PlatformKey`, `PLATFORMS`,
  `PLATFORM_ORDER`.
- `components/PlatformIcon.tsx`: `linkedin_page` → LinkedIn glyph.
- New `app/connect/select-pages/page.tsx` (client): read `?session`, GET the
  session orgs, render a multi-select Page list, POST the chosen ids to
  `/connect/linkedin_page/select`, then deep-link back via the returned
  `app`/`callbackScheme` (success screen). Handle the empty (no admin pages) +
  error states.

### LP8 — Docs, build, Notion
- New `docs/linkedin-pages-cma-setup.md` (dedicated CMA app dashboard checklist).
- `go build ./...` + `go vet`; brand-app + portal typecheck.
- Update Notion: re-point the LinkedIn Media + Insights tickets' notes to
  `linkedin_page`; mark the "🏢 LinkedIn Pages connect + posting" ticket as
  implemented (Review/On Hold).

## Acceptance
1. Personal LinkedIn connect works with only OIDC + `w_member_social` (no org
   scopes; no regression).
2. A brand admin connects via the **new "LinkedIn Page"** tile → consents on the
   **CMA app** → picks one or more Pages → each appears in Connected Accounts as a
   Page (badge), sharing one token via `tokenRef`.
3. Page posting (org feed), comments inbox, and page insights all operate on the
   `linkedin_page` account; personal `linkedin` shows none of these.
4. `go build`/vet clean; brand-app + portal typecheck clean for touched files.

## Out of scope / follow-ups
- Showcase-page nuances beyond Company Pages (treated identically).
- Carousel/document org posts (text + single/multi image only, reusing existing).
- Shared-token refresh job wiring (the refresh Lambda) — coordinate-refresh per
  member; flagged for the token-refresh ticket.
- The picker assumes a member admins a manageable number of Pages (no paging UI).
