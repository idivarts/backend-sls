# Amazon SES Setup — SendGrid → SES Migration Walkthrough

All transactional email from `backend-sls` (contract lifecycle, payments, KYC,
shipments, disputes, SLA nudges, auth) is sent by `pkg/myemail`. This doc covers
moving that path from **SendGrid** to **Amazon SES v2**.

The code change is already done. `EMAIL_PROVIDER` picks the delivery path at
runtime, so the cutover — and the rollback — is a single environment variable,
not a redeploy.

> ⚠️ **`EMAIL_PROVIDER` defaults to `ses`.** Deploying this code switches a
> stage to SES unless that stage's GitHub Environment says otherwise. Do §1–§2
> **before** the first deploy, or set `EMAIL_PROVIDER=sendgrid` in the `prod`
> Environment first. See §7.

This doc is written as a **walkthrough you follow in order**: AWS/DNS setup →
sandbox exit → dev → verify → prod → decommission.

---

## 0. The shape of it, at a glance

| | Detail |
|---|---|
| Region | **us-east-1** (`serverless.trendly.yml` declares no `provider.region`, so everything deploys there) |
| Sending identity | `updates.trendly.now` — the existing `no-reply@updates.trendly.now` sender |
| MAIL FROM domain | `bounce.updates.trendly.now` |
| Configuration set | `trendly-transactional-{dev,prod}` — **created by `sls deploy`**, see §3 |
| Auth | Lambda execution role — **no API key** |
| Cutover switch | `EMAIL_PROVIDER` variable in the stage's GitHub Environment (`dev` / `prod`) |
| Templates | Rendered locally with `html/template`; SES never sees them |
| Not migrating | Marketing contacts (`pkg/mysendgrid/contact.go`) — see §8 |

**Why the sending identity stays `updates.trendly.now`:** that subdomain already
carries the domain reputation built up through SendGrid. SES DKIM records
coexist with SendGrid's, so nothing breaks while both are live — which is what
makes an instant rollback possible.

### Package layout

`pkg/myemail` owns templates and content; the providers own delivery. They talk
through `mailer.Sender`, so adding or removing a provider touches one file.

```
pkg/mailer/       Message + Sender interface (depends on nothing)
pkg/myses/        Amazon SES v2 delivery          → implements mailer.Sender
pkg/mysendgrid/   SendGrid delivery + marketing contacts (legacy)
pkg/crm/          ContactDetails — the contact type hubspot + sendgrid share
pkg/myemail/      Renders templates, picks the sender from EMAIL_PROVIDER
```

Handlers keep calling `myemail.SendCustomHTMLEmail(...)` exactly as before.

---

## 1. Verify the domain identity (SES → Identities → Create identity)

Create a **domain** identity for `updates.trendly.now` in **us-east-1**.

### 1a. Easy DKIM

Choose **Easy DKIM**, RSA_2048, and **enable** "Publish DNS records to Route53"
if the console offers it — the hosted zone for `trendly.now` is
`Z02250033690XMWB8LXL7` and is in the same account, so SES can write the three
CNAMEs itself. Otherwise copy them into Route53 by hand.

Wait for status **Verified** (usually minutes; DNS can take up to 72h).

> Do **not** delete SendGrid's existing DKIM / link-branding records. Multiple
> DKIM selectors coexist fine, and you need SendGrid working for rollback.

### 1b. Custom MAIL FROM domain

On the identity → **MAIL FROM domain** → set `bounce.updates.trendly.now`.
Add the two records SES gives you to Route53:

- `MX` → `feedback-smtp.us-east-1.amazonses.com` (priority 10)
- `TXT` → `"v=spf1 include:amazonses.com ~all"`

Set "Behavior on MX failure" to **Use default MAIL FROM domain**.

**Why this matters:** without it SES uses `amazonses.com` as the envelope-from,
so SPF authenticates a domain that isn't yours and never *aligns* for DMARC.
DKIM alignment alone would still pass DMARC, but you want both.

### 1c. DMARC

Add a TXT record at `_dmarc.trendly.now` (the org domain, not the subdomain):

```
v=DMARC1; p=none; rua=mailto:dmarc@trendly.now; fo=1; pct=100
```

Start at `p=none`, read the aggregate reports for ~2 weeks, then tighten to
`p=quarantine`. Gmail and Yahoo both require a DMARC record for bulk senders.

---

## 2. Request production access (do this on day one)

SES starts every account in the **sandbox**: 200 messages/day, 1 message/sec,
and **only to verified recipient addresses**. That is the long pole — approval
is usually ~24h but the request gets bounced back if the description is thin.

SES → **Account dashboard** → *Request production access*:

- **Mail type:** Transactional
- **Website URL:** `https://trendly.now`
- **Use case:** describe it concretely — per-contract lifecycle notifications
  (payments, shipments, deliverables, disputes, payouts), auth emails
  (verification, password reset), and SLA reminders, all triggered by user
  actions inside the product. Recipients are registered brands and creators.
- **Bounce/complaint handling:** say that a configuration set publishes bounce
  and complaint events to SNS, that the account-level suppression list is
  enabled, and that recipients are product users who can disable notifications
  in-app.

