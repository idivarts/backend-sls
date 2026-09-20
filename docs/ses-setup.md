# Amazon SES Setup

All transactional email from `backend-sls` (contract lifecycle, payments, KYC,
shipments, disputes, SLA nudges, auth) is sent through **Amazon SES v2** by
`pkg/myemail`. This doc covers the AWS-side setup, how the code is wired, and
how to verify it.

> ⚠️ **There is no SendGrid fallback.** The SendGrid sending path was removed —
> if the SES identity in §1 is not verified, email does not go out at all.
> Complete §1 **before** deploying. Rollback is `git revert`, not a variable.

SES **production access is already granted** on this account, so the sandbox
restrictions (200/day, verified recipients only) no longer apply.

---

## 0. The shape of it, at a glance

| | Detail |
|---|---|
| Region | **us-east-1** (`serverless.trendly.yml` declares no `provider.region`, so everything deploys there) |
| Sending identity | `updates.trendly.now` — the `no-reply@updates.trendly.now` sender |
| MAIL FROM domain | `bounce.updates.trendly.now` |
| Configuration set | `trendly-transactional-{dev,prod}` — **created by `sls deploy`**, see §3 |
| Auth | Lambda execution role — **no API key** |
| Production access | Granted |
| Templates | Rendered locally with `html/template`; SES never sees them |
| Still on SendGrid | Marketing contacts only (`pkg/mysendgrid/contact.go`) — see §7 |

### Package layout

`pkg/myemail` owns templates and content; delivery sits behind `mailer.Sender`.
Swapping or adding a provider means changing one assignment in
`pkg/myemail/config.go` — no handler knows which service delivers the mail.

```
pkg/mailer/       Message + Sender interface (depends on nothing)
pkg/myses/        Amazon SES v2 delivery → implements mailer.Sender
pkg/myemail/      Renders templates, owns the sender
pkg/mysendgrid/   Marketing contacts only — no longer sends mail
pkg/crm/          ContactDetails shared by hubspot + mysendgrid
```

Handlers call `myemail.SendCustomHTMLEmail(...)` exactly as before.

---

## 1. Verify the domain identity — required before first deploy

SES → Identities → Create identity → **domain** `updates.trendly.now`, in
**us-east-1**.

### 1a. Easy DKIM

Choose **Easy DKIM**, RSA_2048, and enable "Publish DNS records to Route53" if
the console offers it — the hosted zone for `trendly.now` is
`Z02250033690XMWB8LXL7` and is in the same account, so SES can write the three
CNAMEs itself. Otherwise copy them into Route53 by hand.

Wait for status **Verified** (usually minutes; DNS can take up to 72h).

> SendGrid's existing DKIM / link-branding records can stay — multiple DKIM
> selectors coexist fine, and leaving them costs nothing.

### 1b. Custom MAIL FROM domain

On the identity → **MAIL FROM domain** → `bounce.updates.trendly.now`. Add the
two records SES gives you to Route53:

- `MX` → `feedback-smtp.us-east-1.amazonses.com` (priority 10)
- `TXT` → `"v=spf1 include:amazonses.com ~all"`

Set "Behavior on MX failure" to **Use default MAIL FROM domain**.

**Why this matters:** without it SES uses `amazonses.com` as the envelope-from,
so SPF authenticates a domain that isn't yours and never *aligns* for DMARC.
DKIM alignment alone would still pass DMARC, but you want both.

### 1c. DMARC

TXT record at `_dmarc.trendly.now` (the org domain, not the subdomain):

```
v=DMARC1; p=none; rua=mailto:dmarc@trendly.now; fo=1; pct=100
```

Start at `p=none`, read the aggregate reports for ~2 weeks, then tighten to
`p=quarantine`. Gmail and Yahoo both require a DMARC record for bulk senders.

---

## 2. Configuration sets — created for you by `sls deploy`

**You do not create these by hand.** `serverless.trendly.yml` declares them as
CloudFormation resources, so `sls deploy` creates and updates them per stage:

| Resource | What it is |
|---|---|
| `EmailConfigurationSet` | `trendly-transactional-{stage}` — the set itself, reputation metrics on |
| `EmailEventsTopic` | SNS topic `trendly-be-email-events-{stage}` |
| `EmailEventsTopicPolicy` | Lets the `ses.amazonaws.com` principal publish to that topic |
| `EmailConfigurationSetEventDestination` | Routes `send`, `reject`, `bounce`, `complaint`, `delivery`, `renderingFailure` to the topic |

`SES_CONFIGURATION_SET` is wired with `!Ref EmailConfigurationSet`, so the name
can never drift from the resource or the IAM policy.

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

> **Still manual:** the sending **identity** (§1). CloudFormation cannot verify
> a domain for you.

> **Follow-up (not yet built):** subscribe a Lambda to `EmailEventsTopic` and
> persist suppressions into Firestore so the app stops mailing dead addresses.
> Per the standing rule in `CLAUDE.md`, that needs a model in
> `internal/models/trendlymodels/` plus matching `firestore.rules` and index
> updates in `backend-sls/firestore/trendly/`.

---

## 3. IAM

In `serverless.trendly.yml` — the Lambda execution role gets:

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

No API key is involved — that is the main operational win. The Lambda role
authenticates, so there is no sending credential to rotate or leak.

---

## 4. Environment variables

