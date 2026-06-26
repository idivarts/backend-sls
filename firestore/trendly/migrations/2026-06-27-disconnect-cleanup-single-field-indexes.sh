#!/usr/bin/env bash
#
# 2026-06-27-disconnect-cleanup-single-field-indexes.sh
# -----------------------------------------------------------------------------
# Enterprise (prod) single-field indexes backing the "delete an account's data on
# disconnect" cleanup (list_brand.go DeleteBrandSocial → DeleteInboxConversationsBySocial
# / DeleteInboxMediaBySocial / DeleteAnalyticsBySocial).
#
# Each runs a `where socialId ==` equality query over a per-brand subcollection:
#   - brands/{brandId}/inbox
#   - brands/{brandId}/inboxMedia
#   - brands/{brandId}/analyticsCache
#
# WHY A SEPARATE SCRIPT
#   Firestore Enterprise REJECTS fieldOverrides on deploy, so single-field indexes
#   are created out-of-band via gcloud (see create-enterprise-single-field-indexes.sh
#   for the full rationale). This is a self-contained batch for the disconnect
#   cleanup feature, kept separate from the main script.
#
# NOTE
#   - Dev (Standard) auto-creates these single-field indexes — this script is
#     prod-only.
#   - analyticsSnapshots.socialId is NOT here: it is already served by the leading
#     prefix of the existing (socialId, date) composite index.
#   - Disconnect is a rare, small per-brand operation, so Enterprise will run these
#     unindexed (full subcollection scan) even without this script — the indexes
#     are a perf optimization, safe to create lazily.
#
# USAGE
#   bash 2026-06-27-disconnect-cleanup-single-field-indexes.sh           # create
#   DRY_RUN=1 bash 2026-06-27-disconnect-cleanup-single-field-indexes.sh # print only
#
# Index builds run async; check status with:
#   gcloud firestore operations list --database="$DATABASE" --project="$PROJECT"
# -----------------------------------------------------------------------------

set -euo pipefail

PROJECT="${PROJECT:-trendly-9ab99}"
DATABASE="${DATABASE:-trendly-prod}"   # prod Enterprise DB id (from firebase.prod.json)
DRY_RUN="${DRY_RUN:-0}"

# idx <collection> <query-scope> <field-config...>
idx() {
  local collection="$1"; shift
  local scope="$1"; shift
  local args=(firestore indexes composite create
    --project="$PROJECT"
    --database="$DATABASE"
    --collection-group="$collection"
    --query-scope="$scope"
    --async)
  local fc
  for fc in "$@"; do
    args+=(--field-config="$fc")
  done
  echo "+ gcloud ${args[*]}"
  if [[ "$DRY_RUN" != "1" ]]; then
    gcloud "${args[@]}"
  fi
}

# Disconnect cleanup — where socialId ==
idx inbox COLLECTION field-path=socialId,order=ascending
idx inboxMedia COLLECTION field-path=socialId,order=ascending
idx analyticsCache COLLECTION field-path=socialId,order=ascending

echo
echo "Done. Index builds are async — track with:"
echo "  gcloud firestore operations list --database=\"$DATABASE\" --project=\"$PROJECT\""
