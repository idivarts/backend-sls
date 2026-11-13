// Package mediastore is the ONE place bytes may reach S3.
//
// The rule it exists to enforce: nothing goes to the bucket unregistered. Not a
// logo, not a generated frame nobody ended up using, not a render that failed
// review. Every object gets a row in the brand's media library at the moment it
// is created, whether anything references it or not.
//
// Three things depend on that being true, and none of them can be bolted on
// afterwards:
//
//   - PICKING. "Use the still I made last week as the start frame" is a pick
//     from a list, not a download-and-reupload, only if last week's still was
//     filed when it was made.
//   - DELETION. Bytes can only be removed safely if something counted the
//     references, and a reference count can only be trusted if every object
//     has a row to count against.
//   - MEASUREMENT. A per-brand byte total assembled later can only say "since
//     we started counting". Started with the first object, it is true.
//
// ⚠️ A pre-signed upload is registered BEFORE its bytes exist. The backend hands
// out a URL and never sees the transfer, so there is no moment afterwards at
// which it could notice. The row goes in as `pending` at hand-out, and Confirm
// HEADs the object and promotes it. The size on the row is therefore S3's
// number, never the client's — it is meant to be billable, and a number the
// uploader reports is a number the uploader can understate.
//
// ⚠️ DO NOT call mys3.Client.PutObject outside this package. A CI audit fails
// the build on it (scripts/audit-s3-writes), because every bypass is an object
// nothing knows about, and the library stops being a history the moment one of
// them exists.
package mediastore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mys3"
)

// Target says which bucket and prefix an object belongs in.
//
// Two exist because the video bucket feeds a transcoding pipeline keyed on its
// `raw_videos/` prefix, while everything else — images, generated clips,
// renders, extracted frames — lives in the image bucket under `uploads/`,
// fronted by the CDN the apps already read. Generated VIDEO deliberately uses
// the image target: it is already encoded and must not be re-transcoded.
type Target string

const (
	// TargetImage is the general bucket: images, generated clips, renders,
	// frames, audio. The apps' own pre-signed uploads use it.
	TargetImage Target = "image"
	// TargetVideo is the raw-upload bucket whose prefix triggers transcoding.
	// Only a user's own camera footage should go here.
	TargetVideo Target = "video"
	// TargetAttachment is the attachment bucket. Contract and chat files live
	// here, and so — for historical reasons that are not worth a migration —
	// do generated audio and Canva exports, each under its own prefix.
	//
	// Whether a row is filed is therefore NOT decided by the bucket: a signed
	// agreement and a generated voiceover sit in the same one. The caller says
	// so with NoLibrary.
	TargetAttachment Target = "attachment"
)

// Spec describes one object to store and the row to file for it.
type Spec struct {
	BrandID string
	Target  Target

	// Filename is used for the extension and nothing else; the stored key is
	// always generated, so two users uploading "video.mp4" cannot collide.
	Filename    string
	ContentType string

	// Prefix overrides the target's default key prefix. Set only to preserve
	// an address shape that already exists in the wild — generated audio has
	// always lived under `audio/` and the apps play those URLs directly.
	Prefix string

	// ExactKey stores the object at this key verbatim, skipping the generated
	// one. Needed only for a file whose PATH IS PART OF ITS MEANING.
	//
	// The generated key exists so two people uploading "video.mp4" cannot
	// collide, which is right for media. A Design Studio v3 project is not
	// media: it is a directory the Hyperframes CLI reads, where
	// `compositions/hook.html` is mounted by that exact relative path from
	// index.html. Rename it and the film loses a scene.
	//
	// Callers own uniqueness when they set this. The design's own id is in
	// the prefix, so two designs cannot collide, and re-authoring a design
	// deliberately overwrites its own files.
	ExactKey string

	// NoLibrary stores the bytes without filing a library row. The ONE
	// legitimate use is a file that is not the brand's media at all: a
	// contract PDF, a chat attachment. Everything else files a row, because
	// an object with no row is an object nothing can ever find, count or
	// remove.
	NoLibrary bool

	// Kind is the library's kind: image | video | audio.
	Kind string
	// Source is the library's tab: upload | stock | generated | brand.
	Source string

	// Origin is what produced this. Required for anything the backend makes:
	// a row whose provenance is unknown is a row the AI cannot reuse and a
	// person cannot account for.
	Origin *trendlymodels.AssetOrigin

	Title      string
	Alt        string
	Width      int
	Height     int
	DurationMs int

	ContentID string
	CreatedBy string
}

// Stored is what a caller gets back: where the bytes are, and the library row
// that now points at them.
type Stored struct {
	MediaID  string
	URL      string
	Bucket   string
	Key      string
	SizeByte int64
}

// Presigned is a hand-out: a URL the client PUTs to, the public URL the object
// will have, and the pending row to confirm afterwards.
type Presigned struct {
	UploadURL string
	URL       string
	MediaID   string
	Bucket    string
	Key       string
	// AppleURL/PlayURL are the video pipeline's two addresses, kept because
	// the apps' existing upload callers read them by name.
	AppleURL string
	PlayURL  string
}

