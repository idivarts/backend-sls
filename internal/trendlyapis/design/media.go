package design

// Putting media into a film by hand.
//
// ── How this differs from Phase 9a ───────────────────────────────────────
// 9a resolves the media a film ASKED FOR as it was written: the director
// declares `<img data-media="image" data-media-intent="cold brew over
// ice">` and the worker fetches it. This is choosing media afterwards, for
// a film that already exists — "put a photograph in the second scene",
// "give it music".
//
// ── Why adding media is still a DECLARATION ──────────────────────────────
// `media-use` is a CLI verb that lives in the render image. This endpoint
// runs in a Lambda, which has no Hyperframes and no HeyGen CLI, so it
// cannot fetch anything — and a Lambda that shelled out to a Batch job and
// waited would turn "add a photograph" into a two-minute spinner.
//
// So placing new media writes the same declaration the director would have
// written, and the next preview or render resolves it through exactly the
// Phase 9a path. One resolver, one ledger, one credit limit. The app says
// what will happen rather than pretending it already has.
//
// Placing media the project ALREADY holds is immediate, because there is
// nothing to fetch: it is a file in the directory, and this writes its
// path into the element.

import (
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfedit"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/mediastore"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// The media kinds a film can declare, as `media-use resolve --type` names
// them. Mirrors MEDIA_TYPES in the worker's media.ts; the two must agree,
// and this is the end that validates a request.
var mediaTypes = map[string]bool{
	"image": true, "icon": true, "logo": true, "bgm": true, "sfx": true, "voice": true,
}

// What a declared type becomes in the document.
var mediaTag = map[string]string{
	"image": "img", "icon": "img", "logo": "img",
	"bgm": "audio", "sfx": "audio", "voice": "audio",
}

type placeMediaRequest struct {
	// SceneID is the scene to put it in. Required when adding; the element
	// is appended inside that scene so it inherits its timing.
	SceneID string `json:"sceneId"`

	// ElementID names an existing media element to REPLACE the media on.
	// With it, nothing is added and nothing around it is disturbed.
	ElementID string `json:"elementId"`

	// Type is one of the mediaTypes. Required when adding.
	Type string `json:"type"`

	// Intent is what the media should be, in plain words. Resolved on the
	// next preview or render.
	Intent string `json:"intent"`

	// AssetPath places a file the project already holds — immediately,
	// because there is nothing to fetch.
	AssetPath string `json:"assetPath"`
}

const maxIntent = 300

// ListDesignAssets says what media the project already holds.
// GET /api/design/brands/:brandId/designs/:designId/assets
func ListDesignAssets(c *gin.Context) {
	brandID, designID, ok := guard(c, false)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}
	if d.ProjectPrefix == "" {
		c.JSON(http.StatusOK, gin.H{"assets": []gin.H{}})
		return
	}
	paths, lerr := mediastore.ListPrefix(ctx, mediastore.TargetAttachment, d.ProjectPrefix)
	if lerr != nil {
		mysentry.Capture(lerr, map[string]string{"op": "design.assets.list", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read this film's files"})
		return
	}
	assets := make([]gin.H, 0, len(paths))
	for _, p := range paths {
		kind := assetKind(p)
		if kind == "" {
			continue
		}
		// The URL as well as the path: the panel lists these so they can
		// be OPENED, and a project-relative path is not something a
		// browser can show. Built here rather than in the app, which
		// would have to know the distribution and how a project prefix
		// maps onto it.
		url, uerr := mediastore.PublicURL(mediastore.TargetAttachment, d.ProjectPrefix+"/"+p)
		if uerr != nil {
			// A file with no address is still worth listing — it is in the
			// film either way, and saying so beats hiding it.
			url = ""
		}
		assets = append(assets, gin.H{"path": p, "kind": kind, "name": path.Base(p), "url": url})
	}
	sort.Slice(assets, func(i, j int) bool {
		return assets[i]["path"].(string) < assets[j]["path"].(string)
	})
	c.JSON(http.StatusOK, gin.H{"assets": assets})
}

// PlaceDesignMedia adds media to a scene, or replaces the media on an
// element.
// POST /api/design/brands/:brandId/designs/:designId/media
func PlaceDesignMedia(c *gin.Context) {
	brandID, designID, ok := guard(c, true)
	if !ok {
		return
	}
	var req placeMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}
	if d.ProjectPrefix == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This design has nothing in it yet."})
		return
	}
	source, rerr := readCurrentDocument(ctx, d)
	if rerr != nil {
		mysentry.Capture(rerr, map[string]string{"op": "design.media.read", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read this film"})
		return
	}

	edited, note, perr := applyMediaPlacement(source, req)
	if perr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": perr.Error()})
		return
	}

	out, message, theirs, cerr := commitEditedDocument(ctx, brandID, designID, d, edited,
		trendlymodels.DesignRevisionInspector, note)
	if cerr != nil || out == nil {
		status := http.StatusBadGateway
		body := gin.H{"error": message}
		if theirs {
			status = http.StatusBadRequest
			body = gin.H{"error": "that change would break the film", "message": message}
		}
		c.JSON(status, body)
		return
	}

	// `pending` is what the app shows: a declaration is not media yet, and
	// saying it is would have someone render a film expecting a photograph
	// that has not been fetched.
	c.JSON(http.StatusOK, gin.H{
		"revision":   out.Revision,
		"previewUrl": out.PreviewURL,
		"durationMs": out.DurationMs,
		"pending":    req.AssetPath == "",
	})
}

// applyMediaPlacement works the request into the document and returns the
// line for the revisions list.
func applyMediaPlacement(source string, req placeMediaRequest) (string, string, error) {
	assetPath := strings.TrimSpace(req.AssetPath)
	intent := strings.TrimSpace(req.Intent)

	// ── Replacing what is on an element ──────────────────────────────────
	if req.ElementID != "" {
		el, found := hfedit.FindByID(source, req.ElementID)
		if !found {
			return "", "", fmt.Errorf("that element is no longer in this film")
		}
		if !isMediaTag(el.TagName) {
			return "", "", fmt.Errorf("that is a <%s>, which does not hold media", el.TagName)
		}
		var edits []hfedit.Edit
		if assetPath != "" {
			if err := checkAssetPath(assetPath); err != nil {
				return "", "", err
			}
			e, err := hfedit.SetAttribute(el, source, "src", assetPath)
			if err != nil {
				return "", "", err
			}
			edits = append(edits, e)
			// Whatever it was asked to be, it is now this file. Marking it
			// resolved is what stops the next render fetching the old
			// declaration and overwriting the choice just made.
			r, err := hfedit.SetAttribute(el, source, "data-media-resolved", assetPath)
			if err != nil {
				return "", "", err
			}
			edits = append(edits, r)
			out, aerr := hfedit.Apply(source, edits)
			return out, fmt.Sprintf("%s · %s", req.ElementID, path.Base(assetPath)), aerr
		}
		if intent == "" {
			return "", "", fmt.Errorf("say what it should be, or pick a file")
		}
		if len(intent) > maxIntent {
			return "", "", fmt.Errorf("that description is longer than this box is for")
		}
		// Re-declared: new words, and nothing resolved, so the next
		// preview fetches it.
		for _, set := range [][2]string{
			{"data-media-intent", intent},
			{"src", ""},
			{"data-media-resolved", ""},
		} {
			if set[0] != "data-media-intent" {
				if _, has := el.Attrs[set[0]]; !has {
					continue
				}
			}
			e, err := hfedit.SetAttribute(el, source, set[0], set[1])
			if err != nil {
				return "", "", err
			}
			edits = append(edits, e)
		}
		if _, has := el.Attrs["data-media"]; !has {
			return "", "", fmt.Errorf("that element does not take media it has to fetch")
		}
		out, aerr := hfedit.Apply(source, edits)
		return out, fmt.Sprintf("%s · %s", req.ElementID, shortened(intent, 40)), aerr
	}

	// ── Adding media to a scene ──────────────────────────────────────────
	kind := strings.ToLower(strings.TrimSpace(req.Type))
	if !mediaTypes[kind] {
		return "", "", fmt.Errorf("%q is not a kind of media a film can hold", req.Type)
	}
	if assetPath == "" && intent == "" {
		return "", "", fmt.Errorf("say what it should be, or pick a file")
	}
	if len(intent) > maxIntent {
		return "", "", fmt.Errorf("that description is longer than this box is for")
	}
	if assetPath != "" {
		if err := checkAssetPath(assetPath); err != nil {
			return "", "", err
		}
	}

	scenes, serr := readScenes(source)
	if serr != nil {
		return "", "", serr
	}
	target := scenes[0]
	if req.SceneID != "" {
		found := false
		for _, s := range scenes {
			if s.matches(req.SceneID) {
				target, found = s, true
				break
			}
		}
		if !found {
			return "", "", fmt.Errorf("that scene is no longer in this film")
		}
	}

	id := freeMediaID(source, kind)
	tag := mediaTag[kind]
	var b strings.Builder
	fmt.Fprintf(&b, "\n  <%s id=%q", tag, id)
	// ── Timing, from the scene it goes in ────────────────────────────────
	// A media element with no timing plays from the first frame to the
	// last, which for a photograph dropped into the third scene means it
	// is on screen over the first two as well.
	fmt.Fprintf(&b, ` data-start=%q data-duration=%q`,
		secondsAttr(target.startMs), secondsAttr(target.durationMs))
	// ── And which row it is on ───────────────────────────────────────────
	// Written here as well as by the director, because media added from
	// the panel is exactly the media a person then wants to drag: a clip
	// with no row falls back to one inferred from its kind, which draws
	// fine and cannot be changed.
	fmt.Fprintf(&b, ` data-track-index="%d"`, hfproject.DefaultTrackFor(kind))
	if tag == "img" {
		// Sized to its slot rather than left at its natural size, which
		// for a 4000px stock photograph is a film with one corner of a
		// photograph in it.
		b.WriteString(` class="clip" style="width:100%;height:100%;object-fit:cover"`)
	} else {
		b.WriteString(` class="clip"`)
	}
	if assetPath != "" {
		// data-media as well as the file, so the document SAYS what this
		// is. Without it an uploaded voiceover is an <audio> like any
		// other, and reads back as music — the one pair the tag cannot
		// tell apart. The worker skips it: a declaration whose
		// data-media-resolved already agrees with its src is done.
		fmt.Fprintf(&b, ` data-media=%q src=%q data-media-resolved=%q`, kind, assetPath, assetPath)
	} else {
		fmt.Fprintf(&b, ` data-media=%q data-media-intent=%q`, kind, intent)
	}
	if tag == "img" {
		b.WriteString(" />")
	} else {
		fmt.Fprintf(&b, "></%s>", tag)
	}

	// Appended INSIDE the scene, just before its closing tag, so it
	// belongs to the scene and moves with it.
	closing := len("</" + target.el.TagName + ">")
	at := target.el.End - closing
	out, aerr := hfedit.Apply(source, []hfedit.Edit{{Start: at, End: at, Text: b.String()}})
	what := intent
	if assetPath != "" {
		what = path.Base(assetPath)
	}
	// Named the way the author named it — "end", not "s2". A revisions
	// list is read by the person who wrote the film.
	where := strings.TrimSpace(target.el.Attrs["data-scene"])
	if where == "" {
		where = target.id
	}
	return out, fmt.Sprintf("%s in %s · %s", kind, where, shortened(what, 40)), aerr
}

// checkAssetPath refuses anything that is not a file inside the project.
//
// The path is written into the document as a src and is resolved relative
// to the project directory, so an absolute URL here would put a remote
// dependency into a film that is supposed to render with no network — and
// `..` would point outside the directory the worker materialises.
func checkAssetPath(p string) error {
	if strings.Contains(p, "://") || strings.HasPrefix(p, "//") {
		return fmt.Errorf("a film's media has to be a file in the project, not a link")
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("%q is not a path inside the project", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "" {
			return fmt.Errorf("%q is not a path inside the project", p)
		}
	}
	if assetKind(p) == "" {
		return fmt.Errorf("%q is not a kind of file a film can play", path.Base(p))
	}
	return nil
}

// assetKind says what a file is, by extension, or "" for something a film
// cannot use.
func assetKind(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg":
		return "image"
	case ".mp3", ".wav", ".m4a", ".aac", ".ogg":
		return "audio"
	case ".mp4", ".webm", ".mov":
		return "video"
	}
	return ""
}

func isMediaTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "img", "audio", "video":
		return true
	}
	return false
}

// shortened is a label for the revisions list, cut to fit.
func shortened(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// freeMediaID names a new media element something nothing else is called.
func freeMediaID(source, kind string) string {
	for n := 1; n < 200; n++ {
		id := fmt.Sprintf("%s-%d", kind, n)
		if _, taken := hfedit.FindByID(source, id); !taken {
			return id
		}
	}
	return kind + "-x"
}
