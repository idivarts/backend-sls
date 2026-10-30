#!/usr/bin/env bash
#
# Enterprise (prod) single-field index migration — the asset library.
#
# Why this file exists at all: the prod Firestore database runs the Enterprise
# edition, which REJECTS field-index configuration on
# `firebase deploy --only firestore:indexes`, so every single-field index for
# prod is created out-of-band with gcloud. See
# create-enterprise-single-field-indexes.sh for the long version.
#
# Idempotent in effect: gcloud errors if an index already exists, which is safe
# to ignore.
#
# Usage:
#   PROJECT_ID=trendly-9ab99 DATABASE_ID=<prod-db-id> \
#     ./2026-10-30-asset-library-single-field-indexes.sh
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-trendly-9ab99}"
: "${DATABASE_ID:?set DATABASE_ID to the prod (Enterprise) database id}"

# contents.updatedAt (DESCENDING)
#
# Added for the asset library's derived view, which lists a brand's recent
# content to reconstruct library rows from attachments.
#
# It replaced a posting-time RANGE filter, and that is the point: a Firestore
# range filter skips documents missing the field entirely, so filtering on
# postingTimeStamp cannot see an unscheduled draft — which is exactly where an
# upload sits before anyone puts it on the calendar. Measured on dev, one brand
# had 0 of 10 contents with a posting time and 10 of 10 with an updatedAt.
#
# Enterprise still RUNS this query unindexed, just more slowly, so this is a
# performance migration rather than a correctness blocker.
gcloud firestore indexes composite create \
  --project="$PROJECT_ID" \
  --database="$DATABASE_ID" \
  --collection-group=contents \
  --query-scope=COLLECTION \
  --field-config=field-path=updatedAt,order=descending

echo "done — contents.updatedAt (desc) requested for $PROJECT_ID/$DATABASE_ID"

# assets.ingest.status (ASCENDING, COLLECTION_GROUP)
#
# Added for the footage ingest sweep, which finishes what the render worker
# started on an uploaded video. It is a COLLECTION-GROUP query because ingest
# completes on a global cron while assets are brand-scoped subcollections — a
# per-brand sweep would have to enumerate every brand first.
#
# Enterprise does not auto-create single-field indexes and rejects field-index
# config on deploy, so it is created here. It still runs the query unindexed,
# just more slowly, so this is performance rather than a correctness blocker.
gcloud firestore indexes composite create \
  --project="$PROJECT_ID" \
  --database="$DATABASE_ID" \
  --collection-group=assets \
  --query-scope=COLLECTION_GROUP \
  --field-config=field-path=ingest.status,order=ascending

echo "done — assets.ingest.status (collection group) requested"