All set in `serverless.trendly.yml` (`provider.environment`). **None of them
need a GitHub Actions variable or secret** — they are literals or resolved by
CloudFormation at deploy time:

| Variable | Value | Notes |
|---|---|---|
| `EMAIL_SENDER_NAME` | `Trendly` | |
| `EMAIL_SENDER_ADDRESS` | `no-reply@updates.trendly.now` | Must match the verified identity |
| `EMAIL_REPLY_TO` | `support@trendly.now` | |
| `SES_REGION` | `${aws:region}` | |
| `SES_CONFIGURATION_SET` | `!Ref EmailConfigurationSet` | Resolved by CloudFormation |

`SENDGRID_API_KEY` (repo-level secret) is still passed through, but **only** for
the marketing-contacts sync — see §7.

The legacy `SENDGRID_NAME` / `SENDGRID_EMAIL` are still read as a fallback for
the sender identity, so a stale deploy cannot produce an empty `From` (which SES
rejects outright).

### Local development

SES uses the ambient AWS credential chain (`SharedConfigState: SharedConfigEnable`),
the same as the existing S3 upload code — so your normal AWS profile works. Add
to `.env.local` only if you want to override the defaults:

```
SES_REGION=us-east-1
EMAIL_SENDER_ADDRESS=no-reply@updates.trendly.now
```

---

## 5. Testing — use the mailbox simulator

SES provides addresses that exercise each outcome **without touching your
reputation**. Use these, never a real inbox, for anything automated:

| Address | Result |
|---|---|
| `success@simulator.amazonses.com` | Delivered |
| `bounce@simulator.amazonses.com` | Hard bounce |
| `complaint@simulator.amazonses.com` | Marked as spam |
| `suppressionlist@simulator.amazonses.com` | Rejected, on the suppression list |
| `ooto@simulator.amazonses.com` | Out-of-office auto-reply |

They do not count against your bounce/complaint rates.

> `internal/trendlyapis/collaborations/collab_test.go:54` currently sends to
> real addresses (`rahul@idiv.in` and a gmail account). Point it at
> `success@simulator.amazonses.com`.

Verify the plain-text alternative and link handling locally:

```bash
go test ./pkg/myemail/ -run TestHtmlToText -v
```

---

## 6. Rollout

1. **Finish §1.** The identity must read **Verified** before the first deploy —
   there is no fallback provider.
2. **Deploy dev.** Trigger real flows (signup verification, an application, a
   shipment) and confirm delivery plus events arriving on `EmailEventsTopic`.
3. **Deploy prod.**
4. **Watch for a week:** SES Account dashboard (bounce + complaint rate), the
   SNS event stream, and CloudWatch logs for `myemail:` / `myses:` lines.
5. **If it goes wrong:** `git revert` the migration and redeploy. There is no
   environment-variable rollback.

### Reputation / warm-up

You are on SES's shared IP pool, so the domain+IP pairing is new even though the
domain is not. At transactional volume this is a non-issue. Above roughly
10k/day, ramp over 2–4 weeks rather than switching all at once.

---

## 7. ⚠️ Marketing contacts still run on SendGrid

`pkg/mysendgrid/contact.go` uses the **SendGrid Marketing Contacts API**
(`/v3/marketing/contacts`) with custom fields `user_type`, `company`,
`profile_completion`, `social_link`, `creation_time`, `last_use_time`.

**SES has no equivalent.** Its "contact lists" exist only for unsubscribe
management — a single opaque attributes blob, no segmentation, no campaigns. So
this one path still calls SendGrid and still needs `SENDGRID_API_KEY`.

Callers: `internal/trendlyapis/crm.go`, `scripts/sync-sengrid/main.go`.

**Recommended resolution — fold into HubSpot.** `pkg/hubspot.CreateOrUpdateContacts`
takes the identical `[]crm.ContactDetails`, and `crm.go` already calls **both**
side by side. Drop the SendGrid call, retire `scripts/sync-sengrid`, and the
SendGrid account can be closed entirely.

**Check first:** if any *marketing* campaigns are sent from the SendGrid UI, SES
cannot replace those (no campaign builder). The usual split is SES for
transactional + SendGrid/Loops/Customer.io for marketing.

---

## 8. Deliverability follow-ups

- **`List-Unsubscribe` on bulk mail.** Gmail/Yahoo bulk-sender rules require
  one-click unsubscribe (`List-Unsubscribe` + `List-Unsubscribe-Post`) on
  non-transactional mail. The SLA nudges (`templates/sla_nudge_*.html`) and
  `message_reminder.html` are the borderline ones. The plumbing is in place —
  `mailer.Message.Headers` passes straight through to SES as `MessageHeader`
  entries — so this is a small change once the unsubscribe endpoint exists.
- **Spam complaint rate** must stay under 0.3% (Google Postmaster Tools).
- **Empty template:** `templates/payment_order_created.html` is a 0-byte file
  committed empty since `dfe1022`. SES rejects a send with an empty body, so
  this is now a hard failure rather than a silent one — tracked separately.

---

## 9. Cost

SES is roughly **$0.10 per 1,000 emails** plus data transfer, versus a SendGrid
monthly plan. At Trendly's transactional volume this is single-digit dollars a
month. Confirm against current SES pricing for us-east-1 before quoting the
saving anywhere.
