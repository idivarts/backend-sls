package trendlymodels

// ContentAudio is the generated audio attached to a video content, muxed in at
// render time. References generatedAudio docs by id/url.
type ContentAudio struct {
	MusicID         string  `json:"musicId,omitempty" firestore:"musicId,omitempty"`
	MusicURL        string  `json:"musicUrl,omitempty" firestore:"musicUrl,omitempty"`
	MusicTitle      string  `json:"musicTitle,omitempty" firestore:"musicTitle,omitempty"`
	MusicVolume     float64 `json:"musicVolume,omitempty" firestore:"musicVolume,omitempty"`     // 0..1
	MusicFade       bool    `json:"musicFade,omitempty" firestore:"musicFade,omitempty"`         // fade in/out
	VoiceoverID     string  `json:"voiceoverId,omitempty" firestore:"voiceoverId,omitempty"`
	VoiceoverURL    string  `json:"voiceoverUrl,omitempty" firestore:"voiceoverUrl,omitempty"`
	VoiceoverVolume float64 `json:"voiceoverVolume,omitempty" firestore:"voiceoverVolume,omitempty"` // 0..1
	DuckMusic       bool    `json:"duckMusic,omitempty" firestore:"duckMusic,omitempty"`
	CaptionSource   string  `json:"captionSource,omitempty" firestore:"captionSource,omitempty"`
}
