package director

import (
	"context"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// AttachUserMedia resolves the files the user attached and puts them on the
// manifest.
//
// ── Why resolution is fatal rather than best-effort ────────────────────────
// Every failure here is returned as a FATAL problem naming the handle. A
// silent drop would be the worst available outcome: the brief still reads
// perfectly ("@attachment1 is the face of the video"), the plan comes back
// without it, and the user has no way to tell whether the planner ignored
// their instruction or never received the file. Refusing costs them a retry;
// dropping costs them a plan they cannot debug and the tokens it took.
//
// It also means this runs BEFORE the model call, so a brief whose attachments
// do not resolve is refused without spending anything.
//
// ── Why the client's word is not taken for anything but the id ─────────────
// `AttachmentRef` carries a handle and a media id. Kind, aspect, duration and
// description are read from the library row here. A client that reports them
// is a client that can misreport them, and the planner makes real decisions
// off "this is a 9:16 video of a person".
//
// The lookup is brand-scoped, so a media id belonging to another brand simply
// does not resolve — which is the access check as well as the existence check.
func (m *Manifest) AttachUserMedia(
	ctx context.Context,
	brandID string,
	refs []trendlymodels.RunAttachment,
) []Problem {
	if len(refs) == 0 {
		return nil
	}

	var problems []Problem

	if len(refs) > MaxAttachments {
		return []Problem{{
			Where: "attachments",
			Message: fmt.Sprintf(
				"%d files were attached; at most %d can be sent with one brief.",
				len(refs), MaxAttachments),
			Fatal: true,
		}}
	}

	// Collect the ids first so the whole set is one read rather than one per
	// handle, and so a duplicate handle is caught before any lookup.
	seen := map[string]bool{}
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ref := strings.TrimPrefix(strings.TrimSpace(r.Ref), "@")
		if ref == "" || strings.TrimSpace(r.MediaID) == "" {
			problems = append(problems, Problem{
				Where:   "attachments",
				Message: "An attachment arrived with no handle or no file behind it.",
				Fatal:   true,
			})
			continue
		}
		if seen[ref] {
			// Two files under one handle makes every sentence using it
			// ambiguous, and picking one would be a guess about which.
			problems = append(problems, attachmentProblem(ref,
				"Two different files were attached under this handle."))
			continue
		}
		seen[ref] = true
		ids = append(ids, strings.TrimSpace(r.MediaID))
	}
	if len(ids) == 0 {
		return problems
	}

	rows, err := trendlymodels.GetAssets(ctx, brandID, ids)
	if err != nil {
		return append(problems, Problem{
			Where:   "attachments",
			Message: "The attached files could not be read back.",
			Fatal:   true,
		})
	}

	for _, r := range refs {
		ref := strings.TrimPrefix(strings.TrimSpace(r.Ref), "@")
		id := strings.TrimSpace(r.MediaID)
		if ref == "" || id == "" {
			continue // already reported above
		}

		a := rows[id]
		if a == nil {
			// Missing, deleted, or another brand's. Deliberately one message
			// for all three: which it is tells the user nothing useful and
			// telling them "that id belongs to someone else" would confirm it
			// exists.
			problems = append(problems, attachmentProblem(ref,
				"This file is not in your library any more."))
			continue
		}
		if a.NoAIInput {
			// The licence flag exists precisely for this. A stock photo is the
			// row that has it, and feeding it to a model is the thing the
			// licence forbids.
			problems = append(problems, attachmentProblem(ref,
				"This file's licence does not allow it to be sent to an AI model."))
			continue
		}

		m.Attachments = append(m.Attachments, Attachment{
			Ref:        ref,
			MediaID:    a.ID,
			Kind:       a.Kind,
			Describes:  describeAsset(*a),
			Aspect:     aspectOf(a.Width, a.Height),
			DurationMs: a.DurationMs,
		})
	}

	return problems
}

// AttachmentBlock is what the planner is told about these files.
//
// Said explicitly rather than left to be inferred from a JSON key. The planner
// is given a manifest with forty library rows in it; without this, four of
// them being the user's own deliberate choice is just a second array.
func AttachmentBlock(atts []Attachment) string {
	if len(atts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\nFILES THE USER ATTACHED TO THIS BRIEF:\n")
	for _, a := range atts {
		b.WriteString(fmt.Sprintf("  @%s — %s (%s", a.Ref, a.MediaID, a.Kind))
		if a.Aspect != "" {
			b.WriteString(", " + a.Aspect)
		}
		if a.DurationMs > 0 {
			b.WriteString(fmt.Sprintf(", %.1fs", float64(a.DurationMs)/1000))
		}
		b.WriteString(")")
		if a.Describes != "" {
			b.WriteString(": " + a.Describes)
		}
		b.WriteString("\n")
	}
	b.WriteString(`
These are the user's own files, chosen deliberately. They OUTRANK anything you
pick out of the media library on your own. An instruction attached to a handle
is binding, not a suggestion: if the brief says a handle is the face of the
video, that file's id belongs in a cast member's anchor, not merely mentioned.

A handle may be used as a cast anchor, a location anchor, a scene's start
frame, or a scene's media. Refer to them by media id in the plan — the handle
is how the USER names them, not a value the plan should contain.

`)
	// Said because it is a real limit of the pipeline rather than a choice,
	// and a planner that assumes it has watched the clip will describe shots
	// that are not in it.
	for _, a := range atts {
		if a.Kind == "video" {
			b.WriteString("You have NOT seen inside any attached video — only its " +
				"duration, aspect and description. Do not describe its contents.\n\n")
			break
		}
	}
	return b.String()
}
