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
	"sort"
	"strconv"
	"strings"

	"github.com/idivarts/backend-sls/pkg/mediastore"
)

// File is one file of a project, at its path relative to the project root.
type File struct {
	Path    string
	Content string
}

// Scene is one clip the root composition mounts: a piece of the film with a
// place on the timeline.
//
// Extracted here, at authoring time, because the APP needs it and nothing else
// can give it. The project lives in S3 as HTML; the app would otherwise have
// to fetch and parse several documents just to draw a timeline, and would then
// be a second implementation of this parsing that could disagree with the
// renderer's.
type Scene struct {
	ID         string `json:"id" firestore:"id"`
	Src        string `json:"src" firestore:"src"`
	StartMs    int    `json:"startMs" firestore:"startMs"`
	DurationMs int    `json:"durationMs" firestore:"durationMs"`
	// TrackIndex and TrackKind are Hyperframes' own layout of the timeline —
	// which row a clip sits on and what it carries. Passed through rather
	// than interpreted, so the app draws what the renderer will render.
	TrackIndex int    `json:"trackIndex" firestore:"trackIndex"`
	TrackKind  string `json:"trackKind,omitempty" firestore:"trackKind,omitempty"`
}

// Project is a complete, uploadable project.
type Project struct {
	Files []File
	// DurationMs is read off the root composition's data-duration. Carried
	// out of here because the WALLET needs it before a worker ever opens the
	// project — a charge that can only be computed after the render has
	// started is not a gate.
	DurationMs int
	// Scenes are the clips the root mounts, in timeline order.
	Scenes []Scene
}

var (
	rootDurationRe = regexp.MustCompile(`data-duration\s*=\s*["']([0-9.]+)["']`)
	compositionSrc = regexp.MustCompile(`data-composition-src\s*=\s*["']([^"']+)["']`)
	// A whole opening tag that declares a scene. Matched as a unit so the
	// attributes read off it belong to THAT element — scanning the document
	// for `data-start` instead would happily pair one scene's start with
	// another's duration.
	//
	// Two shapes, because both exist in the wild:
	//   data-scene="…"            the single-document form the contract asks
	//                             for now, and the only one that renders
	//   data-composition-src="…"  the old mounted-sub-composition form, kept
	//                             so a design authored before 2026-11-13
	//                             still draws its timeline rather than
	//                             silently losing it
	sceneTagRe  = regexp.MustCompile(`<[a-zA-Z][^>]*\sdata-scene\s*=\s*["'][^"']+["'][^>]*>`)
	legacyTagRe = regexp.MustCompile(`<[a-zA-Z][^>]*\sdata-composition-src\s*=\s*["'][^"']+["'][^>]*>`)
	sceneAttrRe = attrRe(`data-scene`)
	attrRe      = func(name string) *regexp.Regexp {
		return regexp.MustCompile(name + `\s*=\s*["']([^"']*)["']`)
	}
	sceneIDRe     = attrRe(`data-composition-id`)
	sceneSrcRe    = attrRe(`data-composition-src`)
	sceneStartRe  = attrRe(`data-start`)
	sceneDurRe    = attrRe(`data-duration`)
	sceneTrackRe  = attrRe(`data-track-index`)
	sceneKindRe   = attrRe(`data-track-kind`)
	sceneFallback = attrRe(`id`)
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

	// ⚠️ A MOUNTED SUB-COMPOSITION IS REFUSED, not merely discouraged.
	//
	// It passes `check`, renders without a warning, and produces a film of
	// unstyled text — the single most expensive failure this project has had,
	// because every signal says the design is fine. Measured against the same
	// project in the same image: `play` and `snapshot` are correct and
	// `render` is not, because render is forced into screenshot capture
	// inside Docker with no GPU and sub-composition mounting does not survive
	// that path. A single-document project renders correctly there.
	//
	// So it fails HERE, at authoring, where the message reaches the model and
	// costs nothing, rather than after a charged render. The wording is aimed
	// at the model: this error comes back as a tool result and the next turn
	// is its attempt to fix it.
	if m := compositionSrc.FindStringSubmatch(index); m != nil {
		return nil, fmt.Errorf(
			"index.html mounts %q with data-composition-src. Mounted sub-compositions do not render here: "+
				"put every scene in index.html as a <section class=\"clip\" data-scene=\"…\" data-start=\"…\" "+
				"data-duration=\"…\">, with all the styles in one <style> in <head>, and write no files under "+
				"compositions/", m[1])
	}

	durationMs, err := rootDurationMs(index)
	if err != nil {
		return nil, err
	}

	return &Project{Files: files, DurationMs: durationMs, Scenes: parseScenes(index)}, nil
}