// PresignUpload issues a pre-signed PUT and files a pending row for it.
//
// The row is what makes a user's own upload visible to the library at all: the
// transfer goes from their device straight to the bucket, so this hand-out is
// the only moment the backend is involved.
func PresignUpload(ctx context.Context, sp Spec) (*Presigned, error) {
	bucket, cdn, prefix, err := resolveTarget(sp.Target)
	if err != nil {
		return nil, err
	}
	key := sp.ExactKey
	if key == "" {
		key = buildKey(orDefault(sp.Prefix, prefix), sp.Filename)
	}
	url := cdn + "/" + key

	req, _ := mys3.Client.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	uploadURL, err := req.Presign(15 * time.Minute)
	if err != nil {
		return nil, fmt.Errorf("mediastore: could not sign the upload: %w", err)
	}

	out := &Presigned{
		UploadURL: uploadURL,
		URL:       url,
		Bucket:    bucket,
		Key:       key,
		AppleURL:  url,
		PlayURL:   url,
	}

	// No brand means no library — the influencer app's uploads, for one, have
	// no brand to file against. The hand-out still works; it just has no row.
	if sp.BrandID == "" || sp.NoLibrary {
		return out, nil
	}

	origin := sp.Origin
	if origin == nil {
		origin = &trendlymodels.AssetOrigin{Kind: trendlymodels.AssetOriginUpload}
	}
	id, err := trendlymodels.RegisterAsset(ctx, sp.BrandID, &trendlymodels.Asset{
		Kind:            orDefault(sp.Kind, kindFromFilename(sp.Filename)),
		Source:          orDefault(sp.Source, trendlymodels.AssetSourceUpload),
		URL:             url,
		S3Bucket:        bucket,
		S3Key:           key,
		Status:          trendlymodels.AssetStatusPending,
		Origin:          origin,
		Title:           sp.Title,
		Alt:             sp.Alt,
		MimeType:        orDefault(sp.ContentType, mimeFromFilename(sp.Filename)),
		OriginContentID: sp.ContentID,
		CreatedBy:       sp.CreatedBy,
	})
	if err != nil {
		// A library row that could not be written must not block the upload.
		// The sweep that reconciles the bucket against the library is the
		// backstop; refusing the upload would be a worse trade for the user.
		return out, nil
	}
	out.MediaID = id
	return out, nil
}

// Confirm promotes a pending row once its bytes have landed, measuring the
// object rather than believing the client.
func Confirm(ctx context.Context, brandID, mediaID string, extra trendlymodels.AssetMeasurement) error {
	a, err := trendlymodels.GetAsset(ctx, brandID, mediaID)
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("mediastore: %s is not in the library", mediaID)
	}
	if a.S3Bucket != "" && a.S3Key != "" {
		head, herr := mys3.Client.HeadObject(&s3.HeadObjectInput{
			Bucket: aws.String(a.S3Bucket),
			Key:    aws.String(a.S3Key),
		})
		if herr != nil {
			// The object is not there. The upload failed, was cancelled, or
			// the client never started it — all of which are the same state
			// as far as the library is concerned.
			return trendlymodels.MarkAssetFailed(ctx, brandID, mediaID, "the upload did not complete")
		}
		if head.ContentLength != nil {
			extra.SizeBytes = *head.ContentLength
		}
		if extra.MimeType == "" && head.ContentType != nil {
			extra.MimeType = *head.ContentType
		}
	}
	return trendlymodels.ConfirmAsset(ctx, brandID, mediaID, extra)
}

// Put writes bytes and files a ready row for them in one step.
//
// What everything the backend produces itself uses: a generated clip hosted off
// a provider's expiring URL, a finished render, a frame cut out of a video, a
// voiceover. The row is ready immediately because, unlike a pre-signed upload,
// the bytes are in hand.
func Put(ctx context.Context, sp Spec, data []byte) (*Stored, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("mediastore: nothing to store")
	}
	bucket, cdn, prefix, err := resolveTarget(sp.Target)
	if err != nil {
		return nil, err
	}
	key := sp.ExactKey
	if key == "" {
		key = buildKey(orDefault(sp.Prefix, prefix), sp.Filename)
	}
	contentType := orDefault(sp.ContentType, mimeFromFilename(sp.Filename))

	if _, err := mys3.Client.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}); err != nil {
		return nil, fmt.Errorf("mediastore: could not store the object: %w", err)
	}

	out := &Stored{
		URL:      cdn + "/" + key,
		Bucket:   bucket,
		Key:      key,
		SizeByte: int64(len(data)),
	}
	if sp.BrandID == "" || sp.NoLibrary {
		return out, nil
	}

	origin := sp.Origin
	if origin == nil {
		// Deliberately not defaulted to "upload": something the backend wrote
		// and could not describe is a gap worth seeing in the library rather
		// than a row that claims a person chose it.
		origin = &trendlymodels.AssetOrigin{Kind: trendlymodels.AssetOriginHosted}
	}
	id, err := trendlymodels.RegisterAsset(ctx, sp.BrandID, &trendlymodels.Asset{
		Kind:            orDefault(sp.Kind, kindFromFilename(sp.Filename)),
		Source:          orDefault(sp.Source, trendlymodels.AssetSourceGenerated),
		URL:             out.URL,
		S3Bucket:        bucket,
		S3Key:           key,
		SizeBytes:       out.SizeByte,
		Status:          trendlymodels.AssetStatusReady,
		Origin:          origin,
		SourceMediaID:   origin.SourceMediaID,
		Title:           sp.Title,
		Alt:             sp.Alt,
		Width:           sp.Width,
		Height:          sp.Height,
		DurationMs:      sp.DurationMs,
		MimeType:        contentType,
		OriginContentID: sp.ContentID,
		CreatedBy:       sp.CreatedBy,
		Provider:        origin.Provider,
	})
	if err != nil {
		return out, fmt.Errorf("mediastore: stored the object but could not file it: %w", err)
	}
	out.MediaID = id
	return out, nil
}

