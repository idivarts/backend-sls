# Social OAuth Platform Setup Guide

This document covers every step needed to configure each social platform's developer console for Trendly's first-party OAuth integration. Follow these steps for **both** dev and prod environments.

---

## Redirect URIs at a glance

All OAuth callbacks hit the backend. The only difference between environments is the path prefix API Gateway adds for the `dev` stage.

| Environment | Backend base | Callback pattern |
|---|---|---|
| **Prod** | `https://be.trendly.now` | `https://be.trendly.now/connect/{platform}/callback` |
| **Dev** | `https://be.trendly.now/dev` | `https://be.trendly.now/dev/connect/{platform}/callback` |

Full list of redirect URIs to register (copy-paste into each console):

```
# Prod
https://be.trendly.now/connect/instagram/callback
https://be.trendly.now/connect/facebook/callback
https://be.trendly.now/connect/youtube/callback
https://be.trendly.now/connect/linkedin/callback
https://be.trendly.now/connect/twitter/callback

# Dev
https://be.trendly.now/dev/connect/instagram/callback
https://be.trendly.now/dev/connect/facebook/callback
https://be.trendly.now/dev/connect/youtube/callback
https://be.trendly.now/dev/connect/linkedin/callback
https://be.trendly.now/dev/connect/twitter/callback
```

---

## 1. Instagram

**Console:** https://developers.facebook.com

Instagram and Facebook share the same Meta app. A single Meta app covers both platforms.

### Steps

1. Go to **My Apps → Create App**.
2. Select **Business** as the use case → name it `Trendly` (or `Trendly Dev` for the dev app).
3. Add product: **Instagram** (under "Add Products to Your App").
4. Go to **Instagram → API setup with Instagram login**.
5. Under **Instagram App ID** — note the **App ID** and **App Secret**. These map to `INSTA_CLIENT_ID` and `INSTA_CLIENT_SECRET`.
6. Under **OAuth Redirect URIs**, add:
   ```
   https://be.trendly.now/connect/instagram/callback
   https://be.trendly.now/dev/connect/instagram/callback   ← add to dev app only
   ```
7. Under **Deauthorize Callback URL** — set to `https://be.trendly.now/connect/instagram/deauth` (informational, not wired yet).
8. Under **App Domains**, add:
   ```
   be.trendly.now
   connect.trendly.now
   dev.connect.trendly.now
   ```

### Required Scopes

```
instagram_business_basic
instagram_business_manage_insights
instagram_business_manage_messages
instagram_business_manage_comments
instagram_business_content_publish
```

These are requested in code (`social_connect/instagram.go`). No action needed in the console beyond granting them during App Review (required before going live).

`instagram_business_content_publish` is required for publishing photos, reels, and carousels to a connected Instagram Business account.

### App Review (Prod only)

Before prod goes live, submit for review. Required permissions:
- `instagram_business_basic`
- `instagram_business_manage_insights`
- `instagram_business_manage_messages`
- `instagram_business_manage_comments`
- `instagram_business_content_publish`

### Env vars to set

| Variable | Where to find it |
|---|---|
| `INSTA_CLIENT_ID` | App Dashboard → App ID |
| `INSTA_CLIENT_SECRET` | App Dashboard → App Secret |

---

## 2. Facebook

**Console:** https://developers.facebook.com

Uses the **same Meta app** as Instagram. No separate app needed.

### Steps

1. Open the same Meta app created in step 1 above.
2. Add product: **Facebook Login** (under "Add Products to Your App").
3. Go to **Facebook Login → Settings**.
4. Under **Valid OAuth Redirect URIs**, add:
   ```
   https://be.trendly.now/connect/facebook/callback
   https://be.trendly.now/dev/connect/facebook/callback   ← dev app only
   ```
5. Turn **on**: "Login with the JavaScript SDK" → **No** (we use server-side code exchange).
6. Turn **on**: "Enforce HTTPS" → Yes.
7. Under **App Domains** (App Settings → Basic), ensure these are listed:
   ```
   be.trendly.now
   connect.trendly.now
   dev.connect.trendly.now
   ```

### Required Scopes

```
pages_show_list
pages_read_engagement
pages_messaging
pages_manage_engagement
pages_manage_metadata
pages_manage_posts
```

These are requested in code (`social_connect/facebook.go`). Submit for App Review before prod.

`pages_manage_posts` is required for publishing photos and feed posts to a connected Page via `PublishPagePhoto` / `PublishPageFeed` in `pkg/facebook/publish.go`.

### Env vars to set

| Variable | Where to find it |
|---|---|
| `FB_CLIENT_ID` | App Dashboard → App ID (same as INSTA_CLIENT_ID if same app) |
| `FB_CLIENT_SECRET` | App Dashboard → App Secret |