In the same request, ask for the sending quota you actually need (messages/day
and messages/second) based on peak volume.

While you wait, verify your own address as a recipient identity so you can test.

---

## 3. Configuration sets — created for you by `sls deploy`

**You do not create these by hand.** `serverless.trendly.yml` declares them as
CloudFormation resources, so `sls deploy` creates (and updates) them per stage:

| Resource | What it is |
|---|---|
| `EmailConfigurationSet` | `trendly-transactional-{stage}` — the set itself, reputation metrics on |
| `EmailEventsTopic` | SNS topic `trendly-be-email-events-{stage}` |
| `EmailEventsTopicPolicy` | Lets the `ses.amazonaws.com` principal publish to that topic |
| `EmailConfigurationSetEventDestination` | Routes `send`, `reject`, `bounce`, `complaint`, `delivery`, `renderingFailure` to the topic |

`SES_CONFIGURATION_SET` is wired with `!Ref EmailConfigurationSet`, so the name
can never drift from the resource.

**What a configuration set is, and why it matters:** it is the handle SES
attaches per-send that makes delivery observable — without one you get no
bounce, complaint or delivery events at all. SES does **not** create one during
domain verification, and a send naming a set that does not exist fails with
`ConfigurationSetDoesNotExistException`. Declaring it in CloudFormation removes
both failure modes.

SES throttles or pauses an account whose **bounce rate exceeds 5%** or
**complaint rate exceeds 0.1%**, and unlike SendGrid there is no free dashboard
for this. The account-level suppression list is enabled by default — good, but
it means sends get silently dropped, so you need the events to know.

> **Still manual:** the sending **identity** (§1) — the verified domain and its
> DKIM / MAIL FROM DNS — plus production access (§2). CloudFormation cannot
> verify a domain for you.

> **Follow-up (not in this change):** subscribe a Lambda to `EmailEventsTopic`
> and persist suppressions into Firestore so the app stops mailing dead
> addresses. Per the standing rule in `CLAUDE.md`, that needs a model in
> `internal/models/trendlymodels/` plus matching `firestore.rules` and index
> updates in `backend-sls/firestore/trendly/`.

---

## 4. IAM

Already committed in `serverless.trendly.yml` — the Lambda execution role gets:

```yaml
- Effect: "Allow"
  Action:
    - "ses:SendEmail"
  Resource:
    - "arn:aws:ses:${aws:region}:${aws:accountId}:identity/updates.trendly.now"
    - "arn:aws:ses:${aws:region}:${aws:accountId}:configuration-set/trendly-transactional-${self:provider.stage}"
```

Both ARNs are required: SES authorizes the identity **and** the configuration
set separately, so omitting the second one fails every send with `AccessDenied`.

No API key is involved — which is the main operational win here. Once SendGrid
is fully decommissioned the `SENDGRID_API_KEY` secret disappears from GitHub.

---

## 5. Environment variables

Set in `serverless.trendly.yml` (`provider.environment`):

| Variable | Value | Notes |
|---|---|---|
| `EMAIL_PROVIDER` | `${env:EMAIL_PROVIDER, 'ses'}` | The cutover switch |
| `EMAIL_SENDER_NAME` | `Trendly` | |
| `EMAIL_SENDER_ADDRESS` | `no-reply@updates.trendly.now` | Must match the verified identity |
| `EMAIL_REPLY_TO` | `support@trendly.now` | |
| `SES_REGION` | `${aws:region}` | |
| `SES_CONFIGURATION_SET` | `!Ref EmailConfigurationSet` | Resolved by CloudFormation |

### The cutover switch

The deploy job is already bound to a GitHub **Environment** (`dev` on the `dev`
branch, `prod` on `master`), so `vars.*` resolve per-stage on their own — one
variable name, two values, no `_DEV` / `_PROD` suffixes:

```yaml
# .github/workflows/deploy-trendly.yaml
environment: ${{ github.ref == 'refs/heads/master' && 'prod' || 'dev' }}
...
EMAIL_PROVIDER: ${{ vars.EMAIL_PROVIDER }}
```

| Value of `EMAIL_PROVIDER` in that Environment | Result |
|---|---|
| unset (or empty) | **SES** — the serverless default |
| `ses` | SES |
| `sendgrid` | SendGrid (rollback) |
| anything else | SES, with a warning logged at startup |

The legacy `SENDGRID_NAME` / `SENDGRID_EMAIL` are still read as a fallback for
the sender, so a half-rolled-out deploy can never produce an empty `From`
(which SES rejects outright).

### Local development

SES uses the ambient AWS credential chain (`SharedConfigState: SharedConfigEnable`),
the same as the existing S3 upload code — so your normal AWS profile works. Set
in `.env.local`:

```
EMAIL_PROVIDER=ses
SES_REGION=us-east-1
EMAIL_SENDER_ADDRESS=no-reply@updates.trendly.now
```

While the account is still in the sandbox, every local test recipient must be a
verified identity.

---

## 6. Testing — use the mailbox simulator

SES provides addresses that exercise each outcome **without touching your
reputation**. Use these, never a real inbox, for anything automated:

