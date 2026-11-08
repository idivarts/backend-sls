package trendlymodels

// Registration, reference counting and archival for the brand's media library.
//
// This file exists because of one rule: NOTHING REACHES S3 UNREGISTERED. Not a
// logo, not a generated frame nobody used, not a render that failed review. The
// library is the history of everything the brand has ever put in the bucket,
// which is what makes "use the still I made last week as the start frame" a
// pick rather than a download-and-reupload.
//
// Three things follow from that rule and all three live here:
//
//   - A pre-signed upload is registered BEFORE its bytes exist, because the
//     backend hands out a URL and never sees the transfer. The row goes in as
//     `pending` and is confirmed afterwards against a HEAD of the object, so
//     the size on the row is S3's number and not the client's.
//   - A reference count, maintained with atomic increments, is what makes
//     "archive, never hard-delete" something the code can enforce instead of
//     something the UI politely avoids.
//   - A per-brand byte total, maintained on the same writes, so the number is
//     real whenever storage is ever charged for — not reconstructed later by
//     walking the bucket.
//
// ⚠️ Callers OUTSIDE this package must not write these fields through
// UpdateAsset. Register/Confirm/Archive and AdjustAssetRefCount are the whole
// surface, because each of them also moves the rollup, and a size written
// without its rollup is a storage number that silently drifts.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// RegisterAsset files one piece of media and returns its id.
//
// The single entry point for everything that puts bytes in the bucket. It is
// called by pkg/mediastore and by nothing else; a caller reaching for
// CreateAsset directly is a caller that has skipped the rollup.
//
// `status` defaults to ready. A pre-signed upload passes pending explicitly,
// and only a pending row may later be confirmed.
func RegisterAsset(ctx context.Context, brandID string, a *Asset) (string, error) {
	if a == nil || brandID == "" {
		return "", fmt.Errorf("RegisterAsset: a brand and an asset are required")
	}
	if a.URL == "" && a.S3Key == "" {
		return "", fmt.Errorf("RegisterAsset: media needs a url or an s3 key")
	}
	if a.Status == "" {
		a.Status = AssetStatusReady
	}
	if a.Origin != nil && a.Origin.At == 0 {
		a.Origin.At = time.Now().UnixMilli()
	}
	if a.Origin != nil && a.SourceMediaID == "" {
		a.SourceMediaID = a.Origin.SourceMediaID
	}

	id, err := CreateAsset(ctx, brandID, a)
	if err != nil {
		return "", err
	}
	// Only ready bytes count toward the brand's total. A pending row has no
	// bytes yet by definition, and counting it would make the number jump
	// around on every abandoned upload.
	if a.Status == AssetStatusReady {
		bumpMediaUsage(ctx, brandID, a.Kind, a.SizeBytes, 1)
	}
	return id, nil
}

// ConfirmAsset turns a pending row into a ready one once its bytes have landed.
//
// `sizeBytes` is what S3 reported, which is why this is a separate call rather
// than a field on the hand-out: at hand-out time nobody knows how big the file
// is, including the person choosing it.
//
// Idempotent in the way that matters — confirming an already-ready row does not
// double-count it in the rollup.
func ConfirmAsset(ctx context.Context, brandID, assetID string, m AssetMeasurement) error {
	existing, err := GetAsset(ctx, brandID, assetID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("ConfirmAsset: %s is not in the library", assetID)
	}

	updates := []firestore.Update{
		{Path: "status", Value: AssetStatusReady},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	}
	if m.SizeBytes > 0 {
		updates = append(updates, firestore.Update{Path: "sizeBytes", Value: m.SizeBytes})
	}
	if m.Width > 0 {
		updates = append(updates, firestore.Update{Path: "width", Value: m.Width})
	}
	if m.Height > 0 {
		updates = append(updates, firestore.Update{Path: "height", Value: m.Height})
	}
	if m.DurationMs > 0 {
		updates = append(updates, firestore.Update{Path: "durationMs", Value: m.DurationMs})
	}
	if m.MimeType != "" {
		updates = append(updates, firestore.Update{Path: "mimeType", Value: m.MimeType})
	}
	if _, err := assetsCollection(brandID).Doc(assetID).Update(ctx, updates); err != nil {
		return err
	}

	// The rollup moves by the DIFFERENCE, and the row only joins the count
	// when it was not already ready.
	deltaCount := 0
	if existing.Status != AssetStatusReady {
		deltaCount = 1
	}
	bumpMediaUsage(ctx, brandID, existing.Kind, m.SizeBytes-existing.SizeBytes, deltaCount)
	return nil
}

// AssetMeasurement is what is learned about media once its bytes exist.
type AssetMeasurement struct {
	SizeBytes  int64
	Width      int
	Height     int
	DurationMs int
	MimeType   string
}

