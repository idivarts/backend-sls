package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// countAlias is the aggregation alias used for every COUNT query below. The
// value is arbitrary — Firestore just echoes it back as the result key.
const countAlias = "count"

// CountQuery runs a server-side COUNT aggregation over q and returns how many
// documents match.
//
// Prefer this over the fetch-all-then-len() pattern used elsewhere in this
// package (see CountOrgMembers, CountInboxConversations): those bill one read
// per document, which is fine for a single small subcollection but not for the
// admin CRM, which fans counts out across every brand. A COUNT aggregation is
// billed per 1000 index entries scanned instead, and transfers no document data.
func CountQuery(ctx context.Context, q firestore.Query) (int, error) {
	res, err := q.NewAggregationQuery().WithCount(countAlias).Get(ctx)
	if err != nil {
		return 0, err
	}
	raw, ok := res[countAlias]
	if !ok {
		return 0, nil
	}
	val, ok := raw.(*pb.Value)
	if !ok {
		return 0, nil
	}
	return int(val.GetIntegerValue()), nil
}
