package studio

// AI video for a scene: one beat at a time, with the consistency rules enforced.
//
// ── Why one beat per request ────────────────────────────────────────────────
// Every model this product can reach tops out at 8 seconds. A longer sequence
// is several clips on the timeline, not a longer generation — so the unit here
// is ONE SCENE, and a 24-second sequence is three requests the client walks.
// That also means a failure costs one beat, and the user can stop after two.
//
// ── The consistency rules, enforced rather than suggested ───────────────────
// Three of them live here because the plan's doctrine is only real if the code
// refuses to break it:
//
//  1. A CAST MEMBER IN MORE THAN TWO GENERATED CLIPS NEEDS AN ANCHOR. Without
//     one, each clip is generated from its own reading of a description and the
//     face drifts. Two clips is the tolerance — a before and an after, where a
//     slight difference reads as time passing rather than as a different
//     person.
//  2. THE `look` STRING IS APPENDED VERBATIM. Not summarised, not rephrased:
//     paraphrasing the wardrobe between clips is how a navy jumper becomes a
//     blue shirt.
//  3. A CHAIN AND AN ANCHOR CANNOT BOTH APPLY. The provider forbids combining a
//     keyframe with a reference image, so a beat either continues the previous
//     instant or is guaranteed the same face. Refused in mediajobs.SubmitVideo
//     with that explanation rather than discovered as a 422.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/mediajobs"
	"github.com/idivarts/backend-sls/internal/trendlyapis/rendering"
)

// How many generated clips a cast member may appear in without an anchor.
//
// Two: a before and an after, where a slight difference in the face reads as
// time passing. By the third appearance it reads as a different person, and no
// prompt wording fixes it.
const anchorRequiredAbove = 2

type generateVideoRequest struct {
	ModelID string `json:"modelId" binding:"required"`
	Prompt  string `json:"prompt" binding:"required"`

	DurationSecs  int    `json:"durationSecs"`
	Resolution    string `json:"resolution"`
	GenerateAudio bool   `json:"generateAudio"`

	// CastIDs are the people in this beat. Their `look` strings are appended
	// verbatim and their anchors become reference images.
	CastIDs    []string `json:"castIds"`
	LocationID string   `json:"locationId"`

	// StartFrameMediaID chains this beat onto the previous one.
	//
	// It is a MEDIA ID, not a URL, and it has to exist already — extraction is
	// a worker job, so the flow is: POST …/prepare-chain, wait for the frame to
	// appear in the library (the app is subscribed), then generate with it.
	// Doing the extraction inline would mean holding a request open for a
	// download, a probe and an ffmpeg pass.
	StartFrameMediaID string `json:"startFrameMediaId"`
	ChainedFromScene  string `json:"chainedFromSceneId"`
}

