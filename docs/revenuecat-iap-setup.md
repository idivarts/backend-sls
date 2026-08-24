# RevenueCat Native IAP Setup — Dev → Prod Walkthrough

Native In-App Purchase (Apple App Store + Google Play) for the Trendly brand
app runs through **RevenueCat**. RevenueCat validates store receipts and posts
server-to-server webhook events to the backend, which maps them onto the SAME
org billing engine the web/Razorpay path uses (`trendlymodels.ApplyPlanToOrg`
for subscriptions, `AddTopup` for token packs).

Web keeps Razorpay (~3% fee); native uses IAP because Apple/Google mandate it
for in-app digital goods. Entitlements are **org-level** — a plan bought on any
surface unlocks the whole org everywhere.

This doc is written as a **walkthrough you follow in order**: one-time store
setup → dev environment (free, sandbox) → verify end-to-end → promote to prod.
It mirrors how the rest of the stack already separates dev/prod (own Firestore
DB, own websocket host, own GitHub Actions Environment) — IAP gets the same
treatment: **two RevenueCat projects**, one per environment.

---

## 0. The dev/prod architecture, at a glance

| | Dev | Prod |
|---|---|---|
| RevenueCat project | `Trendly Brands (Dev)` | `Trendly Brands (Prod)` |
| Backend webhook URL | `https://be.trendly.now/dev/revenuecat/webhook` | `https://be.trendly.now/revenuecat/webhook` |
| Backend `REVENUECAT_WEBHOOK_AUTH` | GitHub **`dev`** Environment secret | GitHub **`prod`** Environment secret |
| Client SDK key (iOS) | GitHub **`dev`** Environment var `REVENUECAT_IOS_KEY` | GitHub **`prod`** Environment var `REVENUECAT_IOS_KEY` |
| Client SDK key (Android) | GitHub **`dev`** Environment var `REVENUECAT_ANDROID_KEY` | GitHub **`prod`** Environment var `REVENUECAT_ANDROID_KEY` |
| Firestore data | Dev DB (`FIRESTORE_DATABASE_DEV_ID`) | Prod DB (`FIRESTORE_DATABASE_PROD_ID`) |
| Real money involved? | **Never** — Apple Sandbox / Play License Testing purchases are always free | Real purchases, real money |
| Built by | push a `dev-*` tag → `.github/workflows/ios.yaml` / `android.yaml` (TestFlight/Play **internal testing** track) | push to `master` → same workflows (App Store / Play **production** track) |

Why two RevenueCat **projects** instead of one: the app already provisions
separate `REVENUECAT_IOS_KEY` / `REVENUECAT_ANDROID_KEY` values per GitHub
Environment (`trendly-brands/.github/workflows/ios.yaml:64`,
`android.yaml:60`) — that only makes sense if dev and prod are backed by
distinct RevenueCat API keys. A RevenueCat **Project** is the container that
owns Apps (→ API keys), Products, Entitlements, Offerings, and Webhooks
together, so one project per environment gives you fully isolated webhook
secrets, fully isolated subscriber/event lists, and zero risk of a dev test
purchase touching prod data. Both projects attach to the **same** App Store
Connect app and **same** Google Play Console app (there is only one bundle id,
`pro.trendly.brands` — see `trendly-brands/app.json`) — that's normal; store
API credentials aren't exclusive to one RevenueCat project.

> **Important nuance:** the webhook handler
> (`internal/trendlyapis/revenuecat/webhook.go`) does **not** branch on
> RevenueCat's `environment` field (`SANDBOX` vs `PRODUCTION`) — it applies
> whatever event it receives to whichever org id (`app_user_id`) is in the
> payload, against whichever Firestore DB the running Lambda stage is wired to.
> The dev/prod split is achieved entirely by **pointing separate RevenueCat
> projects' webhooks at separate backend stages** — never point the Prod
> project's webhook at the dev URL or vice versa. As a safety net, org ids are
> environment-scoped (an org created via the dev backend only exists in the
> dev Firestore DB), so a misrouted event just no-ops with "org not found"
> rather than corrupting the other environment's data — but don't rely on that,
> wire it correctly.

---

## 1. One-time store setup (shared by both environments)

Store products are created **once** — the same product ids are used by both
the Dev and Prod RevenueCat projects; only which project's webhook receives
the resulting event differs.

### 1.1 Product identifiers (must match everywhere)

The backend maps store product ids → plan key / token grant in
`internal/trendlyapis/revenuecat/products.go`. Configure these **exact** ids in
App Store Connect, Google Play Console, **and** both RevenueCat projects:

| Product id | Type | Maps to |
|---|---|---|
| `trendly_pro_monthly` | Auto-renewable subscription | plan `pro` |
| `trendly_team_monthly` | Auto-renewable subscription | plan `team` |
| `trendly_topup_1m` | Consumable | +1,000,000 wallet tokens |

RevenueCat **entitlements**: create `pro` and `team` (the webhook also falls
back to matching on entitlement id when the product id is unknown).
Consumables have **no** entitlement.

> Entitlements/token allotments are identical to the web plans (driven by
> `PlanLimitsMap`). Only the **price** differs on mobile — see §1.4.

### 1.2 App Store Connect (iOS)

- App already exists: bundle id `pro.trendly.brands`.
- Create a subscription group; add `trendly_pro_monthly` +
  `trendly_team_monthly` auto-renewable subscriptions.
- Add the `trendly_topup_1m` consumable in-app purchase.
- Set an **App-Specific Shared Secret** (App Store Connect → your app → App
  Information → App-Specific Shared Secret) — RevenueCat's iOS integration
  needs it for receipt validation.
- Generate an **App Store Connect API Key** (Users and Access → Integrations →
  App Store Connect API → Team Keys), role "App Manager" or higher, download
  the `.p8` once — you'll enter the same Issuer ID / Key ID / `.p8` into
  **both** RevenueCat projects in §2.
- Add subscription localizations, terms of service + privacy policy links
  (required for review), disable Family Sharing.
- Complete the Paid Apps agreement + tax/banking (required before any
  purchase — sandbox or production — will resolve prices).
- Add **Sandbox Testers** now (Users and Access → Sandbox → Testers) — you'll
  need at least one for §3. Use a real, never-before-used email (a
  `+sandbox1@yourdomain.com` alias works); Apple ID rules for sandbox testers
  are looser than production but the email still can't already be an Apple ID.

### 1.3 Google Play Console (Android)

- Add matching subscriptions (`trendly_pro_monthly`, `trendly_team_monthly`)
  and the consumable in-app product (`trendly_topup_1m`) with the same ids.
- Upload at least one build to the **Internal testing** track (required before
  Play Billing will resolve *any* purchase, sandbox or production, for
  license testers — an app with zero uploaded builds can't process billing
  calls at all).
- Create a Google Cloud **service account** with Play Developer API access +
  "View financial data" permission (Play Console → Setup → API access) —
  you'll enter it into **both** RevenueCat projects in §2. (This can be the
  same or a different service account than `play-console-service-account.json`
  used for EAS submit — RevenueCat needs its own grant either way.)
- Add **License Testers** now (Play Console → Setup → License testing) — one
  or more Gmail accounts you control, needed for §3.

### 1.4 Pricing (higher mobile prices)

Store commission is 15–30%. To net ≈ the web prices ($29 Pro / $79 Team), set
**higher** store price points, e.g. ~$34 / ~$94 (exact tier TBD) and price the
top-up above $10. Prices are set **store-side only** — the app renders
`product.priceString`, never a hardcoded number. Enrolling in Apple's Small
Business Program / Google's reduced tier (15% under ~$1M/yr) increases margin.

---

## 2. RevenueCat: create the Dev project

Do this section for **Dev only** first — get one environment fully working
end-to-end before touching Prod at all.

1. **Create project** `Trendly Brands (Dev)` in the RevenueCat dashboard.
2. **Add apps**: one iOS app, one Android app, both bundle/package id
   `pro.trendly.brands`.
   - iOS app: paste the App Store Connect API Key (Issuer ID, Key ID, `.p8`
     from §1.2) and the App-Specific Shared Secret.
   - Android app: upload the Play service account JSON from §1.3.
   - Each app gives you a **public API key** — this is the value that goes
     into GitHub var `REVENUECAT_IOS_KEY` / `REVENUECAT_ANDROID_KEY` for the
     **`dev`** GitHub Environment (§4).
3. **Products**: add `trendly_pro_monthly`, `trendly_team_monthly`,
   `trendly_topup_1m` (RevenueCat will list them once it can see the store
   configuration from §1).
4. **Entitlements**: create `pro` and `team`; attach each subscription product
   to the matching entitlement. The consumable gets no entitlement.
5. **Offering**: build a "default" offering with packages for
   `trendly_pro_monthly`, `trendly_team_monthly`, `trendly_topup_1m` — this is
   what the app renders via `Purchases.getOfferings()`
   (`trendly-brands/utils/iap/purchases.native.ts`).