> **Note:** `FB_CLIENT_ID` and `INSTA_CLIENT_ID` may be the same value if you're using a single Meta app for both platforms. The code has separate env vars so you can split them into two apps later if needed.

---

## 3. YouTube (Google)

**Console:** https://console.cloud.google.com

### Steps

1. Create a new project (or use an existing one): `Trendly` / `Trendly Dev`.
2. Go to **APIs & Services → Library** and enable:
   - **YouTube Data API v3**
   - **YouTube Analytics API**
3. Go to **APIs & Services → OAuth consent screen**:
   - User type: **External**
   - App name: `Trendly`
   - Authorized domains: `trendly.now`
   - Scopes to add (click "Add or Remove Scopes"):
     ```
     https://www.googleapis.com/auth/youtube.readonly
     https://www.googleapis.com/auth/yt-analytics.readonly
     https://www.googleapis.com/auth/userinfo.profile
     https://www.googleapis.com/auth/userinfo.email
     ```
   - For **prod**: publish the app (submit for verification if accessing sensitive scopes).
   - For **dev**: keep in **Testing** mode and add test user emails manually.
4. Go to **APIs & Services → Credentials → Create Credentials → OAuth 2.0 Client ID**:
   - Application type: **Web application**
   - Name: `Trendly Backend` (or `Trendly Backend Dev`)
   - **Authorized redirect URIs**:
     ```
     https://be.trendly.now/connect/youtube/callback
     https://be.trendly.now/dev/connect/youtube/callback   ← dev only
     ```
   - **Authorized JavaScript origins** — not needed (server-side flow).
5. Download the client JSON or note the **Client ID** and **Client Secret**.

### Env vars to set

| Variable | Where to find it |
|---|---|
| `YOUTUBE_CLIENT_ID` | Credentials → OAuth 2.0 Client → Client ID |
| `YOUTUBE_CLIENT_SECRET` | Credentials → OAuth 2.0 Client → Client Secret |

### Important notes

- The code requests `access_type=offline&prompt=consent` to force a refresh token on every connect. Without `prompt=consent`, Google only issues a refresh token on the *first* consent.
- Refresh tokens from Google **never expire** (unless the user revokes access or the app exceeds the token limit). The refresh Lambda skips YouTube tokens that have plenty of time left on their access token.

---

## 4. LinkedIn

**Console:** https://www.linkedin.com/developers/apps

### Steps

1. Click **Create App**.
   - App name: `Trendly` (or `Trendly Dev`)
   - LinkedIn Page: link your company page (required).
   - App logo: upload.
2. Go to **Auth** tab.
3. Under **OAuth 2.0 Settings → Authorized redirect URLs**, add:
   ```
   https://be.trendly.now/connect/linkedin/callback
   https://be.trendly.now/dev/connect/linkedin/callback   ← dev app only
   ```
4. Note the **Client ID** and **Client Secret** from the same Auth tab.
5. Go to **Products** tab and request access to:
   - **Sign In with LinkedIn using OpenID Connect** — grants `openid`, `profile`, `email` scopes.
   - *(Optional)* **Share on LinkedIn** — not needed for current implementation.
6. Under **Settings** tab, verify the app is **Active**.

### Required Scopes

```
openid
profile
email
```

These are available immediately after enabling "Sign In with LinkedIn using OpenID Connect". No additional review needed for these three.

### Env vars to set

| Variable | Where to find it |
|---|---|
| `LINKEDIN_CLIENT_ID` | Auth tab → Client ID |
| `LINKEDIN_CLIENT_SECRET` | Auth tab → Client Secret (click the eye icon) |

### Important notes

- LinkedIn access tokens last **60 days**. Refresh tokens are only issued if the `r_liteprofile` offline access scope is granted — currently the code requests it conditionally. If no refresh token is stored, the user will be prompted to reconnect when the token expires.
- LinkedIn enforces exact redirect URI matching — no wildcards, no trailing slashes.

---

## 5. Twitter / X

**Console:** https://developer.twitter.com/en/portal/projects-and-apps

### Steps

1. Apply for a **Developer Account** if you don't have one (requires a use-case description).
2. Create a **Project** → Create an **App** inside it.
   - Name: `Trendly` (or `Trendly Dev`)
3. Go to **App Settings → User authentication settings** → click **Set up**.
   - **OAuth 2.0**: Enable.
   - **Type of App**: Web App, Automated App, or Bot → choose **Web App**.
   - **App permissions**: Read (for profile data).
   - **Callback URI / Redirect URL** — add:
     ```
     https://be.trendly.now/connect/twitter/callback
     https://be.trendly.now/dev/connect/twitter/callback   ← dev app only
     ```
   - **Website URL**: `https://trendly.now`