// GenerateSceneVideo starts one generated clip for one scene.
// POST …/studios/:studioId/scenes/:sceneId/generate-video
//
// Returns 202. The scene shows as pending until the job resolves, which is
// minutes away — nothing here waits for it.
func GenerateSceneVideo(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req generateVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	sceneID := c.Param("sceneId")

	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}
	sc, err := trendlymodels.GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil || sc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such scene"})
		return
	}

	scenes, err := trendlymodels.ListStudioScenes(ctx, brandID, contentID, studioID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the scenes"})
		return
	}

	// ── Rule 1: an anchor, above two appearances ──
	cast, refusal := resolveCast(s, req.CastIDs, sceneID, scenes)
	if refusal != nil {
		c.JSON(refusal.Status, refusal.Body)
		return
	}

	// ── Rule 2: the look, verbatim ──
	prompt := strings.TrimSpace(req.Prompt)
	var referenceURLs []string
	for _, m := range cast {
		if look := strings.TrimSpace(m.Look); look != "" {
			prompt += " " + look
		}
		if m.AnchorMediaID != "" {
			if a, aerr := trendlymodels.GetAsset(ctx, brandID, m.AnchorMediaID); aerr == nil && a != nil {
				if a.NoAIInput {
					// A licence that forbids feeding the image to a model. Said
					// here rather than discovered at the provider, because the
					// refusal there reads as the feature being broken.
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "anchor_not_usable",
						"message": fmt.Sprintf(
							"The still anchoring %s can't be used as an AI reference. Pick another.",
							m.ID),
					})
					return
				}
				referenceURLs = append(referenceURLs, a.URL)
			}
		}
	}
	if loc := findLocation(s, req.LocationID); loc != nil {
		if look := strings.TrimSpace(loc.Look); look != "" {
			prompt += " " + look
		}
		if loc.AnchorMediaID != "" && len(referenceURLs) == 0 {
			// A location anchor is only used when no cast anchor is: the
			// provider takes a small number of references, and a face matters
			// more than a room.
			if a, aerr := trendlymodels.GetAsset(ctx, brandID, loc.AnchorMediaID); aerr == nil && a != nil && !a.NoAIInput {
				referenceURLs = append(referenceURLs, a.URL)
			}
		}
	}

	startFrameURL := ""
	if req.StartFrameMediaID != "" {
		frame, ferr := trendlymodels.GetAsset(ctx, brandID, req.StartFrameMediaID)
		if ferr != nil || frame == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "no_start_frame",
				"message": "That start frame isn't in your media yet. Prepare the chain first.",
			})
			return
		}
		startFrameURL = frame.URL
	}

	duration := req.DurationSecs
	if duration <= 0 {
		// The scene's own length, rounded to whole seconds — which is all any
		// of these models take.
		duration = int((trendlymodels.EffectiveSceneMs(*sc) + 999) / 1000)
		if duration <= 0 {
			duration = 4
		}
	}

	out, sub := mediajobs.SubmitVideo(ctx, mediajobs.VideoRequest{
		BrandID:   brandID,
		ContentID: contentID,
		Target: mediajobs.Target{
			StudioID: studioID,
			SceneID:  sceneID,
		},
		ModelID:       req.ModelID,
		Prompt:        prompt,
		AspectRatio:   aspectFor(s.Width, s.Height),
		DurationSecs:  duration,
		Resolution:    req.Resolution,
		GenerateAudio: req.GenerateAudio,
		StartFrameURL: startFrameURL,
		ReferenceURLs: referenceURLs,
		CreatedBy:     c.GetString("userId"),
	})
	if sub != nil {
		c.JSON(sub.Status, sub.Body)
		return
	}

	// Record what was asked for BEFORE reporting success, so a scene whose
	// generation is in flight can be read as such from the document alone —
	// the rail subscribes to the scene, not to this response.
	if uerr := trendlymodels.UpdateStudioScene(ctx, brandID, contentID, studioID, sceneID,
		[]firestore.Update{
			{Path: "pending", Value: true},
			{Path: "gen", Value: &trendlymodels.SceneGeneration{
				Model:              req.ModelID,
				Prompt:             prompt,
				JobID:              out.JobID,
				Status:             "running",
				ChainedFromSceneID: req.ChainedFromScene,
				StartFrameMediaID:  req.StartFrameMediaID,
				ReferenceMediaIDs:  anchorIDs(cast),
				CastIDs:            req.CastIDs,
				LocationID:         req.LocationID,
			}},
		}); uerr != nil {
		c.Header("X-Scene-Bookkeeping", "failed")
	}

	c.JSON(http.StatusAccepted, gin.H{
		"jobId":       out.JobID,
		"status":      trendlymodels.MediaJobPending,
		"tokensSpent": gin.H{"charged": out.Tokens > 0, "tokens": out.Tokens},
	})
}

// resolveCast returns the cast members for a beat, refusing when one of them
// appears in more than two generated clips without an approved anchor.
//
// Counting ACROSS the studio rather than within the request is the whole point:
// the drift happens between separate generations, so the question is how many
// clips this person is in altogether.
func resolveCast(
	s *trendlymodels.Studio,
	castIDs []string,
	sceneID string,
	scenes []trendlymodels.StudioScene,
) ([]trendlymodels.CastMember, *mediajobs.Refusal) {
	out := make([]trendlymodels.CastMember, 0, len(castIDs))
	for _, id := range castIDs {
		member := findCast(s, id)
		if member == nil {
			return nil, &mediajobs.Refusal{
				Status: http.StatusBadRequest,
				Body: map[string]any{
					"error":   "unknown_cast",
					"message": fmt.Sprintf("There is no cast member %q in this studio.", id),
				},
			}
		}

		appearances := 1 // this one
		for _, other := range scenes {
			if other.ID == sceneID || other.Gen == nil {
				continue
			}
			for _, cid := range other.Gen.CastIDs {
				if cid == id {
					appearances++
					break
				}
			}
		}

		if appearances > anchorRequiredAbove && member.AnchorMediaID == "" {
			return nil, &mediajobs.Refusal{
				Status: http.StatusBadRequest,
				Body: map[string]any{
					"error":       "anchor_required",
					"castId":      id,
					"appearances": appearances,
					"message": fmt.Sprintf(
						"%s appears in %d generated clips. Above two, each clip invents its own "+
							"face unless they all reference one approved still — so pick or make "+
							"an anchor for them first.",
						strings.TrimSpace(member.Description), appearances),
				},
			}
		}
		out = append(out, *member)
	}
	return out, nil
}

type prepareChainRequest struct {
	// FromSceneID is the beat this one continues. Its clip's LAST frame
	// becomes the start frame.
	FromSceneID string `json:"fromSceneId" binding:"required"`
}