6. **Webhook** (Project Settings → Integrations → Webhooks):
   - URL: `https://be.trendly.now/dev/revenuecat/webhook`
   - Authorization header: generate a random secret (e.g.
     `openssl rand -hex 32`) — this exact value becomes the **`dev`** GitHub
     Environment secret `REVENUECAT_WEBHOOK_AUTH` (§4). Do not reuse the prod
     value.
   - Leave event types unfiltered (the handler ignores types it doesn't
     recognize — see `internal/trendlyapis/revenuecat/webhook.go`'s
     `process()` switch — so it's safe to send everything).
7. **Transfer behavior**: set to "transfer to the new App User ID" for shared
   purchases (an Apple ID used across two orgs). The backend revokes the org a
   subscription transfers away from (`TRANSFER` event).

Repeat step 6 with the dev-specific URL only for now — you'll create the Prod
project (identical steps, prod URL/secret/keys) in §6 once dev is verified.

---

## 3. Zero-cost sandbox testing — the actual purchase flow

Sandbox (iOS) and License Testing (Android) purchases **never charge real
money**, regardless of which backend they're wired to. This is the mechanism
that lets you test the whole flow — including renewals, cancellations, and
billing issues — for free.

### 3.1 Get a dev-wired build onto a device

Two options, both produce the same EAS `production` build profile (per
`trendly-brands/package.json`'s `build-ios`/`build-android` scripts) but baked
with dev env vars:

- **CI (recommended, closest to real release path):** push a git tag matching
  `dev-*` (e.g. `dev-iap-test-1`). This triggers
  `trendly-brands/.github/workflows/ios.yaml` and `android.yaml` with
  `environment: dev`, which bakes in `EXPO_PUBLIC_REVENUECAT_KEY` from the
  `dev` Environment's `REVENUECAT_IOS_KEY`/`REVENUECAT_ANDROID_KEY`, points the
  app at the dev backend/Firestore DB, and submits to **TestFlight internal
  testing** (iOS) / **Internal testing track** (Android).
- **Local EAS build:** `eas build --profile production --local` with
  `.env.local` set to the dev values, then sideload via TestFlight/internal
  track or `eas build --profile development` + a dev client for faster
  iteration (StoreKit/Play Billing still require a properly signed,
  store-provisioned build — a plain Metro/simulator debug build without the
  IAP entitlement won't resolve real sandbox purchases).

> **Fully offline alternative (UI-only, no webhook):** for quick iteration on
> the purchase *screen* without any store/network round-trip, Xcode's
> **StoreKit Configuration file** lets the iOS Simulator simulate purchases
> completely locally. It's free and instant, but because it never talks to
> Apple's servers, **no RevenueCat webhook fires** — it only proves the client
> UI works, not the backend billing path. Use §3.2 for anything that needs to
> reach the webhook.

### 3.2 iOS: Apple Sandbox

1. On the test device, do **not** sign into the Sandbox Apple ID via Settings
   on iOS 16+ (it's deprecated there) — instead just launch the app and start
   a purchase; iOS will prompt you to sign in with a **Sandbox Apple ID**
   in-line. Use the sandbox tester created in §1.2.
2. Complete the purchase — you'll see "[Environment: Sandbox]" in the payment
   sheet, and it will **not** charge the card on the Apple ID (sandbox
   purchases are simulated).
3. Sandbox subscriptions auto-renew on an accelerated clock (e.g. a monthly
   subscription can renew every few minutes) — this is useful for testing the
   `RENEWAL` event quickly without waiting 30 days.
4. To test cancellation/expiration: Settings → App Store → Sandbox Account →
   Manage → cancel the subscription, then wait for the accelerated expiry (or
   use §3.4's test-event shortcut instead of waiting).

### 3.3 Android: Play License Testing

1. Install the app via the **Internal testing** opt-in link (Play Console →
   Internal testing → testers tab has a shareable link) on a device signed in
   with a **License Tester** Gmail account from §1.3.
2. Start a purchase — Play Billing will show "This is a test order, you will
   not be charged" for license testers.
3. License-tester subscriptions also renew on an accelerated sandbox clock,
   same as iOS.

### 3.4 Fastest path for edge-case events: RevenueCat's test-event sender

Doing a real purchase, then a real cancellation, then waiting for real expiry
just to test the `EXPIRATION` handler is slow. RevenueCat's dashboard
(Project → Integrations → Webhooks → your webhook → **Send test event**) can
POST a synthetic event of a chosen type directly to your webhook URL with a
fake `app_user_id`. Use this to exercise `BILLING_ISSUE`, `EXPIRATION`,
`CANCELLATION`, and `TRANSFER` quickly. Two things to know before relying on
it:

- It uses a **fake app_user_id**, so `org.Get(orgID)` in
  `internal/trendlyapis/revenuecat/webhook.go` will fail to find a real org and
  the handler will ack-and-no-op. That's fine for confirming the endpoint is
  reachable and auth passes (check CloudWatch logs, see §3.5) — for a real
  state-change assertion, point it at a real dev-org id you've already funded
  via §3.2/§3.3's real purchase.
- Exact wording/location of this feature may differ slightly by RevenueCat
  dashboard version — look under the webhook integration's detail page if
  "Send test event" isn't where described above.

### 3.5 Verifying the backend actually received it

1. **CloudWatch**: find the `trendly_revenuecat_webhook` Lambda (dev stage) log
   group and look for `revenuecat webhook received <body>` — logged
   unconditionally at the top of `Handler()` before auth-dependent processing
   fails, so it also helps debug a 401 (auth secret mismatch between the
   RevenueCat dashboard's Authorization header and the `dev` GitHub
   Environment's `REVENUECAT_WEBHOOK_AUTH`).
2. **RevenueCat dashboard**: Customers tab shows the sandbox/test subscriber
   and its entitlement state.
3. **Firestore (dev DB)**: the org doc's `billing` map should reflect
   `provider: "revenuecat"`, `accessState`, `planKey`, `store`, `periodEnd`
   per `applyActiveSubscription()` in `webhook.go`.
4. **App UI**: reopen the brand app (dev build) — entitlements should reflect
   the new plan (paywall unlocked, wallet topped up, etc).

### 3.6 Full checklist before calling dev "done"

Cover every event type in the table below at least once against a real dev org
(not just the fake test-event sender):

| Event | How to trigger in dev | Expected result |
|---|---|---|
| `INITIAL_PURCHASE` | Real sandbox/license-test purchase | Org funded, `accessState=active` |
| `RENEWAL` | Wait for accelerated sandbox renewal | Wallet refilled, `periodEnd` advances |
| `NON_RENEWING_PURCHASE` (topup) | Buy `trendly_topup_1m` | `AddTopup` credits +1,000,000 tokens |
| `BILLING_ISSUE` | RevenueCat test-event, or a sandbox card-decline simulation | `accessState=past_due` |
| `EXPIRATION` | Cancel + wait for accelerated expiry, or test-event | `accessState=canceled` |
| `CANCELLATION` | Cancel via Settings/Play Console | No state change (access retained until expiry) — confirm nothing broke |
| `TRANSFER` | Purchase on the same sandbox Apple ID logged into a second dev org | Old org revoked |
| Duplicate delivery | Re-send the same event from RevenueCat's dashboard | Second delivery is a no-op (`webhookEvents` ledger dedupes by RevenueCat event id) |

---

## 4. Wire the Dev secrets into GitHub

Both repos already bind their deploy jobs to a GitHub Actions **Environment**
named `dev` or `prod` based on branch/tag
(`backend-sls/.github/workflows/deploy-trendly.yaml`,
`trendly-brands/.github/workflows/ios.yaml`,
`trendly-brands/.github/workflows/android.yaml`). Add the IAP values as
Environment-scoped secrets/vars so `secrets.*`/`vars.*` resolve correctly per
environment — do **not** add these as repo-level (ungated) secrets/vars, or
dev and prod will collide.

In **`backend-sls`** GitHub repo → Settings → Environments → **`dev`**:

| Name | Kind | Value |
|---|---|---|
| `REVENUECAT_WEBHOOK_AUTH` | Environment secret | The Authorization header value you set on the **Dev** RevenueCat project's webhook (§2 step 6) |

In **`trendly-brands`** GitHub repo → Settings → Environments → **`dev`**:

| Name | Kind | Value |
|---|---|---|
| `REVENUECAT_IOS_KEY` | Environment variable | Dev project's iOS app public API key (§2 step 2) |
| `REVENUECAT_ANDROID_KEY` | Environment variable | Dev project's Android app public API key (§2 step 2) |

These are already read by the workflows —
`backend-sls/.github/workflows/deploy-trendly.yaml` (`REVENUECAT_WEBHOOK_AUTH:
${{ secrets.REVENUECAT_WEBHOOK_AUTH }}`), `trendly-brands/ios.yaml:64` /
`android.yaml:60` (`EXPO_PUBLIC_REVENUECAT_KEY: ${{ vars.REVENUECAT_IOS_KEY /
REVENUECAT_ANDROID_KEY }}`) — you only need to populate the values, not touch
the workflow YAML.

Push a `dev-*` tag (or merge to `dev` for the backend) and re-run §3 for real
this time against the deployed dev stage.

---

## 5. App User ID (both environments)

The app calls `Purchases.logIn(organizationId)` so purchases attach to the org
(billing is org-level, not per-manager). The webhook reads `app_user_id` as the
org id. This behavior is identical in dev and prod — only the RevenueCat
project + backend stage differ.

---

## 6. Once dev is fully verified: set up Prod

Repeat §2 verbatim, with these substitutions:

- Project name: `Trendly Brands (Prod)`.
- Webhook URL: `https://be.trendly.now/revenuecat/webhook` (no `/dev` prefix).
- Webhook Authorization header: a **new**, different random secret — never
  reuse the dev value.
- Products/Entitlements/Offering: same ids as dev (`trendly_pro_monthly`,
  `trendly_team_monthly`, `trendly_topup_1m`, entitlements `pro`/`team`) —
  they're the same store products, just re-declared inside the Prod project.

Then in GitHub:

- `backend-sls` → Settings → Environments → **`prod`** → add
  `REVENUECAT_WEBHOOK_AUTH` (Environment secret) = the Prod project's webhook
  secret.
- `trendly-brands` → Settings → Environments → **`prod`** → add
  `REVENUECAT_IOS_KEY` / `REVENUECAT_ANDROID_KEY` (Environment variables) =
  the Prod project's iOS/Android public API keys.

### Pre-launch checklist

- [ ] Dev checklist in §3.6 fully passed on both iOS and Android.
- [ ] App Store Connect: subscriptions submitted for review (Apple reviews IAP
      alongside the first binary that offers them — plan the release
      accordingly).
- [ ] Play Console: subscriptions active, app promoted at least to a closed
      track before production (Play requires this progression).
- [ ] Prod RevenueCat project's webhook reachable — do a manual "Send test
      event" (§3.4) against prod immediately after the backend prod deploy and
      confirm a 200 + CloudWatch log entry, **before** any real user can
      purchase.
- [ ] Merge to `master` → triggers backend prod deploy + app prod build/submit
      (`ios.yaml`/`android.yaml` `environment: prod` path).
- [ ] After the first real production purchase, verify in the Prod RevenueCat
      dashboard + prod Firestore DB + CloudWatch, same as §3.5 but against
      prod resources.

---

## 7. Event reference

| RevenueCat event | Effect |
|---|---|
| `INITIAL_PURCHASE`, `RENEWAL`, `PRODUCT_CHANGE`, `UNCANCELLATION` | `ApplyPlanToOrg` (refill wallet + entitlements), `AccessState=active`, `PeriodEnd=expiration` |
| `NON_RENEWING_PURCHASE` | `AddTopup` (token pack) |
| `BILLING_ISSUE` | `AccessState=past_due` |
| `EXPIRATION` | `AccessState=canceled` |
| `CANCELLATION` | no-op (access retained until expiry) |
| `TRANSFER` | revoke the org(s) it moved away from |

All events are idempotent — a retried delivery is deduped on the RevenueCat
event id via the `webhookEvents` ledger (important so top-ups are never
double-credited).

## 8. Cron interaction

The 1st-of-month billing cron (`internal/trendlyapis/billing/cron.go`) **skips**
orgs with `Billing.Provider == "revenuecat"` — IAP renews on the store
anniversary via the `RENEWAL` webhook, not the 1st-of-month anchor, so the cron
must not refill them off-cycle. This applies identically in dev and prod.

## 9. Secrets & vars quick reference

| Name | Scope | Dev value | Prod value | Consumed by |
|---|---|---|---|---|
| `REVENUECAT_WEBHOOK_AUTH` | `backend-sls` Environment secret | Dev RC project's webhook secret | Prod RC project's webhook secret | `serverless.trendly.yml:46` → `internal/trendlyapis/revenuecat/webhook.go` |
| `REVENUECAT_IOS_KEY` | `trendly-brands` Environment var | Dev RC project's iOS API key | Prod RC project's iOS API key | `ios.yaml:64` → `EXPO_PUBLIC_REVENUECAT_KEY` → `utils/iap/purchases.native.ts` |
| `REVENUECAT_ANDROID_KEY` | `trendly-brands` Environment var | Dev RC project's Android API key | Prod RC project's Android API key | `android.yaml:60` → `EXPO_PUBLIC_REVENUECAT_KEY` → `utils/iap/purchases.native.ts` |
