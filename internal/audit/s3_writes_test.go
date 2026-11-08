package audit

// The audit that makes "nothing reaches S3 unregistered" a build failure rather
// than a convention.
//
// It is a TEST rather than a script so it runs wherever `go test ./...` runs,
// which is already every CI job. A rule enforced by a script somebody remembers
// to run is a rule that holds until the first busy afternoon.
//
// What it looks for: a direct S3 write outside pkg/mediastore. Every one of
// those is an object with no library row — nothing can find it, nothing counts
// a reference to it, and it is invisible to the brand's storage total. The
// library stops being a history the moment one exists.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// s3WriteCalls are the SDK calls that put bytes in a bucket, or hand someone
// else a URL to put bytes in a bucket. A pre-signed PUT counts: the backend
// never sees that transfer, so the hand-out is the only moment it could file a
// row, and a hand-out outside mediastore files none.
var s3WriteCalls = []string{
	"PutObject(",
	"PutObjectRequest(",
	"PutObjectWithContext(",
	"CreateMultipartUpload(",
	"NewUploader(",
	"CopyObject(",
}

// allowed lists the files that may write to S3 directly, each with the reason.
//
// ⚠️ Adding a path here is a decision about the product, not a lint fix. The
// question to answer first is "whose media is this, and where is its row?" —
// and if the answer is a brand's, the file belongs behind mediastore instead.
var allowed = map[string]string{
	// The one true writer.
	"pkg/mediastore/mediastore.go": "the chokepoint itself",

	// Instagram scraping for influencer discovery. These bytes are a scraped
	// profile picture in the discovery bucket — not any brand's media, never
	// offered in a picker, and not chargeable to a brand's storage. It has no
	// brand to file a row against.
	"internal/utilities/scrapping-utility/movetos3.go": "influencer discovery scrape; no brand owns it",
	"internal/utilities/scrapping-utility/s3_test.go":  "test for the above",
}

func TestNoS3WritesOutsideMediastore(t *testing.T) {
	root := repoRoot(t)

	var offenders []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case "node_modules", ".git", "dist", "vendor", "services":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, ok := allowed[rel]; ok {
			return nil
		}
		// This file names the calls it is looking for, in string literals.
		if strings.HasPrefix(rel, "internal/audit/") {
			return nil
		}

		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		src := string(body)
		for _, call := range s3WriteCalls {
			if strings.Contains(src, call) {
				offenders = append(offenders, rel+" calls "+strings.TrimSuffix(call, "("))
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf(`these files write to S3 without going through pkg/mediastore:

  %s

Every object needs a row in the brand's media library, filed in the same step
as the bytes. Use mediastore.Put, mediastore.HostFromURL or
mediastore.PresignUpload. If the bytes genuinely belong to no brand (a contract
attachment, a discovery scrape), pass NoLibrary and add the path to the
allowlist in this file WITH ITS REASON.`, strings.Join(offenders, "\n  "))
	}
}

// TestMediastoreIsTheOnlyWriterOfTheLibrary guards the other half of the rule:
// a library row must not be created except through the registry, because
// RegisterAsset is what also moves the per-brand storage rollup.
//
// CreateAsset and RecordAsset remain for rows that have no bytes of ours at all
// — a hotlinked stock photo, the derived view of old attachments — which is why
// they are not deleted outright.
func TestRegisterAssetIsUsedForStoredBytes(t *testing.T) {
	root := repoRoot(t)
	// pkg/mediastore must be the only caller of RegisterAsset outside the
	// model package itself. If something else calls it, it is storing bytes
	// somewhere mediastore does not know about.
	var callers []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			if info != nil && info.IsDir() {
				switch info.Name() {
				case "node_modules", ".git", "dist", "vendor", "services":
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/models/trendlymodels/") ||
			strings.HasPrefix(rel, "pkg/mediastore/") ||
			strings.HasPrefix(rel, "internal/audit/") {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.Contains(string(body), "RegisterAsset(") {
			callers = append(callers, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	if len(callers) > 0 {
		t.Fatalf(`these files call RegisterAsset directly:

  %s

RegisterAsset also moves the brand's storage rollup, and it is called by
pkg/mediastore in the same step as the bytes. A caller that files a row without
storing bytes through mediastore produces a row pointing at an object nobody
measured.`, strings.Join(callers, "\n  "))
	}
}

// repoRoot walks up from the test's directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find the repository root (no go.mod above the test)")
	return ""
}
