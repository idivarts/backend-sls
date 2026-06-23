# Firestore index migrations

Scripts that create indexes which **cannot** be expressed in
`../firestore.indexes.enterprise.json`.

## Why this folder exists

- **Composite indexes (≥2 fields)** belong in the JSON index files
  (`../firestore.indexes.json` for the Standard/dev DB and
  `../firestore.indexes.enterprise.json` for the Enterprise/prod DB) and are
  deployed with `firebase deploy --only firestore:indexes`.
- **Single-field indexes** can't be deployed to the **Enterprise** (prod) DB —
  Firestore returns `HTTP 400: Enterprise Edition does not support updating field
  index configuration` for any `fieldOverrides` (only TTL is allowed). Enterprise
  also does not auto-create single-field indexes. So single-field indexes for prod
  are created out-of-band with `gcloud firestore indexes composite create` (one
  `--field-config`). Those `gcloud` commands live here.

## Rule of thumb

> If a new index **can** go directly in the enterprise JSON (it's composite) →
> put it there. If it **can't** (it's single-field) → add a `gcloud` line to a
> script in this folder.

## Scripts

| Script | Purpose |
|---|---|
| `create-enterprise-single-field-indexes.sh` | Single-field indexes on the prod Enterprise DB (`trendly-prod`) not already covered by a composite-index prefix. Idempotent-ish: re-running an existing index is a no-op error you can ignore. Supports `DRY_RUN=1`. |

## Usage

```bash
gcloud auth login
gcloud config set project trendly-9ab99

DRY_RUN=1 bash create-enterprise-single-field-indexes.sh   # preview only
bash create-enterprise-single-field-indexes.sh             # create (async)

# track build progress
gcloud firestore operations list --database=trendly-prod --project=trendly-9ab99
```

Enterprise runs unindexed single-field queries anyway (slower full-scan), so
these are performance optimizations, not correctness fixes — create lazily as
specific prod queries prove slow.
