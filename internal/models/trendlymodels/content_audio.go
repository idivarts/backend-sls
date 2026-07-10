package trendlymodels

// ContentAudio is the generated audio attached to a video content, muxed in at
// render time. References generatedAudio docs by id/url.
type ContentAudio struct {
	MusicID       string  `json:"musicId,omitempty" firestore:"musicId,omitempty"`
	MusicURL      string  `json:"musicUrl,omitempty" firestore:"musicUrl,omitempty"`
	MusicVolume   float64 `json:"musicVolume,omitempty" firestore:"musicVolume,omitempty"`
	VoiceoverID   string  `json:"voiceoverId,omitempty" firestore:"voiceoverId,omitempty"`
	VoiceoverURL  string  `json:"voiceoverUrl,omitempty" firestore:"voiceoverUrl,omitempty"`
	DuckMusic     bool    `json:"duckMusic,omitempty" firestore:"duckMusic,omitempty"`
	CaptionSource string  `json:"captionSource,omitempty" firestore:"captionSource,omitempty"`
}
