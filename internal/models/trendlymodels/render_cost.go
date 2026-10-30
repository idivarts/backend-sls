package trendlymodels

// ── What a server-side render costs the wallet ──────────────────────────────
//
// Rendering used to run on the user's own device, so it was free to us. It now
// runs on our infrastructure, so it is metered like any other AI action: the
// job's real cost is estimated in USD and converted with TokensForCost, which
// keeps ONE definition of what a wallet token is worth.
//
// ⚠️ PROVISIONAL, like every other Phase-1 billing number. Re-tune once a month
// of real render telemetry exists — these are modelled, not measured.
//
// Where the figures come from (us-east-1, arm64):
//
//	Compute is almost free. A slide screenshot is ~0.5s on a 4 GB arm64 Lambda
//	(~$0.00008); a second of video is ~2s of a 4 vCPU Fargate Spot task
//	(~$0.00009). Neither is what makes a render cost money.
//
//	DELIVERY is what costs. A 1080x1350 PNG is ~2 MB and a 30s 1080x1920 MP4 is
//	~19 MB, all of it served from CloudFront at ~$0.085/GB, several times over
//	the asset's life. That is roughly 6x the compute for an image and 2x for a
//	video, and it is the reason a render is metered per SLIDE and per SECOND
//	rather than per job.
//
//	On top of the modelled cost sits a 2x markup, covering retries, failed
//	renders we don't charge for, Spot falling back to on-demand, and the fact
//	that a modelled number is always an optimistic one.
//
// What is NOT charged:
//   - a retry after a failure — the user already paid for the attempt we lost
//   - a duplicate queue delivery — the worker skips an already-rendered revision
//   - a re-render of an unchanged revision — only `force` re-renders, and that
//     is a deliberate user action, so it charges again
const (
	// renderCostMarkupX covers retries, failures and model optimism.
	renderCostMarkupX = 2.0

	// renderImageSlideUSD is the modelled all-in cost of one rendered slide.
	renderImageSlideUSD = 0.00058

	// renderVideoFixedUSD covers container pull, browser launch and the encode
	// tail — the part a one-second video pays just as much as a sixty-second one.
	renderVideoFixedUSD = 0.0015

	// renderVideoPerSecondUSD is compute plus delivery per second of output.
	renderVideoPerSecondUSD = 0.00025

	// renderClipUSD is the per-clip overhead of a composition: one more
	// intermediate encode, one more file written and read back, and one more
	// xfade pass over the join. A one-clip composition pays none of this — it
	// takes the passthrough path and is literally the same encode as before.
	renderClipUSD = 0.0004

	// renderDecodePerSecondUSD prices a second of DECODED footage, which a
	// design clip does not pay. Decoding is cheaper than rendering a browser
	// frame but it is not free, and a timeline of long clips trimmed down to
	// seconds still has to decode what it skips past.
	renderDecodePerSecondUSD = 0.00008
)

// RenderTokenCost returns the wallet tokens one render job should deduct.
//
// Worked examples at today's constants:
//
//	single post  (1 slide)      ~  221 tokens
//	carousel     (6 slides)     ~1,326 tokens
//	reel         (15 seconds)   ~2,000 tokens
//	reel         (30 seconds)   ~3,429 tokens
//
// For scale: one AI design generation is typically 20k-80k tokens, so a render
// lands around 2-5% of the generation that produced it — a real cost, correctly
// proportioned against the thing it finishes.
func RenderTokenCost(docType string, slideCount, durationMs int) int64 {
	if docType == "video" {
		seconds := float64(durationMs) / 1000
		if seconds < 1 {
			seconds = 1
		}
		usd := renderVideoFixedUSD + renderVideoPerSecondUSD*seconds
		return TokensForCost(usd * renderCostMarkupX)
	}

	slides := slideCount
	if slides < 1 {
		slides = 1
	}
	return TokensForCost(renderImageSlideUSD * float64(slides) * renderCostMarkupX)
}

// CompositionRenderTokenCost prices a timeline render.
//
// It is the video price plus what a timeline actually adds: a per-clip overhead
// for the extra intermediate encode and join, and a per-second decode charge on
// the clips whose pixels come from a file rather than from Chromium.
//
// A single-design composition is deliberately priced IDENTICALLY to the design
// render it replaces. Every pre-existing video design now goes through the
// composition path, and a user whose reel suddenly cost more for no visible
// reason would be right to complain — the render is the same encode, so it is
// the same price. That is what the clipCount == 1 short-circuit is for, not an
// optimisation.
//
// ⚠️ PROVISIONAL like every other Phase-1 billing number — modelled, not
// measured. See the note at the top of this file.
func CompositionRenderTokenCost(durationMs, clipCount, decodedMs int) int64 {
	if clipCount <= 1 && decodedMs <= 0 {
		return RenderTokenCost("video", 1, durationMs)
	}

	seconds := float64(durationMs) / 1000
	if seconds < 1 {
		seconds = 1
	}
	clips := clipCount
	if clips < 1 {
		clips = 1
	}
	decodeS := float64(decodedMs) / 1000
	if decodeS < 0 {
		decodeS = 0
	}

	usd := renderVideoFixedUSD +
		renderVideoPerSecondUSD*seconds +
		renderClipUSD*float64(clips) +
		renderDecodePerSecondUSD*decodeS
	return TokensForCost(usd * renderCostMarkupX)
}

// CompositionDecodedMs is how much SOURCE video a composition has to decode.
//
// Source time, not timeline time, and the difference is the whole function. A
// trim is already a source span, so when one is set it is the answer outright.
// Without a trim, the span is the clip's slot scaled BY the speed: a 3-second
// slot at 2x plays 6 seconds of footage through the decoder to fill it, while
// the same slot at half speed only needs 1.5.
//
// Only footage and generated clips count. A design clip's pixels come out of
// Chromium, and a still is decoded once however long it is on screen.
func CompositionDecodedMs(c *Composition) int {
	total := 0
	for _, clip := range compositionVideoClips(c) {
		s := clip.Source
		if s.Kind != ClipSourceFootage && s.Kind != ClipSourceGenVideo {
			continue
		}
		span := clip.DurationMs
		if s.InMs != nil && s.OutMs != nil && *s.OutMs > *s.InMs {
			span = *s.OutMs - *s.InMs
		} else if s.Speed != nil && *s.Speed > 0 {
			span = int(float64(span) * *s.Speed)
		}
		if span > 0 {
			total += span
		}
	}
	return total
}