// parseScenes reads the clips the root composition mounts.
//
// Deliberately forgiving: a project that renders but whose scene attributes
// are odd must still render. Everything here feeds a TIMELINE DRAWING, not the
// render and not the price, so a missing attribute costs the user a less
// precise picture rather than their film. Build has already refused the
// project if a mounted file is absent, which is the failure that matters.
func parseScenes(index string) []Scene {
	tags := sceneTagRe.FindAllString(index, -1)
	legacy := false
	if len(tags) == 0 {
		// Nothing in the current shape. A design written before the contract
		// moved to one document still has mounted sub-compositions, and its
		// timeline is worth drawing even though it can no longer render.
		tags = legacyTagRe.FindAllString(index, -1)
		legacy = true
	}
	scenes := make([]Scene, 0, len(tags))

	for _, tag := range tags {
		id := first(sceneAttrRe, tag)
		src := first(sceneSrcRe, tag)
		if legacy {
			if src == "" {
				continue
			}
			if id = first(sceneIDRe, tag); id == "" {
				// Hyperframes keys a composition by data-composition-id; the
				// element id is the next best handle, and src always exists.
				if id = first(sceneFallback, tag); id == "" {
					id = src
				}
			}
		} else if id == "" {
			continue
		}
		scenes = append(scenes, Scene{
			ID:         id,
			Src:        src,
			StartMs:    secondsToMs(first(sceneStartRe, tag)),
			DurationMs: secondsToMs(first(sceneDurRe, tag)),
			TrackIndex: atoiOr(first(sceneTrackRe, tag), 0),
			TrackKind:  first(sceneKindRe, tag),
		})
	}

	// Timeline order, not document order. They usually agree, but a project
	// that mounts its scenes out of order would otherwise draw a timeline
	// that runs backwards.
	sort.SliceStable(scenes, func(i, j int) bool { return scenes[i].StartMs < scenes[j].StartMs })
	return scenes
}

func first(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// secondsToMs converts a data-* time, which Hyperframes writes in SECONDS.
// Milliseconds everywhere on our side, because every other time on a design
// document is milliseconds and a mixed unit is how a timeline ends up with
// clips a thousand times too long.
func secondsToMs(v string) int {
	if v == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(v, 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return int(seconds * 1000)
}

func atoiOr(v string, fallback int) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
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
			// Every file here is MUTABLE at a fixed URL: the worker rewrites
			// index.html when a film's media resolves. Without this header
			// CloudFront served the pre-media film for hours while S3 had
			// the right one — the render had the photograph and the player
			// did not. no-cache still caches; it just revalidates first, so
			// an unchanged film costs a 304.
			CacheControl: projectCacheControl,
		}, []byte(f.Content))
		if err != nil {
			return "", fmt.Errorf("hfproject.Upload: %s: %w", clean, err)
		}
	}
	return prefix, nil
}

// projectCacheControl is what every project object is stored with.
//
// "no-cache" does not mean "do not cache": it means revalidate before use.
// An unchanged film costs a conditional request and a 304, and a changed one
// is never served stale. That is the right trade for a document whose URL
// stays the same while its contents are rewritten underneath it.
const projectCacheControl = "no-cache"

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
	// A brand's logo and its fonts travel with the project (see the design
	// package's brandkit.go). CloudFront serves what we set here, and a
	// woff2 served as octet-stream is a font the browser declines to use.
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	default:
		return "application/octet-stream"
	}
}
