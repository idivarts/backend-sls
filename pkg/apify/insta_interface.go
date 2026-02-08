package apify

type InstagramInfluencer struct {
	Biography            string                     `json:"biography"`
	BusinessCategoryName string                     `json:"businessCategoryName"`
	Error                string                     `json:"error"`
	ErrorDescription     string                     `json:"errorDescription"`
	ExternalUrl          string                     `json:"externalUrl"`
	ExternalUrlShimmed   string                     `json:"externalUrlShimmed"`
	ExternalUrls         []InstagramExternalUrls    `json:"externalUrls"`
	Fbid                 string                     `json:"fbid"`
	FollowersCount       float64                    `json:"followersCount"`
	FollowsCount         float64                    `json:"followsCount"`
	FullName             string                     `json:"fullName"`
	HasChannel           bool                       `json:"hasChannel"`
	HighlightReelCount   float64                    `json:"highlightReelCount"`
	Id                   string                     `json:"id"`
	IgtvVideoCount       float64                    `json:"igtvVideoCount"`
	InputUrl             string                     `json:"inputUrl"`
	IsBusinessAccount    bool                       `json:"isBusinessAccount"`
	IsRestrictedProfile  bool                       `json:"isRestrictedProfile"`
	JoinedRecently       bool                       `json:"joinedRecently"`
	LatestIgtvVideos     []InstagramIgtvVideos      `json:"latestIgtvVideos"`
	LatestPosts          []InstagramPosts           `json:"latestPosts"`
	PostsCount           float64                    `json:"postsCount"`
	Private              bool                       `json:"private"`
	ProfilePicUrl        string                     `json:"profilePicUrl"`
	ProfilePicUrlHD      string                     `json:"profilePicUrlHD"`
	RelatedProfiles      []InstagramRelatedProfiles `json:"relatedProfiles"`
	RestrictionReason    string                     `json:"restrictionReason"`
	Url                  string                     `json:"url"`
	Username             string                     `json:"username"`
	Verified             bool                       `json:"verified"`
}

type InstagramExternalUrls struct {
	LinkType string `json:"link_type"`
	LynxUrl  string `json:"lynx_url"`
	Title    string `json:"title"`
	Url      string `json:"url"`
}

type InstagramIgtvVideos struct {
	Caption            string                 `json:"caption"`
	CommentsCount      float64                `json:"commentsCount"`
	CommentsDisabled   bool                   `json:"commentsDisabled"`
	DimensionsHeight   float64                `json:"dimensionsHeight"`
	DimensionsWidth    float64                `json:"dimensionsWidth"`
	DisplayUrl         string                 `json:"displayUrl"`
	FirstComment       string                 `json:"firstComment"`
	Hashtags           []string               `json:"hashtags"`
	Id                 string                 `json:"id"`
	IsCommentsDisabled bool                   `json:"isCommentsDisabled"`
	LikesCount         float64                `json:"likesCount"`
	LocationId         string                 `json:"locationId"`
	LocationName       string                 `json:"locationName"`
	Mentions           []string               `json:"mentions"`
	OwnerId            string                 `json:"ownerId"`
	OwnerUsername      string                 `json:"ownerUsername"`
	ProductType        string                 `json:"productType"`
	ShortCode          string                 `json:"shortCode"`
	TaggedUsers        []InstagramTaggedUsers `json:"taggedUsers"`
	Timestamp          string                 `json:"timestamp"`
	Title              string                 `json:"title"`
	Type               string                 `json:"type"`
	Url                string                 `json:"url"`
	VideoDuration      float64                `json:"videoDuration"`
	VideoUrl           string                 `json:"videoUrl"`
	VideoViewCount     float64                `json:"videoViewCount"`
}

type InstagramTaggedUsers struct {
	FullName      string `json:"full_name"`
	Id            string `json:"id"`
	IsVerified    bool   `json:"is_verified"`
	ProfilePicUrl string `json:"profile_pic_url"`
	Username      string `json:"username"`
}

type InstagramPosts struct {
	Alt                string                 `json:"alt"`
	Caption            string                 `json:"caption"`
	ChildPosts         []InstagramPosts       `json:"childPosts"`
	CommentsCount      float64                `json:"commentsCount"`
	DimensionsHeight   float64                `json:"dimensionsHeight"`
	DimensionsWidth    float64                `json:"dimensionsWidth"`
	DisplayUrl         string                 `json:"displayUrl"`
	Hashtags           []string               `json:"hashtags"`
	Id                 string                 `json:"id"`
	Images             []string               `json:"images"`
	IsCommentsDisabled bool                   `json:"isCommentsDisabled"`
	IsPinned           bool                   `json:"isPinned"`
	LikesCount         float64                `json:"likesCount"`
	LocationId         string                 `json:"locationId"`
	LocationName       string                 `json:"locationName"`
	Mentions           []string               `json:"mentions"`
	MusicInfo          InstagramMusicInfo     `json:"musicInfo"`
	OwnerId            string                 `json:"ownerId"`
	OwnerUsername      string                 `json:"ownerUsername"`
	ProductType        string                 `json:"productType"`
	ShortCode          string                 `json:"shortCode"`
	TaggedUsers        []InstagramTaggedUsers `json:"taggedUsers"`
	Timestamp          string                 `json:"timestamp"`
	Type               string                 `json:"type"`
	Url                string                 `json:"url"`
	VideoUrl           string                 `json:"videoUrl"`
	VideoViewCount     float64                `json:"videoViewCount"`
}

type InstagramMusicInfo struct {
	ArtistName            string `json:"artist_name"`
	AudioId               string `json:"audio_id"`
	ShouldMuteAudio       bool   `json:"should_mute_audio"`
	ShouldMuteAudioReason string `json:"should_mute_audio_reason"`
	SongName              string `json:"song_name"`
	UsesOriginalAudio     bool   `json:"uses_original_audio"`
}

type InstagramRelatedProfiles struct {
	FullName      string `json:"full_name"`
	Id            string `json:"id"`
	IsPrivate     bool   `json:"is_private"`
	IsVerified    bool   `json:"is_verified"`
	ProfilePicUrl string `json:"profile_pic_url"`
	Username      string `json:"username"`
}

type InstagramScraperInput struct {
	DirectUrls    []string `json:"directUrls"`
	ResultsType   string   `json:"resultsType"`
	ResultsLimit  int      `json:"resultsLimit"`
	AddParentData bool     `json:"addParentData"`
	SearchType    string   `json:"searchType,omitempty"`
	SearchLimit   int      `json:"searchLimit,omitempty"`
}
