package design

// Editing one element of a film, without a model and without a render.
//
// ── What this is for ──────────────────────────────────────────────────────
// Phase 13: "the edits that should never cost a model call. Spending a
// token to change a hex value is absurd, and waiting four seconds for it is
// worse." Changing a headline's words, its colour, its size, when it starts
// and how long it lasts are all things the user can simply say, and the
// film should change while they are still looking at it.
//
// ── Why the whole project is copied for one attribute ─────────────────────
// A revision has to be COMPLETE: restore it and the film must play, which
// means its fonts, its logo and its resolved media have to be in the same
// directory as its index.html. So everything except the document is copied
// server-side — the bytes never leave the bucket — and only the edited
// document is written.
//
// ── What validates it ─────────────────────────────────────────────────────
// hfproject.Build, which is the same validator the authoring path passes
// through, run against the EDITED document before anything is written. The
// plan asks for `check --json` as well; that is the Hyperframes CLI, which
// lives in the render image, and paying a Fargate cold start for a colour
// change would undo the entire point of this endpoint. `check` still runs
// on the next preview or render, and Phase 12 is the way back if it ever
// disagrees.

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfedit"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/mediastore"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

type editElementRequest struct {
	// ElementID is the id the player reported when the user clicked.
	ElementID string `json:"elementId" binding:"required"`

	// Text replaces the element's words. Nil leaves them alone — which is
	// not the same as "" , which empties the element.
	Text *string `json:"text"`

	// Style merges into the element's inline style, property by property.
	// A property with an empty value is REMOVED, which is how the panel
	// clears a background without having to send the whole declaration.
	Style map[string]string `json:"style"`

	// Voice replaces what a voiceover SAYS. Only meaningful on an element
	// declared data-media="voice"; see applyElementEdit for why it clears
	// what the old words resolved to.
	Voice *string `json:"voice"`

	// StartMs and DurationMs set the element's place on the timeline. In
	// milliseconds because that is what the app's timeline works in; the
	// document wants seconds, and the conversion lives here rather than in
	// the client, so one of them cannot drift.
	StartMs    *int `json:"startMs"`
	DurationMs *int `json:"durationMs"`
}

// maxStyleProperties bounds one request. A panel sends a handful; a
// thousand would be something else.
const maxStyleProperties = 40