4. Note the **Client ID** and **Client Secret** from the **Keys and Tokens** tab (under "OAuth 2.0 Client ID and Client Secret").

### Required Scopes

```
tweet.read
users.read
offline.access
```

`offline.access` is critical — without it Twitter won't issue a refresh token and the user must reconnect every 2 hours.

### Env vars to set

| Variable | Where to find it |
|---|---|
| `TWITTER_CLIENT_ID` | Keys and Tokens → OAuth 2.0 Client ID |
| `TWITTER_CLIENT_SECRET` | Keys and Tokens → OAuth 2.0 Client Secret |

### Important notes

- Twitter uses **PKCE (S256)** — the code verifier is generated per-request and embedded in the OAuth `state` parameter. No server-side session needed.
- Twitter **rotates refresh tokens** on every use. The refresh Lambda always stores the new refresh token after each refresh cycle.
- Access tokens expire in **2 hours**. The refresh Lambda catches them on its 6-hour sweep since `refreshWindowDays = 7` — tokens expiring in < 7 days are refreshed. Twitter tokens will always fall into this window after the first refresh cycle.

---

## Environment variable summary

Set these in AWS Secrets Manager or your CI/CD secret store, then reference them in `serverless.trendly.yml`.

| Variable | Platform | Dev value | Prod value |
|---|---|---|---|
| `INSTA_CLIENT_ID` | Instagram | Dev Meta App ID | Prod Meta App ID |
| `INSTA_CLIENT_SECRET` | Instagram | Dev Meta App Secret | Prod Meta App Secret |
| `FB_CLIENT_ID` | Facebook | Dev Meta App ID (may equal INSTA_CLIENT_ID) | Prod Meta App ID |
| `FB_CLIENT_SECRET` | Facebook | Dev Meta App Secret | Prod Meta App Secret |
| `YOUTUBE_CLIENT_ID` | YouTube | Dev Google OAuth Client ID | Prod Google OAuth Client ID |
| `YOUTUBE_CLIENT_SECRET` | YouTube | Dev Google OAuth Client Secret | Prod Google OAuth Client Secret |
| `LINKEDIN_CLIENT_ID` | LinkedIn | Dev LinkedIn Client ID | Prod LinkedIn Client ID |
| `LINKEDIN_CLIENT_SECRET` | LinkedIn | Dev LinkedIn Client Secret | Prod LinkedIn Client Secret |
| `TWITTER_CLIENT_ID` | Twitter | Dev Twitter OAuth 2.0 Client ID | Prod Twitter OAuth 2.0 Client ID |
| `TWITTER_CLIENT_SECRET` | Twitter | Dev Twitter OAuth 2.0 Client Secret | Prod Twitter OAuth 2.0 Client Secret |

---

## Dev vs Prod: separate apps or same app?

**Recommendation: separate apps per platform for Instagram/Facebook, shared for others.**

| Platform | Recommendation | Reason |
|---|---|---|
| Instagram / Facebook | **Separate apps** | Meta enforces strict App Review for production scopes. A separate dev app stays in Development mode indefinitely without review. |
| YouTube | **Separate OAuth clients** (same GCP project is fine) | Keep Testing mode for dev; publish for prod. |
| LinkedIn | **Separate apps** | LinkedIn doesn't have a staging mode — separate apps prevent dev redirect URIs from appearing in the prod app's audit log. |
| Twitter | **Separate apps** (inside the same Project) | Easier to manage callback URIs and avoids rate limit cross-contamination. |

---

## Checklist before going live (prod)

- [ ] Instagram: App Review approved for `instagram_business_basic`, `instagram_business_manage_insights`, `instagram_business_manage_messages`, `instagram_business_manage_comments`, `instagram_business_content_publish`
- [ ] Facebook: App Review approved for `pages_show_list`, `pages_read_engagement`, `pages_messaging`, `pages_manage_engagement`, `pages_manage_metadata`, `pages_manage_posts`
- [ ] YouTube: OAuth consent screen published and verified (if requesting sensitive scopes)
- [ ] LinkedIn: "Sign In with LinkedIn using OpenID Connect" product approved
- [ ] Twitter: App set to "Web App" type with `offline.access` scope enabled
- [ ] All prod redirect URIs registered (see table at top)
- [ ] All 10 env vars set in prod secrets
- [ ] `trendly-connect/serverless.yaml` `REPLACE_ME` values filled in (ACM cert ARN + Route53 hosted zone ID)