// HostFromURL copies somebody else's object into our bucket and files it.
//
// Not optional for anything generated. Providers hand back SIGNED urls that
// expire about an hour out, and a timeline is a document people re-render weeks
// later — a scene pointing at an expired url is a scene nobody can render
// again.
func HostFromURL(ctx context.Context, sp Spec, url string) (*Stored, error) {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mediastore: the provider returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if sp.ContentType == "" {
		sp.ContentType = resp.Header.Get("Content-Type")
	}
	if sp.Filename == "" {
		sp.Filename = filenameFor(sp.Kind, sp.ContentType)
	}
	return Put(ctx, sp, data)
}

// downloadClient is generous on time: a finished clip is tens of megabytes and
// arrives from the provider's CDN, not ours.
var downloadClient = &http.Client{Timeout: 5 * time.Minute}

// resolveTarget returns the bucket, CDN host and key prefix for a target.
func resolveTarget(t Target) (bucket, cdn, prefix string, err error) {
	switch t {
	case TargetVideo:
		bucket = os.Getenv("VIDEO_S3_BUCKET_NAME")
		cdn = strings.TrimRight(os.Getenv("CLOUDFRONT_DISTRIBUTION_URL"), "/")
		prefix = "raw_videos"
	case TargetAttachment:
		bucket = os.Getenv("ATTACHMENT_S3_BUCKET_NAME")
		cdn = strings.TrimRight(os.Getenv("ATTACHMENT_CF_DISTRIBUTION_URL"), "/")
		prefix = "uploads"
	case TargetImage, "":
		bucket = os.Getenv("IMAGE_S3_BUCKET_NAME")
		cdn = strings.TrimRight(os.Getenv("IMAGE_CF_DISTRIBUTION_URL"), "/")
		prefix = "uploads"
	default:
		return "", "", "", fmt.Errorf("mediastore: unknown target %q", t)
	}
	if bucket == "" || cdn == "" {
		return "", "", "", fmt.Errorf("mediastore: the %s bucket is not configured", t)
	}
	return bucket, cdn, prefix, nil
}

// buildKey generates the object key.
//
// Always generated, never the supplied name: the old handlers prefixed a
// timestamp, which two people uploading the same filename in the same second
// still collide on. A uuid cannot.
func buildKey(prefix, filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".bin"
	}
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = sanitize(base)
	if base == "" {
		base = "media"
	}
	if len(base) > 48 {
		base = base[:48]
	}
	return fmt.Sprintf("%s/file_%d_%s_%s%s", prefix, time.Now().Unix(), base, uuid.NewString()[:8], ext)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func mimeFromFilename(filename string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); t != "" {
		return t
	}
	return "application/octet-stream"
}

// kindFromFilename maps an extension onto the library's kinds, so a caller that
// knows only a filename still files the row under the right tab.
func kindFromFilename(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4", ".mov", ".webm", ".mkv", ".avi", ".m4v", ".mpeg", ".flv", ".wmv":
		return trendlymodels.AssetKindVideo
	case ".mp3", ".wav", ".m4a", ".aac", ".ogg", ".flac":
		return trendlymodels.AssetKindAudio
	default:
		return trendlymodels.AssetKindImage
	}
}

func filenameFor(kind, contentType string) string {
	ext := ".bin"
	switch {
	case strings.Contains(contentType, "webm"):
		ext = ".webm"
	case strings.Contains(contentType, "quicktime"):
		ext = ".mov"
	case strings.Contains(contentType, "mp4"):
		ext = ".mp4"
	case strings.Contains(contentType, "png"):
		ext = ".png"
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		ext = ".jpg"
	case strings.Contains(contentType, "webp"):
		ext = ".webp"
	case strings.Contains(contentType, "mpeg"), strings.Contains(contentType, "mp3"):
		ext = ".mp3"
	default:
		switch kind {
		case trendlymodels.AssetKindVideo:
			ext = ".mp4"
		case trendlymodels.AssetKindAudio:
			ext = ".mp3"
		case trendlymodels.AssetKindImage:
			ext = ".png"
		}
	}
	return kind + ext
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
