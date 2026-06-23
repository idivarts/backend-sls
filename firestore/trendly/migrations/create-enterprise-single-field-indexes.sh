#!/usr/bin/env bash
#
# create-enterprise-single-field-indexes.sh
# -----------------------------------------------------------------------------
# Creates SINGLE-FIELD indexes on the Firestore *Enterprise* (prod) database.
#
# WHY THIS SCRIPT EXISTS
#   Firestore Enterprise edition:
#     - does NOT auto-create single-field indexes (Standard does), and
#     - REJECTS `fieldOverrides` in firestore.indexes.json on deploy
#       (HTTP 400: "Enterprise Edition does not support updating field index
#        configuration" — only TTL is allowed).
#   So single-field indexes cannot live in firestore.indexes.enterprise.json.
#   They must be created out-of-band, which is what this script does via
#   `gcloud firestore indexes composite create` (a one-field index).
#
#   Composite indexes are still managed in firestore.indexes.enterprise.json and
#   deployed with `firebase deploy --only firestore:indexes --config firebase.prod.json`.
#
# WHAT'S INCLUDED
#   Only single-field queries whose field is NOT already the LEADING field of a
#   composite index (a composite [A,B,C] already serves a lone query on A via its
#   prefix). Fields that ARE covered are listed at the bottom for reference.
#
# BEFORE RUNNING
#   1. Review every line — delete any index you don't actually want (each adds
#      write/storage cost on prod).
#   2. Confirm DATABASE below matches your prod Enterprise DB id.
#   3. `gcloud auth login` and `gcloud config set project "$PROJECT"` (or rely on
#      the --project flag below).
#   4. Enterprise tolerates unindexed single-field queries (they just full-scan,
#      slower) — so you can create these lazily, only when a specific prod query
#      feels slow. You do NOT have to create them all.
#
# USAGE
#   bash create-enterprise-single-field-indexes.sh          # create the indexes
#   DRY_RUN=1 bash create-enterprise-single-field-indexes.sh # print commands only
#
# Index builds run async; check status with:
#   gcloud firestore operations list --database="$DATABASE" --project="$PROJECT"
# -----------------------------------------------------------------------------

set -euo pipefail

PROJECT="${PROJECT:-trendly-9ab99}"
DATABASE="${DATABASE:-trendly-prod}"   # prod Enterprise DB id (from firebase.prod.json)
DRY_RUN="${DRY_RUN:-0}"

# idx <collection> <query-scope> <field-config...>
#   query-scope: COLLECTION | COLLECTION_GROUP
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

# =============================================================================
# users  (covered already: creationTime, lastUseTime, profile.completionPercentage)
# =============================================================================
# user.go:178 — GetUserByEmail: where email ==
idx users COLLECTION field-path=email,order=ascending
# webhook_routes.go:198 — where kyc.accountId ==
idx users COLLECTION field-path=kyc.accountId,order=ascending
# auth/brand contexts — where userId ==
idx users COLLECTION field-path=userId,order=ascending
# collaboration/index.tsx — where moderations.blockedBrands not-in
idx users COLLECTION field-path=moderations.blockedBrands,order=ascending

# =============================================================================
# brands
# =============================================================================
# brand-context — where onboardingComplete ==
idx brands COLLECTION field-path=onboardingComplete,order=ascending

# =============================================================================
# contents  (covered already: isArchived)
# =============================================================================
# content.go:229 — where status ==
idx contents COLLECTION field-path=status,order=ascending
# use-contents — orderBy createdAt desc
idx contents COLLECTION field-path=createdAt,order=descending
# content.go:155 — where postingTimeStamp >=/<  (range + orderBy on same field)
idx contents COLLECTION field-path=postingTimeStamp,order=ascending

# =============================================================================
# strategies
# =============================================================================
# strategy.go:95 / use-strategies.ts:208 — orderBy updatedAt/createdAt desc
idx strategies COLLECTION field-path=updatedAt,order=descending
idx strategies COLLECTION field-path=createdAt,order=descending

# =============================================================================
# websockets
# =============================================================================
# websocket.go:93 — where userId ==
idx websockets COLLECTION field-path=userId,order=ascending