// EditDesignElement changes one element of a design's film.
// POST /api/design/brands/:brandId/designs/:designId/element
//
// Charges nothing: there is no model call in here. It produces a revision
// like every other write, so an edit is undoable from the moment it lands.
func EditDesignElement(c *gin.Context) {
	brandID, designID, ok := guard(c, true)
	if !ok {
		return
	}
	var req editElementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Text == nil && req.Voice == nil && len(req.Style) == 0 && req.StartMs == nil && req.DurationMs == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing to change"})
		return
	}
	if len(req.Style) > maxStyleProperties {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many style properties in one edit"})
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

	source, gerr := mediastore.Get(ctx, mediastore.TargetAttachment, d.ProjectPrefix+"/index.html")
	if gerr != nil {
		mysentry.Capture(gerr, map[string]string{"op": "design.element.read", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read this film"})
		return
	}

	edited, eerr := applyElementEdit(string(source), req)
	if eerr != nil {
		// The user's end of the problem, in their terms: an element that is
		// no longer there, or a change that does not make sense for it.
		c.JSON(http.StatusBadRequest, gin.H{"error": eerr.Error()})
		return
	}

	// ── The gate, before anything is written ─────────────────────────────
	// An edit that breaks the project is refused here, with the previous
	// revision untouched, rather than discovered by the next render.
	project, berr := hfproject.Build([]hfproject.File{{Path: "index.html", Content: edited}})
	if berr != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "that change would break the film",
			"message": berr.Error(),
		})
		return
	}

	rev, rerr := trendlymodels.AllocateDesignRevision(ctx, brandID, designID)
	if rerr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the change"})
		return
	}
	prefix := hfproject.RevisionPrefix(brandID, designID, rev)

	// Everything but the document, copied inside the bucket. The player
	// pages are left out because they are rewritten below, and a copy of
	// the old one would be served until it was.
	if _, cerr := mediastore.CopyPrefix(ctx, mediastore.TargetAttachment, d.ProjectPrefix, prefix,
		"index.html", "player.html", "preview.html"); cerr != nil {
		mysentry.Capture(cerr, map[string]string{"op": "design.element.copy", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the change"})
		return
	}

	if _, uerr := hfproject.Upload(ctx, brandID, designID, rev, project); uerr != nil {
		mysentry.Capture(uerr, map[string]string{"op": "design.element.upload", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the change"})
		return
	}
	previewURL, perr := hfproject.UploadPlayer(ctx, brandID, designID, rev, project)
	if perr != nil {
		mysentry.Capture(perr, map[string]string{"op": "design.element.player", "designId": designID})
	}

	scenes := scenesOf(project)

	pruned, cerr := trendlymodels.CommitDesignRevision(ctx, brandID, designID, trendlymodels.DesignRevision{
		N:          rev,
		Prefix:     prefix,
		PreviewURL: previewURL,
		By:         trendlymodels.DesignRevisionInspector,
		Note:       editNote(req),
		DurationMs: project.DurationMs,
		Scenes:     scenes,
		// An inspector edit changes the timeline too: emptying a
		// voiceover's src leaves the clip's file and label describing a
		// recording that is no longer there, and a revision stored
		// without clips restores someone else's film onto this one.
		Clips: clipsOf(project),
	})
	if cerr != nil {
		mysentry.Capture(cerr, map[string]string{"op": "design.element.commit", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the change"})
		return
	}
	dropRevisionFiles(ctx, pruned)

	c.JSON(http.StatusOK, gin.H{
		"revision":   rev,
		"previewUrl": previewURL,
		"durationMs": project.DurationMs,
	})
}

// applyElementEdit works the request into the document.
//
// Separated from the handler so it can be tested against a real film
// without a bucket, a session or a design — which is where the bugs in
// this kind of code actually live.
func applyElementEdit(source string, req editElementRequest) (string, error) {
	el, found := hfedit.FindByID(source, req.ElementID)
	if !found {
		return "", fmt.Errorf("that element is no longer in this film")
	}
	if err := editableTarget(el, source); err != nil {
		return "", err
	}

	var edits []hfedit.Edit
	if req.Text != nil {
		// ── Words, not demolition ────────────────────────────────────────
		// Replacing an element's text replaces EVERYTHING inside it. On a
		// headline that is the point; on the <div> that holds every scene
		// it is the whole film gone, and the player's bridge resolves a
		// click through closest("[id]"), so clicking the background
		// selects exactly that div. One keystroke in the text box would
		// have flattened the film.
		if el.HoldsAddressable(source) {
			return "", fmt.Errorf("that is a group of other elements, so there are no words of its own to change")
		}
		// Timed children are animation, not formatting. A caption track's
		// words are one timed span each; replacing its text would flatten
		// them into a single node and the captions would stop appearing
		// word by word, which is the whole point of them.
		if el.HoldsTimed(source) {
			return "", fmt.Errorf("that is animated piece by piece, so its words are not set here")
		}
		e, err := hfedit.SetText(el, *req.Text)
		if err != nil {
			return "", err
		}
		edits = append(edits, e)
	}
	if req.Voice != nil {
		// ── Rewriting what the film says ─────────────────────────────────
		// A voiceover's words are its data-media-intent. Changing them is
		// not enough on its own: the element still has the WAV of the old
		// line beside it, and the resolver skips anything whose
		// data-media-resolved agrees with its src. So both go, which is
		// precisely how this asks for the new line to be spoken — the
		// next preview or render resolves it again, and the captions are
		// rebuilt from the audio that comes back.
		//
		// A film therefore says the new words from the next render, not
		// from this request: speaking takes a round trip to HeyGen, and
		// an endpoint that is otherwise instant should not wait on one.
		words := strings.TrimSpace(*req.Voice)
		if words == "" {
			return "", fmt.Errorf("a voiceover has to say something")
		}
		if !strings.EqualFold(strings.TrimSpace(el.Attrs["data-media"]), "voice") {
			return "", fmt.Errorf("that is not a voiceover")
		}
		e, err := hfedit.SetAttribute(el, source, "data-media-intent", words)
		if err != nil {
			return "", err
		}
		edits = append(edits, e)
		for _, stale := range []string{"src", "data-media-resolved"} {
			if _, has := el.Attrs[stale]; !has {
				continue
			}
			// Emptied rather than removed: removing an attribute and
			// setting another on the same tag means two edits over
			// overlapping bytes, which Apply refuses — and an empty src
			// is as unresolved as a missing one.
			d, derr := hfedit.SetAttribute(el, source, stale, "")
			if derr != nil {
				return "", derr
			}
			edits = append(edits, d)
		}
	}
	if len(req.Style) > 0 {
		merged, err := mergeStyle(el.Attrs["style"], req.Style)
		if err != nil {
			return "", err
		}
		e, err := hfedit.SetAttribute(el, source, "style", merged)
		if err != nil {
			return "", err
		}
		edits = append(edits, e)
	}
	// ── Timing ───────────────────────────────────────────────────────────
	// The runtime reads data-start and data-duration in SECONDS. Formatted
	// to two decimals: a frame at 30fps is 33ms, so hundredths are finer
	// than anything the film can show, and printing the full float would
	// put 1.7000000000000002 in someone's markup.
	if req.StartMs != nil {
		if *req.StartMs < 0 {
			return "", fmt.Errorf("a clip cannot start before the film does")
		}
		e, err := hfedit.SetAttribute(el, source, "data-start", secondsAttr(*req.StartMs))
		if err != nil {
			return "", err
		}
		edits = append(edits, e)
	}
	if req.DurationMs != nil {
		if *req.DurationMs <= 0 {
			return "", fmt.Errorf("a clip has to last longer than no time at all")
		}
		e, err := hfedit.SetAttribute(el, source, "data-duration", secondsAttr(*req.DurationMs))
		if err != nil {
			return "", err
		}
		edits = append(edits, e)
	}
	return hfedit.Apply(source, edits)
}

// editableTarget refuses the one element that must never be rewritten.
//
// The root composition IS the film: its id, its data-composition-id and its
// data-duration are what the renderer enters through. An edit that replaces
// it is not a scoped edit however it is framed, and the AI path calling it
// one would be a lie. Scenes are fair game — rewriting a whole scene is a
// reasonable thing to ask for.
func editableTarget(el hfedit.Element, source string) error {
	if _, isRoot := el.Attrs["data-composition-id"]; isRoot {
		return fmt.Errorf("that is the whole film rather than something in it — pick an element inside it")
	}
	return nil
}

func secondsAttr(ms int) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", float64(ms)/1000), "0"), ".")
}

// mergeStyle folds changed properties into an existing inline style.
//
// Merged rather than replaced, because the panel knows about colour and
// size and the document may carry a transform, a filter or a gradient the
// panel has no control for — and an edit to the colour must not throw those
// away. An empty value removes a property, which is how a background is
// cleared.
//
// Property names are checked against what CSS allows in one, for the same
// reason the brand kit checks its hex values: a declaration assembled from
// a request body is a place where a stray `;` turns one property into two.
func mergeStyle(existing string, changes map[string]string) (string, error) {
	order := make([]string, 0, 8)
	have := map[string]string{}
	for _, decl := range strings.Split(existing, ";") {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}
		name, value, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if _, seen := have[name]; !seen {
			order = append(order, name)
		}
		have[name] = strings.TrimSpace(value)
	}

	// Sorted, so the same request twice produces the same document — a map
	// iterates in a different order every time, and a revision that
	// differs from the last only in declaration order is a revision nobody
	// can read a diff of.
	names := make([]string, 0, len(changes))
	for name := range changes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, rawName := range names {
		name := strings.ToLower(strings.TrimSpace(rawName))
		value := strings.TrimSpace(changes[rawName])
		if !validStyleName(name) {
			return "", fmt.Errorf("%q is not a style property", rawName)
		}
		if value == "" {
			delete(have, name)
			continue
		}
		if !validStyleValue(value) {
			return "", fmt.Errorf("%q is not something a style property can be set to", value)
		}
		if _, seen := have[name]; !seen {
			order = append(order, name)
		}
		have[name] = value
	}

	out := make([]string, 0, len(order))
	for _, name := range order {
		if v, ok := have[name]; ok {
			out = append(out, name+":"+v)
		}
	}
	return strings.Join(out, ";"), nil
}

// validStyleName allows a CSS property and a custom property, and nothing
// that could end a declaration or start another.
func validStyleName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
		default:
			return false
		}
	}
	return true
}

