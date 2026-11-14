package design

// Carrying a project's timeline onto the design document.
//
// One converter, used by every path that writes a project — authoring, the
// inspector, a scoped AI edit, a structural change, media placement. The
// alternative was the same loop copied five times, and the copy that
// drifted would be the one that left the timeline describing a film that
// is no longer there.

import (
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfproject"
)

// scenesOf is the film's spine, for the layer tree and the scene panel.
func scenesOf(p *hfproject.Project) []trendlymodels.DesignScene {
	out := make([]trendlymodels.DesignScene, 0, len(p.Scenes))
	for _, s := range p.Scenes {
		out = append(out, trendlymodels.DesignScene{
			ID: s.ID, Src: s.Src, StartMs: s.StartMs,
			DurationMs: s.DurationMs, TrackIndex: s.TrackIndex, TrackKind: s.TrackKind,
		})
	}
	return out
}

// clipsOf is everything on the timeline, on its row.
func clipsOf(p *hfproject.Project) []trendlymodels.DesignClip {
	out := make([]trendlymodels.DesignClip, 0, len(p.Clips))
	for _, c := range p.Clips {
		out = append(out, trendlymodels.DesignClip{
			ElementID: c.ElementID, Kind: c.Kind, TrackIndex: c.TrackIndex,
			Authored: c.Authored, StartMs: c.StartMs, DurationMs: c.DurationMs,
			Label: c.Label, Src: c.Src,
		})
	}
	return out
}