// MarkAssetFailed records that an upload or a generation never produced bytes.
//
// The row is kept rather than deleted. A failed generation that was charged for
// is something the user is entitled to see, and a failed upload is the evidence
// behind "it didn't work" — a vanished row is indistinguishable from one that
// was never attempted.
func MarkAssetFailed(ctx context.Context, brandID, assetID, reason string) error {
	_, err := assetsCollection(brandID).Doc(assetID).Update(ctx, []firestore.Update{
		{Path: "status", Value: AssetStatusFailed},
		{Path: "alt", Value: strings.TrimSpace(reason)},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})
	return err
}

// AdjustAssetRefCount moves a media row's reference count by delta.
//
// Atomic, because two scenes can place the same media in the same instant and a
// read-modify-write would lose one of them — leaving a count of 1 on media that
// two scenes depend on, which is exactly the state in which the next archive
// deletes something still on screen.
//
// The count is allowed to go to zero and no further; a negative count would
// make unreferenced media look referenced forever.
func AdjustAssetRefCount(ctx context.Context, brandID, assetID string, delta int) error {
	if assetID == "" || delta == 0 {
		return nil
	}
	doc := assetsCollection(brandID).Doc(assetID)
	if _, err := doc.Update(ctx, []firestore.Update{
		{Path: "refCount", Value: firestore.Increment(delta)},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	}); err != nil {
		return err
	}
	if delta < 0 {
		// Clamp. Cheap to check, and the alternative is a library that can
		// never be tidied because half of it reports -3 references.
		if a, err := GetAsset(ctx, brandID, assetID); err == nil && a != nil && a.RefCount < 0 {
			_, _ = doc.Update(ctx, []firestore.Update{{Path: "refCount", Value: 0}})
		}
	}
	return nil
}

