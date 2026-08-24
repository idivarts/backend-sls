# RevenueCat Native IAP Setup

Native In-App Purchase (Apple App Store + Google Play) for the Trendly brand app
runs through **RevenueCat**. RevenueCat validates the store receipts and posts
server-to-server webhook events to the backend, which maps them onto the SAME
org billing engine the web/Razorpay path uses (`trendlymodels.ApplyPlanToOrg`
for subscriptions, `AddTopup` for token packs).

Web keeps Razorpay (~3% fee); native uses IAP because Apple/Google mandate it
for in-app digital goods. Entitlements are **org-level** — a plan bought on any
surface unlocks the whole org everywhere.

---

## 1. Product identifiers (must match everywhere)

The backend maps store product ids → plan key / token grant in
`internal/trendlyapis/revenuecat/products.go`. Configure these **exact** ids in
App Store Connect, Google Play Console, **and** RevenueCat:

| Product id | Type | Maps to |
|---|---|---|
| `trendly_pro_monthly` | Auto-renewable subscription | plan `pro` |
| `trendly_team_monthly` | Auto-renewable subscription | plan `team` |
| `trendly_topup_1m` | Consumable | +1,000,000 wallet tokens |

RevenueCat **entitlements**: create `pro` and `team` (the webhook also falls back
to matching on entitlement id when the product id is unknown). Consumables have
**no** entitlement.

> Entitlements/token allotments are identical to the web plans (driven by
> `PlanLimitsMap`). Only the **price** differs on mobile — see §4.

## 2. App Store Connect (iOS)

- Create a subscription group; add `trendly_pro_monthly` + `trendly_team_monthly`
  auto-renewable subscriptions.
- Add the `trendly_topup_1m` consumable.
- Set **higher price points** (see §4). Disable Family Sharing.
- Add subscription localizations, terms of service + privacy policy links
  (required for review).
- Complete Paid Apps agreement + tax/banking.

## 3. Google Play Console (Android)

- Add matching subscriptions + the consumable in-app product with the same ids.
- Set pricing (§4).

## 4. Pricing (higher mobile prices)

Store commission is 15–30%. To net ≈ the web prices ($29 Pro / $79 Team), set
**higher** store price points, e.g. ~$34 / ~$94 (exact tier TBD) and price the
top-up above $10. Prices are set **store-side only** — the app renders
`product.priceString`, never a hardcoded number. Enrolling in Apple's Small
Business Program / Google's reduced tier (15% under ~$1M/yr) increases margin.

## 5. RevenueCat dashboard

- Create the iOS + Android apps; attach the App Store / Play credentials.
- Register products, attach them to the `pro` / `team` entitlements, build an
  **Offering** (the app renders packages from the current offering).
- **Webhook:** point it at `POST https://be.trendly.now/revenuecat/webhook`
  (dev: `.../dev/revenuecat/webhook`) and set the **Authorization header** value.
  That value must equal the backend env var **`REVENUECAT_WEBHOOK_AUTH`**.
- Configure **transfer behavior** for shared purchases (an Apple ID used across
  two orgs) — "transfer to the new App User ID". The backend revokes the org a
  subscription transfers away from (`TRANSFER` event).

## 6. Backend env

Set `REVENUECAT_WEBHOOK_AUTH` (GitHub Actions secret → injected in
`serverless.trendly.yml`) to the same secret configured on the RevenueCat
webhook. The webhook lambda is `trendly_revenuecat_webhook` → `ANY
/revenuecat/webhook`.

## 7. App User ID

The app calls `Purchases.logIn(organizationId)` so purchases attach to the org
(billing is org-level). The webhook reads `app_user_id` as the org id.

## 8. Events handled

| RevenueCat event | Effect |
|---|---|
| `INITIAL_PURCHASE`, `RENEWAL`, `PRODUCT_CHANGE`, `UNCANCELLATION` | `ApplyPlanToOrg` (refill wallet + entitlements), `AccessState=active`, `PeriodEnd=expiration` |
| `NON_RENEWING_PURCHASE` | `AddTopup` (token pack) |
| `BILLING_ISSUE` | `AccessState=past_due` |
| `EXPIRATION` | `AccessState=canceled` |
| `CANCELLATION` | no-op (access retained until expiry) |
| `TRANSFER` | revoke the org(s) it moved away from |

All events are idempotent — a retried delivery is deduped on the RevenueCat event
id via the `webhookEvents` ledger (important so top-ups are never double-credited).

## 9. Cron interaction

The 1st-of-month billing cron (`internal/trendlyapis/billing/cron.go`) **skips**
orgs with `Billing.Provider == "revenuecat"` — IAP renews on the store
anniversary via the `RENEWAL` webhook, not the 1st-of-month anchor, so the cron
must not refill them off-cycle.

## 10. Testing

- iOS: StoreKit sandbox / TestFlight. Android: Play internal testing.
  RevenueCat sandbox events fire real webhooks.
- Verify each path end-to-end: purchase → RC webhook → org doc updates → app UI
  unlocks. Cover renewal, billing-issue → past_due, expiration → locked, refund,
  top-up credit, and restore purchases.