| Address | Result |
|---|---|
| `success@simulator.amazonses.com` | Delivered |
| `bounce@simulator.amazonses.com` | Hard bounce |
| `complaint@simulator.amazonses.com` | Marked as spam |
| `suppressionlist@simulator.amazonses.com` | Rejected, on the suppression list |
| `ooto@simulator.amazonses.com` | Out-of-office auto-reply |

These work **in the sandbox** and do not count against your bounce/complaint
rates.

> `internal/trendlyapis/collaborations/collab_test.go:54` currently sends to
> real addresses (`rahul@idiv.in` and a gmail account). Point it at
> `success@simulator.amazonses.com`.

Verify the plain-text alternative and link handling locally:

```bash
go test ./pkg/myemail/ -run TestHtmlToText -v
```

---

## 7. Cutover runbook

Because the default is `ses`, **the first deploy of this branch is the
cutover** for any stage whose Environment does not say otherwise. Pick one:

**Option A — AWS first (recommended).** Finish §1 and §2 before merging.
Then merging to `dev` and `master` switches each stage over as it deploys.

**Option B — stage it.** Set `EMAIL_PROVIDER=sendgrid` in the **`prod`** GitHub
Environment *before* merging. Prod keeps using SendGrid; dev goes to SES on its
next deploy. Delete the prod variable when you're ready to flip.

Then:

1. **Verify dev.** Trigger real flows (signup verification, an application, a
   shipment) and confirm delivery plus events arriving on `EmailEventsTopic`.
2. **Wait for production access** before letting prod run on SES — in the
   sandbox it could only mail verified addresses.
3. **Flip prod** (delete the `sendgrid` variable from the prod Environment, or
   just deploy if you took Option A).
4. **Watch for a week:** SES Account dashboard (bounce + complaint rate), the
   SNS event stream, and CloudWatch logs for `myemail:` / `myses:` lines.
5. **Rollback if needed:** set `EMAIL_PROVIDER=sendgrid` in that Environment and
   redeploy. SendGrid DNS and the API key are still live, so this is immediate.
6. **Decommission** only after a clean week: resolve §8, then delete
   `pkg/mysendgrid/`, drop `github.com/sendgrid/sendgrid-go` from `go.mod`, and
   remove the `SENDGRID_*` entries from `serverless.trendly.yml` and the deploy
   workflow.

### Reputation / warm-up

You are moving to SES's shared IP pool, so the domain+IP pairing is new even
though the domain is not. At transactional volume this is a non-issue. Above
roughly 10k/day, ramp over 2–4 weeks rather than switching all at once.

---

## 8. ⚠️ Marketing contacts do NOT migrate

`pkg/mysendgrid/contact.go` uses the **SendGrid Marketing Contacts API**
(`/v3/marketing/contacts`) with custom fields `user_type`, `company`,
`profile_completion`, `social_link`, `creation_time`, `last_use_time`.

**SES has no equivalent.** Its "contact lists" exist only for unsubscribe
management — a single opaque attributes blob, no segmentation, no campaigns.

It therefore ignores `EMAIL_PROVIDER` and always talks to SendGrid, reading
`SENDGRID_API_KEY` directly.

Callers: `internal/trendlyapis/crm.go`, `scripts/sync-sengrid/main.go`.

**Recommended resolution — fold into HubSpot.** `pkg/hubspot.CreateOrUpdateContacts`
takes the identical `[]crm.ContactDetails`, and `crm.go` already calls **both**
side by side. Drop the SendGrid call, retire `scripts/sync-sengrid`, done.

**Check first:** if any *marketing* campaigns are sent from the SendGrid UI, SES
cannot replace those (no campaign builder). The usual split is SES for
transactional + SendGrid/Loops/Customer.io for marketing — in which case
`SENDGRID_API_KEY` stays permanently.

Until this is resolved, `SENDGRID_API_KEY` must remain set even on stages
running `EMAIL_PROVIDER=ses`.

---

## 9. Deliverability follow-ups

- **`List-Unsubscribe` on bulk mail.** Gmail/Yahoo bulk-sender rules require
  one-click unsubscribe (`List-Unsubscribe` + `List-Unsubscribe-Post`) on
  non-transactional mail. The SLA nudges (`templates/sla_nudge_*.html`) and
  `message_reminder.html` are the borderline ones. The plumbing is already in
  place — `mailer.Message.Headers` is passed through to SES as `MessageHeader`
  entries and to SendGrid via `SetHeader`, so this is a small change once the
  unsubscribe endpoint exists.
- **Spam complaint rate** must stay under 0.3% (Google Postmaster Tools).
- **Empty template:** `templates/payment_order_created.html` is a 0-byte file
  committed empty since `dfe1022`. SES rejects a send with an empty body, so
  this is now a hard failure rather than a silent one — tracked separately.

---

## 10. Cost

SES is roughly **$0.10 per 1,000 emails** plus data transfer, versus a
SendGrid monthly plan. At Trendly's transactional volume this is single-digit
dollars a month. Confirm against current SES pricing for us-east-1 before
quoting the saving anywhere.