# =============================================================================
# organizations
# =============================================================================
# organization.go:197 — where ownerId ==
idx organizations COLLECTION field-path=ownerId,order=ascending

# =============================================================================
# notifications  (subcollection of users / managers)
# =============================================================================
# notification-context:67 — where isRead ==
idx notifications COLLECTION field-path=isRead,order=ascending
# notification-context:85 — orderBy timeStamp desc
idx notifications COLLECTION field-path=timeStamp,order=descending

# =============================================================================
# messages  (note: leaf name shared by groups/{id}/messages and
#            ai_conversations/{id}/messages; userId+timestamp pair is a composite)
# =============================================================================
# group-context:208 — orderBy timeStamp desc  (groups/{id}/messages)
idx messages COLLECTION field-path=timeStamp,order=descending
# openrouter/conversation.go:123 — orderBy timestamp asc  (ai_conversations/{id}/messages)
idx messages COLLECTION field-path=timestamp,order=ascending

# =============================================================================
# yupdates  (covered already: isSnapshot)
# =============================================================================
# FirestoreYjsProvider.ts:225 — where createdAt < x, orderBy createdAt asc
idx yupdates COLLECTION field-path=createdAt,order=ascending

# =============================================================================
# comments  (subcollections of strategies/contents/months)
# =============================================================================
# use-*-comments — orderBy createdAt asc
idx comments COLLECTION field-path=createdAt,order=ascending

# =============================================================================
# leads / sources  (CrowdyChat-style org subcollections)
# =============================================================================
# lead.go:45 — where sourceId ==
idx leads COLLECTION field-path=sourceId,order=ascending
# source.go:120 — where userId ==
idx sources COLLECTION field-path=userId,order=ascending

# =============================================================================
# scrapped-socials  (covered already: added_by, reel_scrapped_count)
# =============================================================================
# discovery export flow — where exported ==
idx scrapped-socials COLLECTION field-path=exported,order=ascending

# =============================================================================
# socials / socialsPrivate
# =============================================================================
# where socialScreenShots array-contains
idx socials COLLECTION field-path=socialScreenShots,array-config=contains
# where graphType ==
idx socialsPrivate COLLECTION field-path=graphType,order=ascending

# =============================================================================
# COLLECTION_GROUP single-field queries
# =============================================================================
# conversation.go:173/231/234 — collectionGroup("conversations") where leadId/sourceId/phase ==
idx conversations COLLECTION_GROUP field-path=leadId,order=ascending
idx conversations COLLECTION_GROUP field-path=sourceId,order=ascending
idx conversations COLLECTION_GROUP field-path=phase,order=ascending
# campaigns/create.go — collectionGroup("leadStages"/"collectibles") where campaignId ==
idx leadStages COLLECTION_GROUP field-path=campaignId,order=ascending
idx collectibles COLLECTION_GROUP field-path=campaignId,order=ascending
# brand-context:310 — collectionGroup("orgMembers") where managerId ==
idx orgMembers COLLECTION_GROUP field-path=managerId,order=ascending

echo
echo "Done. Index builds are async — track with:"
echo "  gcloud firestore operations list --database=\"$DATABASE\" --project=\"$PROJECT\""

# -----------------------------------------------------------------------------
# ALREADY COVERED by a composite index prefix — NOT created here:
#   collaborations: brandId, status, isLive, timeStamp
#   collaborations-invites: collaborationId, isDiscover, status, managerId
#   contracts: brandId, collaborationId, userId, status, contractTimestamp.startedOn
#   agency-hires: brandId, status
#   inbox: unread, kind, channel, lastActivityAt
#   analyticsSnapshots: socialId, date
#   groups: userIds, updatedAt
#   ai_conversations: brandId, userId, module, contextId, updatedAt
#   applications [CG]: collaborationId, userId, status, timeStamp
#   invitations [CG]: status, userId, influencerId
#   members [CG]: managerId, status
#   users: creationTime, lastUseTime, profile.completionPercentage
#   yupdates: isSnapshot
#   __name__ (document id) — always indexed by Firestore; never needs an override.
# A single-field query on any of the above is served by the leading prefix of an
# existing composite index in firestore.indexes.enterprise.json.
# -----------------------------------------------------------------------------
