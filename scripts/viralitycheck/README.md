# viralitycheck

Verifies what the AI actually writes, against the deployed dev stack.

The Virality Engine changes the system prompt. Nothing about that is visible to
a unit test or a green deploy — the code compiles and ships identically whether
the craft contract is excellent or empty. The only honest check is to generate
against dev and read the result, so that is what this does, mechanically and
without a human in the loop.

## Why not a browser

Driving `dev.brands.trendly.now` needs a password and a person. This mints an
ID token from the admin SDK instead, so a run is unattended, repeatable, and
leaves a diffable transcript every time. No password is involved anywhere.

## Running it

From the `backend-sls` root, so `service-account.json` resolves:

```sh
export FIREBASE_WEB_API_KEY="<the apiKey from trendly-brands/firebase-config.js>"
go run ./scripts/viralitycheck/seed
NODE_PATH=../trendly-brands/node_modules node scripts/viralitycheck/drive.js
```

`seed` is idempotent — fixed ids, re-running refreshes rather than duplicating.
It also re-applies the plan each time, so a wallet drained by earlier testing
does not turn the next run into a confusing `upgrade_required`.

`drive.js` exits non-zero if any case fails a fatal check, so it works as a
gate as well as a report.

Useful flags: `--only <contentId>` for one case, `--out <dir>` to keep
transcripts somewhere specific.

## What it creates on dev

All fixed-id, all named so they are obviously fixtures, all safe to delete:

| Thing | Id |
|---|---|
| Firebase Auth user | `viralitycheck-manager` |
| `managers/` doc | `viralitycheck-manager` |
| `organizations/` doc | generated, pro plan, funded wallet |
| `brands/` doc | `viralitycheck-brand` |
| `brands/.../contents/` | `viralitycheck-{reel,short,linkedin,twitter}` |

**Dev only.** The seeder refuses to run against any `FIRESTORE_DATABASE_ID`
other than the dev default.

## Two things worth knowing

**A seeded org needs a billing block, not just a plan.** `ApplyPlanToOrg` sets
the top-level `planKey`, entitlements and wallet — but AI model routing reads
`org.Billing.PlanKey` (`modelpick.go` → `brandPlan`), which only the
org-creation path writes. An org seeded without one resolves to the free plan
and every AI task returns `upgrade_required` with no reason attached. Real orgs
are fine; `provisionPersonalOrg` sets both. The seeder sets both too.

**The checks are deliberately mechanical.** "I read the scripts and they look
better" is not a verification anyone can disagree with later. A throat-clear
regex either fires or it does not. They are necessary rather than sufficient —
reading the transcripts is still worth doing, and is where you see whether the
per-platform advice actually landed.