// PrepareChain extracts the previous beat's last frame, as its own media.
// POST …/studios/:studioId/scenes/:sceneId/prepare-chain
//
// Asynchronous, because extraction needs ffmpeg and that lives in the render
// worker. The app is already subscribed to the media library, so the frame
// appears there and the generate call follows with its id.
//
// ⚠️ A chain is for ONE UNBROKEN MOVEMENT carrying on. For a person appearing in
// two different moments, the mechanism is a cast anchor — and the two cannot be
// combined on the same clip, because the provider refuses a keyframe alongside
// a reference image.
func PrepareChain(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req prepareChainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	from, err := trendlymodels.GetStudioScene(ctx, brandID, contentID, studioID, req.FromSceneID)
	if err != nil || from == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such scene to chain from"})
		return
	}

	mediaID := firstMediaID(from)
	if mediaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "nothing_to_chain_from",
			"message": "That scene has no clip yet. Generate it first.",
		})
		return
	}
	src, err := trendlymodels.GetAsset(ctx, brandID, mediaID)
	if err != nil || src == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read that scene's clip"})
		return
	}
	if src.Kind != trendlymodels.AssetKindVideo {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "not_a_video",
			"message": "Only a video has a last frame to continue from.",
		})
		return
	}

	// -1 means the LAST frame, which the worker computes: it has ffprobe and we
	// do not, and an offset guessed from a stored duration lands past the end
	// and yields black — which would then become the next clip's opening image.
	if err := rendering.SubmitFrameExtract(brandID, mediaID, src.URL, -1); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "extract_failed",
			"message": "We couldn't take that frame. Please try again.",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"status":        "extracting",
		"sourceMediaId": mediaID,
		"note":          "The frame appears in your media when it's ready; generate with its id.",
	})
}

// VideoQuote prices a generation before anything is spent.
// GET …/studios/:studioId/video-quote?modelId=&durationSecs=
//
// The cost gate's honest half: the number is shown BEFORE the button, from the
// registry's own per-second rate, so a run's price is never a surprise. It also
// reports what the model actually accepts, so the UI can offer only the lengths
// that will not be refused.
func VideoQuote(c *gin.Context) {
	brandID, _, _, ok := guard(c, false)
	if !ok {
		return
	}
	modelID := c.Query("modelId")
	model, err := trendlymodels.GetMediaModel(c.Request.Context(), modelID)
	if err != nil || model == nil || !model.Enabled || model.Kind != "video" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "unknown_model",
			"message": "That video model isn't available.",
		})
		return
	}

	secs, _ := strconv.Atoi(c.Query("durationSecs"))
	if secs <= 0 {
		secs = 4
	}
	tokens := trendlymodels.GenerativeVideoTokenCost(model, secs)

	c.JSON(http.StatusOK, gin.H{
		"tokens":       tokens,
		"durationSecs": secs,
		"model": gin.H{
			"id":          model.ID,
			"displayName": model.DisplayName,
			"durationsS":  model.DurationsS,
			"minS":        model.MinDurationS,
			"maxS":        model.MaxDurationS,
			"startFrame":  model.StartFrame,
			"endFrame":    model.EndFrame,
			"imageRefs":   model.ImageRefs,
			"nativeAudio": model.NativeAudio,
			"resolutions": model.Resolutions,
		},
		// Said out loud wherever a price is shown. Per-model rates are not
		// published by the providers, so every one of these is an estimate.
		"provisional": true,
		"brandId":     brandID,
	})
}

// ── Small helpers ──────────────────────────────────────────────────────────

func findCast(s *trendlymodels.Studio, id string) *trendlymodels.CastMember {
	for i := range s.Cast {
		if s.Cast[i].ID == id {
			return &s.Cast[i]
		}
	}
	return nil
}

func findLocation(s *trendlymodels.Studio, id string) *trendlymodels.StudioLocation {
	if id == "" {
		return nil
	}
	for i := range s.Locations {
		if s.Locations[i].ID == id {
			return &s.Locations[i]
		}
	}
	return nil
}

func anchorIDs(cast []trendlymodels.CastMember) []string {
	out := make([]string, 0, len(cast))
	for _, m := range cast {
		if m.AnchorMediaID != "" {
			out = append(out, m.AnchorMediaID)
		}
	}
	return out
}

func firstMediaID(sc *trendlymodels.StudioScene) string {
	for _, l := range sc.Layers {
		if l.MediaID != "" {
			return l.MediaID
		}
	}
	return ""
}

// aspectFor maps a canvas to the ratio string the providers take.
//
// Only the two they accept. A canvas that is neither — a 4:5 post — generates
// at 9:16 and is cropped on the way into the scene, which is better than
// refusing to generate for a format the model simply has no label for.
func aspectFor(width, height int) string {
	if height > width {
		return "9:16"
	}
	return "16:9"
}