// validStyleValue refuses what would break OUT of the declaration or the
// attribute: a semicolon starts a second property, a quote or an angle
// bracket ends the attribute, and `url(`, `expression(` or a comment is how
// a style smuggles a request somewhere else.
func validStyleValue(v string) bool {
	if v == "" || len(v) > 200 {
		return false
	}
	if strings.ContainsAny(v, ";\"'<>{}\\") {
		return false
	}
	lower := strings.ToLower(v)
	for _, bad := range []string{"url(", "expression(", "javascript:", "/*", "*/", "@import"} {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	return true
}

// editNote is the line the revisions list shows for this edit.
func editNote(req editElementRequest) string {
	var parts []string
	if req.Text != nil {
		parts = append(parts, "text")
	}
	if req.Voice != nil {
		parts = append(parts, "voiceover")
	}
	if len(req.Style) > 0 {
		names := make([]string, 0, len(req.Style))
		for k := range req.Style {
			names = append(names, strings.ToLower(strings.TrimSpace(k)))
		}
		sort.Strings(names)
		parts = append(parts, names...)
	}
	if req.StartMs != nil || req.DurationMs != nil {
		parts = append(parts, "timing")
	}
	if len(parts) == 0 {
		return "Edited " + req.ElementID
	}
	return fmt.Sprintf("%s · %s", req.ElementID, strings.Join(parts, ", "))
}
