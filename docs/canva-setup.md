# Canva Connect setup — deep-edit bridge

Backend for Module 3 of the AI-Native Studio ticket. Token exchange is
**backend-only** (client secret + Basic auth); the RN app never sees Canva
credentials.

## 1. Create the Canva integration

1. Go to the **Canva Developer Portal** → <https://www.canva.com/developers/>
   → *Your integrations* → **Create an integration** (type: **Public** — required
   for other users to connect; a Private integration only works for your own team).
2. Note the **Client ID** and generate a **Client secret**.
3. **Scopes** — enable:
   `asset:read asset:write design:content:read design:content:write design:meta:read profile:read`.
4. **Redirect URL** — add exactly:
   `https://be.trendly.now/api/integrations/canva/callback`
   (and the dev equivalent `https://be.trendly.now/dev/api/integrations/canva/callback`
   if you run a dev stage). It must match `CANVA_REDIRECT_URI`.
5. **Return navigation** — configure the return URL so "Return to Trendly" from
   the Canva editor comes back to our app with a `correlation_jwt`.

## 2. Credentials → env

| Name | Type | Where |
|---|---|---|
| `CANVA_CLIENT_ID` | var | GitHub Actions **variable** |
| `CANVA_CLIENT_SECRET` | secret | GitHub Actions **secret** |
| `CANVA_REDIRECT_URI` | var | GitHub Actions **variable** (defaults to the prod callback) |

The deploy workflow injects these; serverless reads them into `trendly_studio_apis`.

## 3. Public-integration review (the only external gate — START EARLY)

A **Public** integration must pass Canva's review before non-team users can
connect. Budget time for this; there is no published SLA.

Requirements:
- Secure hosting (HTTPS — we're on `be.trendly.now`).
- OAuth 2.0 + **PKCE (S256)** — implemented in `pkg/canva/oauth.go`.
- Brand/UI compliance ("Design in Canva" / "Import from Canva" buttons per
  Canva's brand guidelines).
- A demo video of the connect → edit → return → export round-trip.
- A completed review questionnaire.

Until approved, test with your own Canva team account (works immediately).

## 4. What the backend exposes (already implemented)

Lambda `trendly_studio_apis`, routes under `/api/integrations/canva`:

| Method | Path | Notes |
|---|---|---|
| `GET`  | `/connect?brandId=` | Gate `CanvaBridge` entitlement (Pro+), returns `authUrl` |
| `GET`  | `/callback` | **Public** — code→token, stores connection, redirects to app |
| `GET`  | `/status` | `{connected}` for the current brand-member |
| `DELETE` | `/connect` | Disconnect |
| `GET`  | `/designs?query=` | List library designs (import picker + Claude Design) |
| `POST` | `/brands/:brandId/contents/:contentId/design` | Seed a design from our render, returns `editUrl` |
| `POST` | `/brands/:brandId/contents/:contentId/export` | Start an export job |
| `GET`  | `/exports/:jobId` | Poll an export job |
| `POST` | `/brands/:brandId/contents/:contentId/return` | Verify `correlation_jwt`, export, copy asset back, attach |

- `pkg/canva` — client: PKCE, token exchange/refresh (single-use rotation),
  URL asset upload, create/list/get design, export job, **design import (PPTX)**,
  and JWKS-based return-JWT verification.
- Tokens live in `canvaConnections/{managerId}` (Firestore, **fully backend-only**
  — rules deny all client access).

## 5. Editable-layer handoff (PPTX bridge)

Seeding a design with `asset_id` embeds a **flat** image. To hand off **editable
layers**, we serialize the scene → PPTX and use Canva **Design Import** (text
stays text, shapes stay separate). `pkg/canva.ImportDesign` implements the import
call; the scene→PPTX serializer is a follow-up (`pkg/scenegraph` → PPTX). Flat
PNG export is the trivial fallback.

## 6. Explicitly skipped

**Autofill + Brand Templates** — both require us *and every user* to be on Canva
Enterprise. Wrong fit for our ICP; not implemented.

## Checklist for you

- [ ] Public integration created; Client ID + secret captured.
- [ ] Scopes + redirect URL + return navigation configured.
- [ ] `CANVA_CLIENT_ID` (var), `CANVA_CLIENT_SECRET` (secret), `CANVA_REDIRECT_URI`
      (var) set in GitHub Actions.
- [ ] Public-integration review submitted (start early).
- [ ] Verify field names in `pkg/canva/designs.go` against the current Connect
      REST reference before go-live (a few response shapes are best-effort).
