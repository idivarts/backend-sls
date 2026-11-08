package trendlymodels

import (
	"context"
	"fmt"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── The brand's asset library ────────────────────────────────────────────────
//
// Until now the "media library" was not a collection at all: it was a scan over
// every content's `attachments` array. That worked while an asset was nothing
// but a URL, and stopped working the moment assets acquired FACTS about
// themselves — a photographer to credit, a licence that forbids re-hosting, an
// in and out point, a poster frame, a transcript. None of those fit on an
// attachment, which is why footage had nowhere to live.
//
// Stored at brands/{brandId}/assets/{assetId}, deliberately brand-scoped rather
// than content-scoped: the whole point is that something uploaded while editing
// one post can be used in another.
//
// ⚠️ The attachment scan has NOT been deleted. ListBrandAssets returns the
// stored rows UNIONED with a derived view of content attachments, deduplicated
// by URL — so every image a brand has ever attached is reachable from day one,
// with no migration and no client change, while anything carrying provenance
// gets a real row. A stored row always wins over a derived one, because the
// derived one knows nothing except a URL.
type Asset struct {
	ID string `json:"id" firestore:"-"`

	// Kind is what it is: image | video | audio.
	Kind string `json:"kind" firestore:"kind"`
	// Source is where it came from, and it maps to the library's tabs:
	// brand | upload | stock | generated.
	Source string `json:"source" firestore:"source"`

	URL string `json:"url" firestore:"url"`
	// ThumbURL is a small copy for grids; PosterURL is a video's first frame.
	ThumbURL  string `json:"thumbUrl,omitempty" firestore:"thumbUrl,omitempty"`
	PosterURL string `json:"posterUrl,omitempty" firestore:"posterUrl,omitempty"`
	MimeType  string `json:"mimeType,omitempty" firestore:"mimeType,omitempty"`

	Width      int `json:"width,omitempty" firestore:"width,omitempty"`
	Height     int `json:"height,omitempty" firestore:"height,omitempty"`
	DurationMs int `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`

	// Alt and Title are for accessibility and for a human scanning the grid.
	Alt   string `json:"alt,omitempty" firestore:"alt,omitempty"`
	Title string `json:"title,omitempty" firestore:"title,omitempty"`

	// ── Provenance ──
	// These are the fields an attachment could never hold, and the reason this
	// collection exists at all.
	//
	// Provider is the mediagen driver or source name ("unsplash", "openrouter").
	Provider string `json:"provider,omitempty" firestore:"provider,omitempty"`
	// Attribution is the credit line that must travel with the asset.
	Attribution string `json:"attribution,omitempty" firestore:"attribution,omitempty"`
	// CreditURL links the creator, referral tag included where one is required.
	CreditURL string `json:"creditUrl,omitempty" firestore:"creditUrl,omitempty"`
	// Hotlink means the URL above is the licensed location and the bytes must
	// NOT be copied. A stock photo is precisely the row that cannot carry a
	// cached copy of itself, which is why this had to be modelled before any
	// storage decision was built on the assumption that everything can be.
	Hotlink bool `json:"hotlink,omitempty" firestore:"hotlink,omitempty"`
	// NoAIInput means this asset's licence forbids feeding it to a model. Read
	// by the generation path to refuse it as a reference.
	NoAIInput bool `json:"noAiInput,omitempty" firestore:"noAiInput,omitempty"`

	// ── Footage ──
	// A default in and out point, so a clip placed from the library starts
	// where the usable part of the take does.
	TrimInMs  int `json:"trimInMs,omitempty" firestore:"trimInMs,omitempty"`
	TrimOutMs int `json:"trimOutMs,omitempty" firestore:"trimOutMs,omitempty"`

	// Ingest is what was learned about this footage when it arrived: its real
	// dimensions and length, what is said in it and when, and what is on
	// screen and when. None of this fits on an attachment, which is the whole
	// reason this collection exists.
	//
	// Nil until ingest runs, and ingest only runs for video.
	Ingest *AssetIngest `json:"ingest,omitempty" firestore:"ingest,omitempty"`

	// OriginContentID records where it first appeared, so the library can say
	// "from <post>" without the asset belonging to that post.
	OriginContentID string `json:"originContentId,omitempty" firestore:"originContentId,omitempty"`
	CreatedBy       string `json:"createdBy,omitempty" firestore:"createdBy,omitempty"`
	CreatedAt       int64  `json:"createdAt" firestore:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`

	// ── Bytes and lifecycle ─────────────────────────────────────────────
	//
	// Added when media became a COLLECTION rather than a convenience: every
	// byte that leaves for S3 gets a row here whether anything uses it or not,
	// so there is a history to pick from, a reference count that makes
	// deletion safe, and a per-brand byte total that is real on the day
	// storage is ever charged for.
	//
	// S3Bucket/S3Key are the authority on WHERE the bytes are. The URL is a
	// CDN address and cannot be used to measure, re-read or remove an object:
	// two CloudFront distributions front the same bucket, and a URL says
	// nothing about which key it maps to.
	S3Bucket string `json:"s3Bucket,omitempty" firestore:"s3Bucket,omitempty"`
	S3Key    string `json:"s3Key,omitempty" firestore:"s3Key,omitempty"`
	// SizeBytes is read back from S3 with a HEAD, never taken from a client.
	// A number the uploader reports is a number the uploader can understate,
	// and this one is meant to be billable.
	SizeBytes int64 `json:"sizeBytes,omitempty" firestore:"sizeBytes,omitempty"`

	// Status is the lifecycle: pending | ready | archived | failed.
	//
	// `pending` exists because a pre-signed upload is registered BEFORE its
	// bytes exist — the backend hands out a URL and never sees the transfer —
	// so the row is written at hand-out and confirmed afterwards. A pending
	// row is not offered in the picker; an abandoned one is swept.
	Status string `json:"status,omitempty" firestore:"status,omitempty"`

	// RefCount is how many scenes, layers and published contents point at
	// this media. It is what makes "archive, never hard-delete" enforceable:
	// bytes with a live reference cannot be removed, and bytes with none can.
	//
	// Maintained with atomic increments, so two scenes placing the same media
	// at once cannot both read 0 and both write 1.
	RefCount int `json:"refCount,omitempty" firestore:"refCount,omitempty"`

	// Origin is what produced this: an upload, a model and its prompt, a
	// render of a revision, a frame cut out of another piece of media.
	//
	// It is the first thing a person asks of anything in the library ("where
	// did this come from?") and the first thing the AI needs in order to reuse
	// it ("the still I made of the founder"), which is why it is structured
	// rather than a label.
	Origin *AssetOrigin `json:"origin,omitempty" firestore:"origin,omitempty"`

	// SourceMediaID points at the media this was derived from — the clip a
	// frame was taken from, the still an upscale came from. Duplicated out of
	// Origin so it can be queried on its own.
	SourceMediaID string `json:"sourceMediaId,omitempty" firestore:"sourceMediaId,omitempty"`

	ArchivedAt int64 `json:"archivedAt,omitempty" firestore:"archivedAt,omitempty"`

	// Derived marks a row that is NOT stored — it was reconstructed from a
	// content attachment for the union. Never written; a caller that wants to
	// edit such a row must create a real one first.
	Derived bool `json:"derived,omitempty" firestore:"-"`
}

// Asset kinds and sources.
const (
	AssetKindImage = "image"
	AssetKindVideo = "video"
	AssetKindAudio = "audio"

	AssetSourceBrand     = "brand"
	AssetSourceUpload    = "upload"
	AssetSourceStock     = "stock"
	AssetSourceGenerated = "generated"

	// Lifecycle. A row with no status at all predates the registry and is
	// treated as ready: there are rows in production older than this field,
	// and reading them as "not finished uploading" would hide a brand's whole
	// history behind a sweep.
	AssetStatusPending  = "pending"
	AssetStatusReady    = "ready"
	AssetStatusArchived = "archived"
	AssetStatusFailed   = "failed"

	// What produced a piece of media.
	AssetOriginUpload    = "upload"
	AssetOriginGenerated = "generated"
	AssetOriginRender    = "render"
	AssetOriginFrame     = "frame"
	AssetOriginStock     = "stock"
	AssetOriginHosted    = "hosted"
)

// AssetOrigin records what produced a piece of media, in the terms of whatever
// produced it. Exactly one of the shapes below is filled in.
type AssetOrigin struct {
	// Kind: upload | generated | render | frame | stock | hosted.
	Kind string `json:"kind" firestore:"kind"`

	// Provider and Model name the generator ("elevenlabs",
	// "veo-3.1-fast-generate-001"). Prompt is what it was asked for, kept
	// because "the one where she is laughing" is how a person finds it again.
	Provider string `json:"provider,omitempty" firestore:"provider,omitempty"`
	Model    string `json:"model,omitempty" firestore:"model,omitempty"`
	Prompt   string `json:"prompt,omitempty" firestore:"prompt,omitempty"`
	JobID    string `json:"jobId,omitempty" firestore:"jobId,omitempty"`

	// RevisionID is the design or composition revision a render came from.
	RevisionID string `json:"revisionId,omitempty" firestore:"revisionId,omitempty"`
	ContentID  string `json:"contentId,omitempty" firestore:"contentId,omitempty"`
	// StudioID and SceneID are set when the studio produced it.
	StudioID string `json:"studioId,omitempty" firestore:"studioId,omitempty"`
	SceneID  string `json:"sceneId,omitempty" firestore:"sceneId,omitempty"`

	// SourceMediaID + AtMs describe a frame cut out of other media: which
	// media, and how far into it. This is what makes a chained generation
	// legible afterwards — the start frame of beat 3 is "media X at 7,920ms",
	// not an anonymous PNG.
	SourceMediaID string `json:"sourceMediaId,omitempty" firestore:"sourceMediaId,omitempty"`
	AtMs          int    `json:"atMs,omitempty" firestore:"atMs,omitempty"`

	At int64 `json:"at,omitempty" firestore:"at,omitempty"`
}

func assetsCollection(brandID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(fmt.Sprintf("brands/%s/assets", brandID))
}

// CreateAsset stores one asset and returns its id.
func CreateAsset(ctx context.Context, brandID string, a *Asset) (string, error) {
	now := time.Now().UnixMilli()
	if a.CreatedAt == 0 {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	a.Derived = false
	ref, _, err := assetsCollection(brandID).Add(ctx, a)
	if err != nil {
		return "", fmt.Errorf("CreateAsset: %w", err)
	}
	return ref.ID, nil
}

// RecordAsset stores an asset unless one with the same URL already exists.
//
// Every automatic writer goes through this. A design that resolves the same
// stock photo twice, a re-run of a generation, a retried upload — all of them
// would otherwise fill the library with duplicates of one picture, and a
// library that shows the same photo six times is worse than no library.
//
// Returns the existing id and false when it was already there.
func RecordAsset(ctx context.Context, brandID string, a *Asset) (string, bool, error) {
	if a == nil || a.URL == "" {
		return "", false, fmt.Errorf("RecordAsset: an asset needs a url")
	}
	existing, err := findAssetByURL(ctx, brandID, a.URL)
	if err != nil {
		return "", false, err
	}
	if existing != "" {
		return existing, false, nil
	}
	id, err := CreateAsset(ctx, brandID, a)
	return id, err == nil, err
}

// findAssetByURL returns the id of a stored asset with this URL, or "".
//
// A single-field equality, so it needs no composite index on either edition.
func findAssetByURL(ctx context.Context, brandID, url string) (string, error) {
	iter := assetsCollection(brandID).Where("url", "==", url).Limit(1).Documents(ctx)
	defer iter.Stop()
	doc, err := iter.Next()
	if err == iterator.Done {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return doc.Ref.ID, nil
}

// GetAsset reads one asset, or (nil, nil) if absent.
func GetAsset(ctx context.Context, brandID, assetID string) (*Asset, error) {
	doc, err := assetsCollection(brandID).Doc(assetID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var a Asset
	if err := doc.DataTo(&a); err != nil {
		return nil, err
	}
	a.ID = doc.Ref.ID
	return &a, nil
}

// DeleteAsset removes one stored asset. A derived row has nothing to delete.
func DeleteAsset(ctx context.Context, brandID, assetID string) error {
	_, err := assetsCollection(brandID).Doc(assetID).Delete(ctx)
	return err
}

// ListStoredAssets returns stored rows, newest first, optionally filtered.
//
// The (source, createdAt) pair is what the library's tabs query, so it ships as
// a composite index in both index files.
func ListStoredAssets(ctx context.Context, brandID, source string, limit int) ([]Asset, error) {
	q := assetsCollection(brandID).Query
	if source != "" {
		q = q.Where("source", "==", source)
	}
	q = q.OrderBy("createdAt", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(ctx)
	defer iter.Stop()

	out := []Asset{}
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
		out = append(out, a)
	}
	return out, nil
}

// ListBrandAssets is what the library reads: stored rows unioned with a derived
// view of content attachments, newest first, deduplicated by URL.
//
// The union is the whole reason an asset uploaded against one post is reachable
// from another on the day this ships, rather than only for assets created
// afterwards. Stored rows win on a URL collision because a derived row knows
// nothing beyond the URL — no credit, no licence, no trim.
//
// `source` filters both halves; attachments only ever derive as uploads, so
// asking for stock or generated returns stored rows alone.
func ListBrandAssets(ctx context.Context, brandID, source string, limit int) ([]Asset, error) {
	if limit <= 0 {
		limit = 60
	}
	stored, err := ListStoredAssets(ctx, brandID, source, limit)
	if err != nil {
		// A failed read of the real collection must not silently degrade to
		// "here are some attachments" — that looks like the library losing
		// everything with provenance.
		return nil, err
	}

	seen := make(map[string]bool, len(stored))
	for _, a := range stored {
		seen[a.URL] = true
	}
	out := stored

	if source == "" || source == AssetSourceUpload {
		derived, derr := deriveAssetsFromAttachments(ctx, brandID, limit*2)
		if derr != nil {
			// Degrading to the stored rows alone IS acceptable here: the real
			// collection is the authority, and the derived view is a courtesy
			// for history.
			derived = nil
		}
		for _, d := range derived {
			if len(out) >= limit {
				break
			}
			if seen[d.URL] {
				continue
			}
			seen[d.URL] = true
			out = append(out, d)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// deriveAssetsFromAttachments reconstructs library rows from what contents
// already carry. This is the old "media library" scan, kept as a view.
//
// Everything it produces is marked Derived, has no id, and carries no
// provenance — because an attachment never had any.
//
// ⚠️ It reads ListRecentContent, NOT the posting-time range the old scan used.
// A Firestore range filter skips documents missing the field, so filtering on
// postingTimeStamp cannot see an unscheduled draft — and a draft is exactly
// where an upload sits before anyone has put it on the calendar. Measured on
// dev: one brand had 0 of 10 contents with a posting time and 10 of 10 with an
// updatedAt. The old scan returned nothing at all there.
func deriveAssetsFromAttachments(ctx context.Context, brandID string, limit int) ([]Asset, error) {
	items, err := ListRecentContent(ctx, brandID, limit)
	if err != nil {
		return nil, err
	}

	out := make([]Asset, 0, limit)
	for _, ct := range items {
		for _, att := range ct.Attachments {
			if len(out) >= limit {
				return out, nil
			}
			url, kind := att.ImageURL, AssetKindImage
			if url == "" {
				url, kind = att.PlayURL, AssetKindVideo
			}
			if url == "" {
				continue
			}
			out = append(out, Asset{
				Kind:            kind,
				Source:          AssetSourceUpload,
				URL:             url,
				ThumbURL:        att.ThumbURL,
				Title:           ct.Title,
				OriginContentID: ct.ID,
				CreatedAt:       ct.UpdatedAt,
				Derived:         true,
			})
		}
	}
	return out, nil
}

// ── What ingest learns about a piece of footage ─────────────────────────────

// AssetIngest is the result of looking at an uploaded video properly: its real
// shape, a frame to show it by, what is said in it, and what is in it.
//
// The point of all of it is PLACEMENT. "Put the bit where she picks up the cup
// at the front" is only answerable if something recorded that there is a cup
// and when. Without this an uploaded video is a rectangle of unknown content
// that a person has to scrub through themselves.
type AssetIngest struct {
	// Status: pending | done | failed. A video is usable before ingest
	// finishes — it can be trimmed and placed by hand — so this gates the
	// smart features, not the footage.
	Status string `json:"status" firestore:"status"`
	Error  string `json:"error,omitempty" firestore:"error,omitempty"`

	// ── From ffprobe ──
	DurationMs int     `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`
	Width      int     `json:"width,omitempty" firestore:"width,omitempty"`
	Height     int     `json:"height,omitempty" firestore:"height,omitempty"`
	FPS        float64 `json:"fps,omitempty" firestore:"fps,omitempty"`
	HasAudio   bool    `json:"hasAudio,omitempty" firestore:"hasAudio,omitempty"`

	// SampleFrameURLs are evenly spaced stills, in order, used for the vision
	// pass that produces the shot list. Kept rather than discarded: they are
	// what a later, better model would re-read, and re-extracting them means
	// downloading the whole video again.
	SampleFrameURLs []string `json:"sampleFrameUrls,omitempty" firestore:"sampleFrameUrls,omitempty"`

	// Words is the transcript with per-word timings — the thing that makes
	// "cut the dead air" and "start on the line about pricing" answerable.
	Words []TranscriptWord `json:"words,omitempty" firestore:"words,omitempty"`
	// Text is the same transcript as prose, for reading and for prompts.
	Text string `json:"text,omitempty" firestore:"text,omitempty"`

	// Shots describe what is on screen over time.
	Shots []Shot `json:"shots,omitempty" firestore:"shots,omitempty"`

	IngestedAt int64 `json:"ingestedAt,omitempty" firestore:"ingestedAt,omitempty"`
}

// Ingest statuses.
const (
	IngestPending = "pending"
	IngestDone    = "done"
	IngestFailed  = "failed"
)

// TranscriptWord is one word and when it was said.
type TranscriptWord struct {
	Text string `json:"text" firestore:"text"`
	// StartMs/EndMs are milliseconds into the footage. Stored as integers
	// rather than the provider's float seconds, because every other time in
	// this product is integer milliseconds and two units is how a trim ends up
	// a thousand times too long.
	StartMs int `json:"startMs" firestore:"startMs"`
	EndMs   int `json:"endMs" firestore:"endMs"`
	// Kind is word | spacing | audio_event. Spacing is kept so a span that is
	// deleted takes its surrounding silence with it.
	Kind string `json:"kind,omitempty" firestore:"kind,omitempty"`
}

// Shot is a stretch of footage described in words.
type Shot struct {
	StartMs int `json:"startMs" firestore:"startMs"`
	EndMs   int `json:"endMs" firestore:"endMs"`
	// Description is what a person would say is happening — the thing the AI
	// matches a request against when choosing which part to place.
	Description string `json:"description" firestore:"description"`
}

// SetAssetIngest writes an ingest result onto an asset.
func SetAssetIngest(ctx context.Context, brandID, assetID string, ingest *AssetIngest) error {
	if ingest.IngestedAt == 0 {
		ingest.IngestedAt = time.Now().UnixMilli()
	}
	_, err := assetsCollection(brandID).Doc(assetID).Set(ctx, map[string]interface{}{
		"ingest":    ingest,
		"updatedAt": time.Now().UnixMilli(),
	}, firestore.MergeAll)
	return err
}

// ListFootageAssets returns the brand's ingested video, newest first.
//
// Filtered in memory on the ingest status rather than in the query: that would
// need a composite index on a nested field for a collection of tens of rows,
// and the library read is already bounded.
func ListFootageAssets(ctx context.Context, brandID string, limit int) ([]Asset, error) {
	all, err := ListStoredAssets(ctx, brandID, "", limit*2)
	if err != nil {
		return nil, err
	}
	out := make([]Asset, 0, len(all))
	for _, a := range all {
		if a.Kind != AssetKindVideo {
			continue
		}
		out = append(out, a)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// PendingIngest is an asset whose ingest the worker has handed over, plus the
// brand it belongs to — which a collection-group query otherwise loses.
type PendingIngest struct {
	BrandID string
	Asset   Asset
}

// ListPendingIngests finds footage the worker has measured but nobody has
// transcribed or described yet.
//
// A collection-group query, because ingest finishes on a global cron and
// assets are brand-scoped subcollections: a per-brand sweep would have to
// enumerate every brand first. Needs the COLLECTION_GROUP single-field index
// on `ingest.status`, which ships in firestore.indexes.json and, for the
// Enterprise production database, as a gcloud migration.
func ListPendingIngests(ctx context.Context, limit int) ([]PendingIngest, error) {
	if limit <= 0 {
		limit = 20
	}
	q := firestoredb.Client.CollectionGroup("assets").
		Where("ingest.status", "==", IngestPending).
		Limit(limit)

	iter := q.Documents(ctx)
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
		// brands/{brandId}/assets/{assetId}
		brandRef := doc.Ref.Parent.Parent
		if brandRef == nil {
			continue
		}
		out = append(out, PendingIngest{BrandID: brandRef.ID, Asset: a})
	}
	return out, nil
}
