#!/usr/bin/env bash
#
# Enterprise (prod) single-field index migrations.
#
# The prod Firestore database runs the **Enterprise** edition, which REJECTS any
# field-index configuration on `firebase deploy --only firestore:indexes`
# (HTTP 400: "Enterprise Edition does not support updating field index
# configuration"). So firestore.indexes.enterprise.json keeps `fieldOverrides: []`
# and every single-field / COLLECTION_GROUP-scoped single-field index for prod is
# created out-of-band here via gcloud.
#
# These are idempotent in effect: gcloud errors if an index already exists — that
# is safe to ignore. Composite (>=2 field) indexes do NOT belong here; put those
# in BOTH firestore.indexes.json and firestore.indexes.enterprise.json instead.
#
# Usage:
#   PROJECT_ID=trendly-9ab99 DATABASE_ID=<prod-db-id> \
#     ./create-enterprise-single-field-indexes.sh
#
# DATABASE_ID is the prod (Enterprise) named database — the same value passed to
# deploys as FIRESTORE_DATABASE_PROD_ID. PROJECT_ID defaults to trendly-9ab99.

set -uo pipefail

PROJECT_ID="${PROJECT_ID:-trendly-9ab99}"
DATABASE_ID="${DATABASE_ID:?Set DATABASE_ID to the prod (Enterprise) database id}"

echo "Creating Enterprise single-field indexes on project=$PROJECT_ID database=$DATABASE_ID"

# ── inbox.participant.id (COLLECTION_GROUP) ───────────────────────────────────
# Backs the Meta data-deletion purge: a collection-group query that finds and
# deletes every brands/{brandId}/inbox conversation a given platform user
# participates in (trendlymodels.DeleteInboxConversationsByParticipant).
gcloud firestore indexes composite create \
  --collection-group=inbox \
  --query-scope=COLLECTION_GROUP \
  --field-config=field-path=participant.id,order=ascending \
  --project="$PROJECT_ID" \
  --database="$DATABASE_ID"

echo "Done."