// AdjustAssetRefCounts applies the same delta to several media rows.
//
// What a scene write calls: a scene references everything its layers reference,
// so placing or clearing one scene moves several counts and they move together.
func AdjustAssetRefCounts(ctx context.Context, brandID string, assetIDs []string, delta int) error {
	var firstErr error
	for _, id := range dedupeStrings(assetIDs) {
		if err := AdjustAssetRefCount(ctx, brandID, id, delta); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ErrAssetReferenced is returned when media cannot be archived because
// something still points at it.
var ErrAssetReferenced = fmt.Errorf("media is still referenced")

// ArchiveAsset hides media from the library without touching the bytes.
//
// Refuses while the reference count is above zero, which is the enforceable
// half of "archive, never hard-delete": a published post pointing at a file is
// a post that breaks if the file goes, and no amount of UI confirmation makes
// that safe. `force` is for the case the user genuinely owns — removing their
// own unreferenced media — and still does not delete the object.
func ArchiveAsset(ctx context.Context, brandID, assetID string, force bool) error {
	a, err := GetAsset(ctx, brandID, assetID)
	if err != nil {
		return err
	}
	if a == nil {
		return nil
	}
	if a.RefCount > 0 && !force {
		return fmt.Errorf("%w: %d place(s) use it", ErrAssetReferenced, a.RefCount)
	}
	if _, err := assetsCollection(brandID).Doc(assetID).Update(ctx, []firestore.Update{
		{Path: "status", Value: AssetStatusArchived},
		{Path: "archivedAt", Value: time.Now().UnixMilli()},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	}); err != nil {
		return err
	}
	if a.Status == AssetStatusReady {
		// Archived bytes still sit in the bucket, so the brand's total does
		// NOT drop — only the visible count does. Reporting a smaller number
		// than the bucket holds is the one thing a storage figure must not do.
		bumpMediaUsage(ctx, brandID, a.Kind, 0, -1)
	}
	return nil
}

// UpdateAsset applies partial updates to a media row.
//
// For descriptive fields — alt text, title, a trim, an ingest result. Not for
// status, size or refCount: those move the rollup, and the calls above are the
// only ones that move both together.
func UpdateAsset(ctx context.Context, brandID, assetID string, updates []firestore.Update) error {
	for _, u := range updates {
		switch u.Path {
		case "status", "sizeBytes", "refCount", "archivedAt":
			return fmt.Errorf("UpdateAsset: %q moves the storage rollup; use the lifecycle calls", u.Path)
		}
	}
	updates = append(updates, firestore.Update{Path: "updatedAt", Value: time.Now().UnixMilli()})
	_, err := assetsCollection(brandID).Doc(assetID).Update(ctx, updates)
	return err
}

// GetAssets reads several media rows by id, skipping any that are gone.
//
// Used wherever a scene's layers are resolved: a scene names its media by id,
// and the renderer, the cost estimator and the picker all need the rows.
func GetAssets(ctx context.Context, brandID string, ids []string) (map[string]*Asset, error) {
	out := make(map[string]*Asset, len(ids))
	for _, id := range dedupeStrings(ids) {
		a, err := GetAsset(ctx, brandID, id)
		if err != nil {
			return out, err
		}
		if a != nil {
			out[id] = a
		}
	}
	return out, nil
}

// ListLibraryAssets returns pickable media: ready rows, newest first.
//
// Distinct from ListBrandAssets, which unions in the derived attachment view
// for history. This one answers "what can I put in a scene right now", so a
// pending upload and an archived row are both absent — the first has no bytes
// and the second was deliberately put away.
func ListLibraryAssets(ctx context.Context, brandID, kind, source string, limit int) ([]Asset, error) {
	if limit <= 0 {
		limit = 60
	}
	// Over-read, then filter in memory. The alternative is a composite index
	// per (status, kind, source, createdAt) combination the picker can ask
	// for, to serve a collection of tens of rows per brand.
	rows, err := ListStoredAssets(ctx, brandID, source, limit*3)
	if err != nil {
		return nil, err
	}
	out := make([]Asset, 0, limit)
	for _, a := range rows {
		if !assetIsPickable(a) {
			continue
		}
		if kind != "" && a.Kind != kind {
			continue
		}
		out = append(out, a)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// assetIsPickable reports whether media can be placed in a scene.
//
// A row with NO status at all is pickable: rows predate the registry, and
// reading an empty status as "not finished uploading" would hide every piece of
// media a brand had before this shipped.
func assetIsPickable(a Asset) bool {
	switch a.Status {
	case "", AssetStatusReady:
		return true
	default:
		return false
	}
}

// SearchAssets finds media by words — its title, its alt text, the prompt that
// made it, or what was said in it.
//
// In memory, on purpose. Firestore has no text search, and the shapes that fake
// one (an array of lowercased tokens maintained on write) are worth building
// when a brand has thousands of rows, not tens. The prompt is included because
// "the clip where she picks up the cup" is how people look for a generation
// they have seen once.
func SearchAssets(ctx context.Context, brandID, query, kind string, limit int) ([]Asset, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := ListStoredAssets(ctx, brandID, "", 400)
	if err != nil {
		return nil, err
	}
	needles := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	out := make([]Asset, 0, limit)
	for _, a := range rows {
		if !assetIsPickable(a) {
			continue
		}
		if kind != "" && a.Kind != kind {
			continue
		}
		if len(needles) > 0 && !assetMatches(a, needles) {
			continue
		}
		out = append(out, a)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func assetMatches(a Asset, needles []string) bool {
	var b strings.Builder
	b.WriteString(strings.ToLower(a.Title))
	b.WriteString(" ")
	b.WriteString(strings.ToLower(a.Alt))
	b.WriteString(" ")
	b.WriteString(strings.ToLower(a.Provider))
	if a.Origin != nil {
		b.WriteString(" ")
		b.WriteString(strings.ToLower(a.Origin.Prompt))
		b.WriteString(" ")
		b.WriteString(strings.ToLower(a.Origin.Model))
		b.WriteString(" ")
		b.WriteString(strings.ToLower(a.Origin.Kind))
	}
	if a.Ingest != nil {
		b.WriteString(" ")
		b.WriteString(strings.ToLower(a.Ingest.Text))
		for _, sh := range a.Ingest.Shots {
			b.WriteString(" ")
			b.WriteString(strings.ToLower(sh.Description))
		}
	}
	hay := b.String()
	// Every word has to appear. "founder laughing" should not match every
	// clip with a founder in it.
	for _, n := range needles {
		if !strings.Contains(hay, n) {
			return false
		}
	}
	return true
}

// ListAbandonedPendingAssets finds pre-signed uploads whose bytes never
// arrived, so a sweep can mark them failed.
//
// A collection-group query for the same reason as pending ingests: the rows are
// brand-scoped subcollections and the sweep is global.
func ListAbandonedPendingAssets(ctx context.Context, olderThanMs int64, limit int) ([]PendingIngest, error) {
	if limit <= 0 {
		limit = 50
	}
	cutoff := time.Now().UnixMilli() - olderThanMs
	iter := firestoreCollectionGroupAssets().
		Where("status", "==", AssetStatusPending).
		Where("createdAt", "<", cutoff).
		Limit(limit).
		Documents(ctx)
	defer iter.Stop()

	out := []PendingIngest{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, err
		}
		var a Asset
		if err := doc.DataTo(&a); err != nil {
			continue
		}
		a.ID = doc.Ref.ID
		brandRef := doc.Ref.Parent.Parent
		if brandRef == nil {
			continue
		}
		out = append(out, PendingIngest{BrandID: brandRef.ID, Asset: a})
	}
	return out, nil
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
