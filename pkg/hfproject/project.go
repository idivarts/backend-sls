// Package hfproject assembles a Design Studio v3 project and puts it where
// the render worker can find it.
//
// A Hyperframes project is a DIRECTORY, not a document:
//
//	index.html              the root composition — variables, and one host per scene
//	compositions/*.html     one scene each, everything inside <template>
//	assets/…                media and fonts, referenced as assets/… with no leading slash
//
// The worker materialises it from S3 into a temp dir and points the CLI at
// it. So the relative paths are load-bearing: index.html mounts a scene by
// `data-composition-src="compositions/hook.html"`, and a renamed file is a
// missing scene rather than an error.
//
// ── What this package will NOT do ──────────────────────────────────────────
// It does not check the project. That is `hyperframes check --json`, which
// runs in the worker because only there is there a CLI and a browser — and it
// audits things no amount of Go string-inspection could: contrast, layout,
// safe areas, whether anything actually moves. The validation here is the
// narrow set of structural facts that would otherwise produce a confusing
// CLI error instead of a clear one.
package hfproject

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/idivarts/backend-sls/pkg/mediastore"
)

// File is one file of a project, at its path relative to the project root.
type File struct {
	Path    string
	Content string
}

// Project is a complete, uploadable project.
type Project struct {
	Files []File
	// DurationMs is read off the root composition's data-duration. Carried
	// out of here because the WALLET needs it before a worker ever opens the
	// project — a charge that can only be computed after the render has
	// started is not a gate.
	DurationMs int
}

var (
	rootDurationRe = regexp.MustCompile(`data-duration\s*=\s*["']([0-9.]+)["']`)
	compositionSrc = regexp.MustCompile(`data-composition-src\s*=\s*["']([^"']+)["']`)
)

// MaxFiles and MaxBytes bound what one authoring turn may write. A model that
// loops can otherwise fill a bucket, and the failure would look like a slow
// render rather than what it is.
const (
	MaxFiles = 40
	MaxBytes = 4 << 20 // 4 MiB of source; media does not travel this way
)

// Build validates the files as a project and extracts what the caller needs
// before rendering.
//
// Every error here is phrased for the MODEL, not for a developer: these come
// back as a tool result, and the next turn is the model's attempt to fix it.
func Build(files []File) (*Project, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("the project has no files; it needs at least index.html")
	}
	if len(files) > MaxFiles {
		return nil, fmt.Errorf("the project has %d files, which is more than the %d allowed", len(files), MaxFiles)
	}

	total := 0
	seen := map[string]bool{}
	var index string

	for _, f := range files {
		clean, err := cleanPath(f.Path)
		if err != nil {
			return nil, err
		}
		if seen[clean] {
			return nil, fmt.Errorf("the project lists %q twice", clean)
		}
		seen[clean] = true
		total += len(f.Content)
		if clean == "index.html" {
			index = f.Content
		}
	}

	if total > MaxBytes {
		return nil, fmt.Errorf("the project is %d bytes of source, over the %d limit", total, MaxBytes)
	}
	if index == "" {
		return nil, fmt.Errorf("the project has no index.html; that is the root composition the renderer enters through")
	}

	// Every mounted scene must exist. The CLI's own failure for a missing
	// scene is a 45-second wait and then static frames, which reads as a
	// rendering problem rather than a missing file.
	for _, m := range compositionSrc.FindAllStringSubmatch(index, -1) {
		src, err := cleanPath(m[1])
		if err != nil {
			return nil, fmt.Errorf("index.html mounts %q, which is not a usable path: %w", m[1], err)
		}
		if !seen[src] {
			return nil, fmt.Errorf("index.html mounts %q but the project does not include that file", src)
		}
	}

	durationMs, err := rootDurationMs(index)
	if err != nil {
		return nil, err
	}

	return &Project{Files: files, DurationMs: durationMs}, nil
}

// rootDurationMs reads the film's length off the root composition.
//
// The FIRST data-duration in index.html is the root's: the root element is
// declared before the scene hosts it contains. A scene's own duration is not
// the film's, and using one would charge for — and cut the film to — a
// fraction of its length.
func rootDurationMs(index string) (int, error) {
	m := rootDurationRe.FindStringSubmatch(index)
	if m == nil {
		return 0, fmt.Errorf("index.html has no data-duration on its root composition; that attribute is the film's length")
	}
	seconds, err := strconv.ParseFloat(m[1], 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("index.html has data-duration=%q, which is not a positive number of seconds", m[1])
	}
	return int(seconds * 1000), nil
}

// cleanPath rejects anything that would escape the project directory or
// confuse the CLI, and normalises what is left.
func cleanPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("a file has an empty path")
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%q starts with a slash; project paths are relative, like compositions/hook.html", p)
	}
	if strings.Contains(p, "\\") {
		return "", fmt.Errorf("%q uses backslashes; project paths use forward slashes", p)
	}
	cleaned := path.Clean(p)
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("%q points outside the project", p)
	}
	return cleaned, nil
}

// Prefix is where a design's project lives in the bucket.
//
// Keyed by brand AND design so two designs cannot collide and a brand's
// projects stay together. The render task role is scoped to design-projects/*
// — widening this path means widening that policy.
func Prefix(brandID, designID string) string {
	return fmt.Sprintf("design-projects/%s/%s", brandID, designID)
}

// Upload writes the project to S3 under the design's prefix.
//
// Through mediastore, because that is the one place bytes may reach the
// bucket — with NoLibrary set, since a project file is source, not media. A
// scene file in the brand's media picker would be noise at best, and
// something a person could delete out from under a design at worst.
//
// ExactKey, because the relative paths are the project: index.html mounts a
// scene by `compositions/hook.html` and a generated key would be a missing
// scene.
func Upload(ctx context.Context, brandID, designID string, p *Project) (string, error) {
	if brandID == "" || designID == "" {
		return "", fmt.Errorf("hfproject.Upload: brandID and designID are required")
	}
	if p == nil || len(p.Files) == 0 {
		return "", fmt.Errorf("hfproject.Upload: nothing to upload")
	}
	prefix := Prefix(brandID, designID)

	for _, f := range p.Files {
		clean, err := cleanPath(f.Path)
		if err != nil {
			return "", err
		}
		_, err = mediastore.Put(ctx, mediastore.Spec{
			BrandID:     brandID,
			Target:      mediastore.TargetAttachment,
			ExactKey:    prefix + "/" + clean,
			Filename:    path.Base(clean),
			ContentType: contentTypeFor(clean),
			NoLibrary:   true,
		}, []byte(f.Content))
		if err != nil {
			return "", fmt.Errorf("hfproject.Upload: %s: %w", clean, err)
		}
	}
	return prefix, nil
}

func contentTypeFor(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}
