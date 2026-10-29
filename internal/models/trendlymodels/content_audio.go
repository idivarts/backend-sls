package trendlymodels

// ContentAudio is the generated audio attached to a video content, muxed in at
// render time. References generatedAudio docs by id/url.
type ContentAudio struct {
	MusicID         string  `json:"musicId,omitempty" firestore:"musicId,omitempty"`
	MusicURL        string  `json:"musicUrl,omitempty" firestore:"musicUrl,omitempty"`
	MusicTitle      string  `json:"musicTitle,omitempty" firestore:"musicTitle,omitempty"`
	MusicVolume     float64 `json:"musicVolume,omitempty" firestore:"musicVolume,omitempty"` // 0..1
	MusicFade       bool    `json:"musicFade,omitempty" firestore:"musicFade,omitempty"`     // fade in/out
	VoiceoverID     string  `json:"voiceoverId,omitempty" firestore:"voiceoverId,omitempty"`
	VoiceoverURL    string  `json:"voiceoverUrl,omitempty" firestore:"voiceoverUrl,omitempty"`
	VoiceoverVolume float64 `json:"voiceoverVolume,omitempty" firestore:"voiceoverVolume,omitempty"` // 0..1
	// MusicMs / VoiceoverMs are the clips' ACTUAL lengths, measured by the app
	// when the content is open and written back. A generated length is only
	// what was asked for, and TTS length is not knowable from the text at all —
	// so without these the backend cannot tell whether a voiceover overruns
	// the video, which is the commonest way a render comes out wrong.
	MusicMs     int64 `json:"musicMs,omitempty" firestore:"musicMs,omitempty"`
	VoiceoverMs int64 `json:"voiceoverMs,omitempty" firestore:"voiceoverMs,omitempty"`
	// SceneBoundaries is where each scene's line actually falls in the
	// voiceover, measured from the provider's character-level timings at
	// generation. It is what lets the SCENES be retimed to the voice instead
	// of the script being trimmed until it happens to fit.
	SceneBoundaries []SceneBoundary `json:"sceneBoundaries,omitempty" firestore:"sceneBoundaries,omitempty"`
	// SceneTimingsExact is false when the provider normalised the text before
	// speaking it, so the boundaries were scaled rather than looked up. Worth
	// carrying: an approximate retime is still useful, and presenting it as
	// exact is not.
	SceneTimingsExact bool   `json:"sceneTimingsExact,omitempty" firestore:"sceneTimingsExact,omitempty"`
	DuckMusic         bool   `json:"duckMusic,omitempty" firestore:"duckMusic,omitempty"`
	CaptionSource     string `json:"captionSource,omitempty" firestore:"captionSource,omitempty"`
}

// SceneBoundary is where one scene's line sits inside the voiceover.
type SceneBoundary struct {
	Scene   int `json:"scene" firestore:"scene"`
	StartMs int `json:"startMs" firestore:"startMs"`
	EndMs   int `json:"endMs" firestore:"endMs"`
}
