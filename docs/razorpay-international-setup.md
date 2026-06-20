# Razorpay International — Setup & Configuration Reference

> **Status:** §7b deliverable of the Trendly **Credit System** ticket.
> **Audience:** founder (dashboard/account owner) + backend engineer.
> **Scope:** every Razorpay dashboard and account setting required to make
> **USD, org-level subscriptions**, **recurring (e-mandate) billing**, and an
> **invoice / payment-link fallback** work end-to-end.

This document is the single source of truth for the manual Razorpay
configuration. Code references below point at the real files that read these
settings so the dashboard config and the backend stay in sync.

---

## 1. Overview & Context

We are moving billing from the old model (**per-brand, INR**) to the new model
(**per-organization, USD**) on **Razorpay International**.

| | Old (current code) | New (target) |
|---|---|---|
| Billing entity | Brand | **Organization** |
| Currency | INR (hardcoded) | **USD** |
| Plans live | per brand | per org (one shared wallet) |
| Webhook routing key | `notes.brandId` | **`notes.organizationId`** (falls back to `brandId` → brand's org for legacy subs) |

### Billing entity = Organization

The billing entity is the **Organization**, not the Brand. Razorpay webhooks
resolve the target org via `notes.organizationId`:

- `internal/trendlyapis/razorpay/payment_webhooks/billing_target.go` →
  `resolveBillingTarget(...)` reads `notes.OrganizationID`. If absent (legacy
  subscriptions) it falls back to `notes.BrandID`, loads the brand, and reads
  `brand.OrganizationID`. If neither resolves, it returns `no-billing-target`
  and the webhook is rejected.
- The notes contract struct lives in `pkg/payments/webhook/subscription.go`
  (`SubscriptionNotes{ BrandID, OrganizationID, PlanKey, ... }`).
- `internal/trendlyapis/razorpay/apis/subscriptionv2.go` →
  `CreateSubscriptionV2(...)` stamps `notes.organizationId` whenever the brand
  has one (`brand.OrganizationID`).

### Tokens (AI wallet)

Each plan refills a **token / credit wallet** at the org level on each
successful charge. The wallet is shared across all brands in the org (single
shared wallet — see the Credit/Subscription revamp note). Free gets a baseline
allotment; paid plans refill more per billing cycle.

### Plans

| planKey | Price (USD) | Interval | Razorpay object |
|---|---|---|---|
| `free` | $0 | — | **No Razorpay plan** (app-side default) |
| `pro` | **$29 / mo** | monthly | USD Subscription Plan |
| `team` | **$79 / mo** | monthly | USD Subscription Plan |
| `agency` | **custom / quote** | custom | Payment Link / manual invoice (no fixed plan) |

---

## 2. Account / Activation (founder only)

These are one-time account-level steps in the Razorpay dashboard. Without
International enabled, USD plans and USD payment links cannot be created.

- [ ] **Enable International Payments** — Dashboard → *Settings → Configuration →
      International Payments*. Requires Razorpay approval.
- [ ] **Complete / refresh KYC** for IDIVARTS Solutions Pvt Ltd (business PAN,
      GST, bank account, signatory KYC).
- [ ] **Business category** set correctly (SaaS / software) — Razorpay reviews
      category for international eligibility.
- [ ] **Website / app review** — Razorpay reviews the public site
      (`https://trendly.now` / brand app) for International approval: pricing
      page, terms, refund/cancellation policy, and contact info must be live.
- [ ] **Confirm USD as a presentment currency** with **INR settlement**.
      - Customers are charged in **USD**; funds settle to the Indian bank
        account in **INR**.
      - **FX margin** applies on conversion (currently ~2–3% — confirm the live
        rate card in the dashboard).
      - **Settlement is ~T+24h** (slower than domestic) — note this for cash-flow
        planning.
- [ ] **eFIRC / export-invoice setup** — international receipts are **export of
      services** under RBI rules. Configure:
      - eFIRC (electronic Foreign Inward Remittance Certificate) generation per
        international transaction (via Razorpay or the settlement bank).
      - Export invoice fields (LUT/GST export status) so collections are
        compliant for RBI export-of-services reporting.

> ⚠️ International activation can take several business days and may require
> back-and-forth with Razorpay support. Start this **first** — everything else
> is blocked on it.

---

## 3. Products / Plans

Create **USD Subscription Plans** for the two fixed paid tiers. Free and Agency
do **not** get a fixed plan.

### Create in dashboard

Dashboard → *Subscriptions → Plans → Create Plan*:

- [ ] **Pro** — Amount `$29.00` USD, Billing cycle **Monthly**, interval `1`.
- [ ] **Team** — Amount `$79.00` USD, Billing cycle **Monthly**, interval `1`.
- [ ] On each plan, set descriptive **notes** (e.g. `planKey: pro`, `planKey: team`)
      so they're identifiable in the dashboard.

> If yearly tiers are added later, create separate plans (the code already keys
> by `planKey:planCycle`, see below).

### Where the `plan_id` maps in code

`CreateSubscriptionV2` resolves the Razorpay plan id via the `Plans` map:

```go
// internal/trendlyapis/razorpay/apis/subscriptionv2.go
planId := payments.Plans[planKey+":"+planCycle]   // e.g. "pro:monthly"
```

The `Plans` map is loaded from `key-secrets.json` at init:

```go
// pkg/payments/init.go
type RazorpaySecrets struct {
    APIKey     string            `json:"key"`
    APISecret  string            `json:"secret"`
    WebhookKey string            `json:"webhookKey"`
    Plans      map[string]string `json:"plans"`   // <-- planKey:cycle -> Razorpay plan_id
}
```

So in **`backend-sls/key-secrets.json`**, the `razorpay.plans` object maps the
composite key `"<planKey>:<planCycle>"` → Razorpay `plan_id`:

```jsonc
{
  "razorpay": {
    "key": "rzp_live_xxxxxxxxxxxxx",
    "secret": "xxxxxxxxxxxxxxxxxxxxxxxx",
    "webhookKey": "whsec_xxxxxxxxxxxxx",
    "plans": {
      "pro:monthly":  "plan_PROxxxxxxxxxx",
      "team:monthly": "plan_TEAMxxxxxxxxx"
      // "pro:yearly":  "plan_...",   // add if/when yearly plans exist
      // "team:yearly": "plan_..."
    }
  }
}
```

### planKey → Razorpay plan_id mapping (fill in after creating plans)

| Composite key (code) | Tier | Currency | Razorpay `plan_id` (placeholder) |
|---|---|---|---|
| `pro:monthly` | Pro | USD | `plan_PROxxxxxxxxxx` |
| `team:monthly` | Team | USD | `plan_TEAMxxxxxxxxx` |
| `free` | Free | — | *(no plan — app default, never sent to Razorpay)* |
| `agency` | Agency | USD | *(no plan — custom Payment Link / quote)* |

### Free and Agency handling

- **Free** — never creates a Razorpay subscription. The org defaults to the free
  tier and its baseline token allotment app-side; `CreateSubscriptionV2`
  rejects an unknown/empty `planId` with `invalid-plan`.
- **Agency** — handled out-of-band with a **custom Payment Link or quote**
  (`payments.CreatePaymentLink` / `CreateSubscriptionLink` with a bespoke
  amount), not a standard plan. Sales agrees the amount; finance issues the link.

---

## 4. Subscriptions / Recurring (e-mandate)

Recurring charging uses Razorpay **Subscriptions**. Created in code via:

```go
// pkg/payments/subscription.go
CreateSubscriptionLink(planId, totalBillingCycles, trialDays, expireDays, notes, offerId)
// -> Client.Subscription.Create(...) ; returns (subscriptionId, shortUrl)
```

### Dashboard config

- [ ] **Enable the Subscriptions product** (Dashboard → *Subscriptions*).
- [ ] Enable **international cards** on subscriptions (depends on §2 activation).
- [ ] Set **customer notifications** (`customer_notify: true` is already passed in
      code) so Razorpay emails the authorization + receipts.

### Recurring constraints to be aware of

- **RBI e-mandate / recurring-card rules**: recurring charges on cards have a
  per-transaction cap and require an **AUTH transaction** to register the mandate.
  - The **first charge** authorizes the mandate (may be a small auth or the first
    full charge depending on plan config).
  - Subsequent monthly charges are auto-debited against the mandate.
- **International card recurring**: not all issuing banks honor recurring
  e-mandates; some charges will fail at the network level. This is the main
  reason the **invoice/payment-link fallback (§5)** exists.
- **Max-amount**: the mandate carries a max debit amount. Ensure it is ≥ the plan
  price (and any future price increase) or the upgrade charge will be declined.

### Razorpay-side retry/dunning vs our cron

| Concern | Owner |
|---|---|
| Network-level retry of a failed auto-debit | **Razorpay** (built-in subscription retry/dunning) |
| Moving the org to `past_due` / locking access | **Our webhook handler** (on `subscription.pending` / `subscription.halted`) |
| Re-attempting billing on the 1st of the month for orgs not on a live mandate | **Our cron** (1st-of-month billing job) — drives the invoice fallback |
| Token/credit refill on success | **Our webhook handler** (on `subscription.charged`) |

> Razorpay's own dunning handles transient declines. Our cron is the
> backstop that, when recurring keeps failing, issues the manual invoice
> (§5) and ultimately locks the org.

---

## 5. Payment Links / Invoices (fallback)

When recurring keeps failing, we fall back to a **manual Payment Link** that
grants **exactly one month** of access. Created in code via:

```go
// pkg/payments/payment_links.go
CreatePaymentLink(amountInRs, customer, notes)   // -> Client.PaymentLink.Create(...)
// returns (paymentLinkId, shortUrl)
```

### Dashboard config

- [ ] **Branding** — logo, brand color, business name on the hosted link page
      (Dashboard → *Settings → Branding / Checkout*).
- [ ] **Expiry** — set a sensible `expire_by` (e.g. 7 days) so stale links don't
      linger. (Code can pass `expire_by`; default link expiry is also set in the
      dashboard.)
- [ ] **Partial payments** — **disabled** (a partial pay must not grant a full
      month). Ensure partial payment is OFF on links used for billing.
- [ ] **Notifications** — enable email/SMS notify so the customer receives the
      link and the paid receipt.
- [ ] **Callback** — the link returns to `RedirectUrl`
      (`https://brands.trendly.now`, see `pkg/payments/init.go`).

### Behaviour

- The fallback link is surfaced **only when recurring is failing** (driven by our
  1st-of-month cron after Razorpay dunning gives up).
- A successful pay (`payment_link.paid` webhook → `handlePaymentLink`) grants
  **+1 month** and re-unlocks the org. Access re-locks at the end of that month
  unless recurring recovers or another link is paid.

---

## 6. Webhooks

### Endpoint

- **URL:** `https://be.trendly.now/razorpay/webhook` (prod) /
  `https://be.trendly.now/dev/razorpay/webhook` (dev).
  - API Gateway route prefix `/razorpay/webhook` → lambda
    `functions/razorpay/webhook/main.go`, which mounts the handler at
    `/payment_webhooks` (`apihandler.GinEngine.Any("/payment_webhooks", paymentwebhooks.Handler)`).
- **Secret:** configured in the dashboard, must match `razorpay.webhookKey` in
  `key-secrets.json` (loaded into `payments.WebhookKey`, verified in
  `paymentwebhooks.Handler` via `webhook.VerifyAndParse`). Reject on signature
  mismatch.

### Events to subscribe to

| Event | Why we need it | Handler effect |
|---|---|---|
| `subscription.activated` | mandate authorized / sub live | mark org billing active |
| `subscription.charged` | a monthly charge succeeded | refill tokens, extend access |
| `subscription.pending` | a charge failed, in dunning | mark `past_due` / warn |
| `subscription.halted` | dunning exhausted | lock org (trigger fallback) |
| `subscription.cancelled` | sub cancelled | downgrade org to free |
| `payment_link.paid` | invoice fallback paid | grant +1 month, unlock org |
| `payment.failed` | charge attempt failed | log / surface to billing UI |

Routing in `paymentwebhooks.Handler` (`app.go`): events prefixed `subscription`
→ `HandleSubscription`; prefixed `payment_link` → `handlePaymentLink`.

### ⚠️ Critical `notes` contract

**Every subscription and every payment link MUST carry `notes.organizationId`.**

`resolveBillingTarget` (`billing_target.go`) uses `notes.organizationId` as the
primary key to find the org billing record. Without it, the only fallback is
`notes.brandId` → brand → `brand.OrganizationID`; if neither resolves the
webhook fails with `no-billing-target` and the payment is **not** reflected in
billing.

- `CreateSubscriptionV2` already stamps `notes.organizationId` when the brand has
  an org. Confirm any new code path (agency links, manual links, cron-issued
  invoices) stamps it too.
- Always include `planKey` (and `planCycle` where relevant) in notes for token
  refill sizing.

---

## 7. Keys / Env

| Secret | Location | Read by |
|---|---|---|
| Intl API **key** | `key-secrets.json` → `razorpay.key` | `pkg/payments/init.go` (`apiKey`) |
| Intl API **secret** | `key-secrets.json` → `razorpay.secret` | `pkg/payments/init.go` (`apiSecret`) |
| **Webhook secret** | `key-secrets.json` → `razorpay.webhookKey` | `payments.WebhookKey`, verified in `paymentwebhooks.Handler` |
| **Plans map** | `key-secrets.json` → `razorpay.plans` | `payments.Plans` (see §3) |

- **Test vs live mode**: Razorpay issues separate `rzp_test_*` and `rzp_live_*`
  key pairs, **separate plans**, and **separate webhook secrets**. The test-mode
  fallback (`rzp_test_Z9T0fM1E1agkpR` / `rzp_test_webhook_1234567890`) is
  hardcoded in `pkg/payments/init.go` **only when `key-secrets.json` is missing**
  — never rely on it in dev/prod; always provide a real file.
- `key-secrets.json` is **not** committed; it is provisioned per environment.

---

## 8. Currency Parametrization (code side)

These functions currently **hardcode `"currency": "INR"`** and must be
parametrized to support USD as part of the `BillingProvider` abstraction:

| File | Function | Hardcoded today |
|---|---|---|
| `pkg/payments/payment_links.go` | `CreatePaymentLink` | `"currency": "INR"` (line ~14) |
| `pkg/payments/orders.go` | `CreateOrder` | `"currency": "INR"` (order + each transfer) |

Plan-driven subscriptions (`pkg/payments/subscription.go` →
`CreateSubscriptionLink`) take currency from the **plan** (set when the plan is
created in the dashboard), so subscriptions become USD simply by pointing at USD
`plan_id`s — **no code change needed there**. The currency hardcode only blocks
the **payment-link / order** paths (the invoice fallback and any one-time charge).

**Action:** introduce a currency parameter (default `USD` for org billing) threaded
through `CreatePaymentLink` / `CreateOrder`, behind the `BillingProvider`
abstraction, so the invoice fallback issues USD links.

---

## 9. Test Plan (sandbox / test mode)

Use Razorpay **Test Mode** keys + test cards. Create test USD plans mirroring
prod.

### 9a. Happy path — USD subscribe → webhook → org billing write
- [ ] Create a test org + brand with `organizationId` set.
- [ ] Call `CreateSubscriptionV2` with `planKey=pro`, `planCycle=monthly`.
- [ ] Complete the auth on the hosted subscription page (test card).
- [ ] Verify `subscription.activated` + `subscription.charged` hit
      `/razorpay/webhook`, signature verifies, and `resolveBillingTarget`
      resolves via `notes.organizationId`.
- [ ] Verify org `Billing` is written (`SetBilling`) and **tokens refilled**.

### 9b. Failed charge → past_due → org lock
- [ ] Use a test card that **declines on recurring**.
- [ ] Verify `subscription.pending` → org marked `past_due`.
- [ ] Verify `subscription.halted` (after dunning) → **org locked**.
- [ ] Confirm app gates premium features for the locked org.

### 9c. Invoice fallback → +1 month → re-lock
- [ ] With the org locked, have the cron issue a fallback **Payment Link**
      (carrying `notes.organizationId`).
- [ ] Pay the link (test card) → `payment_link.paid` → `handlePaymentLink`.
- [ ] Verify org gets **+1 month** access and unlocks.
- [ ] Advance past the month (or simulate) → confirm it **re-locks** unless
      recurring recovers / another link is paid.

---

## 10. Founder Action Checklist & Open Questions

### Founder-only manual dashboard actions
- [ ] Enable **International Payments** (account-level approval).
- [ ] Complete/refresh **KYC** + confirm **business category** = SaaS.
- [ ] Pass **website/app review** (pricing, terms, refund policy live).
- [ ] Confirm **USD presentment + INR settlement**; note FX margin + T+24h.
- [ ] Set up **eFIRC / export-invoice** flow (RBI export-of-services).
- [ ] Create USD **Pro ($29/mo)** and **Team ($79/mo)** plans; record `plan_id`s.
- [ ] Enable **Subscriptions** product + international card recurring.
- [ ] Configure **Payment Link** branding, expiry, partial-pay OFF, notify ON.
- [ ] Register the **webhook** at `/razorpay/webhook` with all events in §6;
      copy the **webhook secret** to the engineer.
- [ ] Hand the engineer **live** `key` / `secret` / `webhookKey` / `plan_id`s for
      `key-secrets.json`.

### Open questions
- [ ] First-charge behaviour: small **auth transaction** then charge, or charge
      full first month immediately? (affects trial UX).
- [ ] Confirm the **FX margin %** and whether to absorb it or bake into price.
- [ ] **Agency** tier — fixed custom plan per client, or per-invoice Payment Links?
- [ ] Yearly plans now or later? (code already supports `:yearly` keys).
- [ ] How long to keep the **`brandId` legacy fallback** in `resolveBillingTarget`
      before requiring `organizationId` on all subscriptions?
- [ ] Refund / proration policy on plan **upgrade/downgrade** (Razorpay
      `Subscription.Update` with `schedule_change_at: now` already exists).
