package n8n

type N8NInfluencer struct {
	Biography            string                              `json:"biography"`
	BusinessCategoryName string                              `json:"businessCategoryName"`
	Error                string                              `json:"error"`
	ErrorDescription     string                              `json:"errorDescription"`
	ExternalUrl          string                              `json:"externalUrl"`
	ExternalUrlShimmed   string                              `json:"externalUrlShimmed"`
	ExternalUrls         []N8NInfluencerExternalUrlsItem     `json:"externalUrls"`
	Fbid                 string                              `json:"fbid"`
	FollowersCount       float64                             `json:"followersCount"`
	FollowsCount         float64                             `json:"followsCount"`
	FullName             string                              `json:"fullName"`
	HasChannel           bool                                `json:"hasChannel"`
	HighlightReelCount   float64                             `json:"highlightReelCount"`
	Id                   string                              `json:"id"`
	IgtvVideoCount       float64                             `json:"igtvVideoCount"`
	InputUrl             string                              `json:"inputUrl"`
	IsBusinessAccount    bool                                `json:"isBusinessAccount"`
	IsRestrictedProfile  bool                                `json:"isRestrictedProfile"`
	JoinedRecently       bool                                `json:"joinedRecently"`
	LatestIgtvVideos     []N8NInfluencerLatestIgtvVideosItem `json:"latestIgtvVideos"`
	LatestPosts          []N8NInfluencerLatestPostsItem      `json:"latestPosts"`
	PostsCount           float64                             `json:"postsCount"`
	Private              bool                                `json:"private"`
	ProfilePicUrl        string                              `json:"profilePicUrl"`
	ProfilePicUrlHD      string                              `json:"profilePicUrlHD"`
	RelatedProfiles      []N8NInfluencerRelatedProfilesItem  `json:"relatedProfiles"`
	RestrictionReason    string                              `json:"restrictionReason"`
	Url                  string                              `json:"url"`
	Username             string                              `json:"username"`
	Verified             bool                                `json:"verified"`
}

type N8NInfluencerExternalUrlsItem struct {
	LinkType string `json:"link_type"`
	LynxUrl  string `json:"lynx_url"`
	Title    string `json:"title"`
	Url      string `json:"url"`
}

type N8NInfluencerLatestIgtvVideosItem struct {
	Alt                interface{}                                        `json:"alt"`
	Caption            string                                             `json:"caption"`
	ChildPosts         []interface{}                                      `json:"childPosts"`
	CommentsCount      float64                                            `json:"commentsCount"`
	CommentsDisabled   bool                                               `json:"commentsDisabled"`
	DimensionsHeight   float64                                            `json:"dimensionsHeight"`
	DimensionsWidth    float64                                            `json:"dimensionsWidth"`
	DisplayUrl         string                                             `json:"displayUrl"`
	FirstComment       string                                             `json:"firstComment"`
	Hashtags           []string                                           `json:"hashtags"`
	Id                 string                                             `json:"id"`
	Images             []interface{}                                      `json:"images"`
	IsCommentsDisabled bool                                               `json:"isCommentsDisabled"`
	LatestComments     []interface{}                                      `json:"latestComments"`
	LikesCount         float64                                            `json:"likesCount"`
	LocationId         string                                             `json:"locationId"`
	LocationName       string                                             `json:"locationName"`
	Mentions           []string                                           `json:"mentions"`
	OwnerId            string                                             `json:"ownerId"`
	OwnerUsername      string                                             `json:"ownerUsername"`
	ProductType        string                                             `json:"productType"`
	ShortCode          string                                             `json:"shortCode"`
	TaggedUsers        []N8NInfluencerLatestIgtvVideosItemTaggedUsersItem `json:"taggedUsers"`
	Timestamp          string                                             `json:"timestamp"`
	Title              string                                             `json:"title"`
	Type               string                                             `json:"type"`
	Url                string                                             `json:"url"`
	VideoDuration      float64                                            `json:"videoDuration"`
	VideoUrl           string                                             `json:"videoUrl"`
	VideoViewCount     float64                                            `json:"videoViewCount"`
}

type N8NInfluencerLatestIgtvVideosItemTaggedUsersItem struct {
	FullName      string `json:"full_name"`
	Id            string `json:"id"`
	IsVerified    bool   `json:"is_verified"`
	ProfilePicUrl string `json:"profile_pic_url"`
	Username      string `json:"username"`
}

type N8NInfluencerLatestPostsItem struct {
	Alt                string                                        `json:"alt"`
	Caption            string                                        `json:"caption"`
	ChildPosts         []N8NInfluencerLatestPostsItemChildPostsItem  `json:"childPosts"`
	CommentsCount      float64                                       `json:"commentsCount"`
	DimensionsHeight   float64                                       `json:"dimensionsHeight"`
	DimensionsWidth    float64                                       `json:"dimensionsWidth"`
	DisplayUrl         string                                        `json:"displayUrl"`
	Hashtags           []string                                      `json:"hashtags"`
	Id                 string                                        `json:"id"`
	Images             []string                                      `json:"images"`
	IsCommentsDisabled bool                                          `json:"isCommentsDisabled"`
	IsPinned           bool                                          `json:"isPinned"`
	LikesCount         float64                                       `json:"likesCount"`
	LocationId         string                                        `json:"locationId"`
	LocationName       string                                        `json:"locationName"`
	Mentions           []string                                      `json:"mentions"`
	MusicInfo          N8NInfluencerLatestPostsItemMusicInfo         `json:"musicInfo"`
	OwnerId            string                                        `json:"ownerId"`
	OwnerUsername      string                                        `json:"ownerUsername"`
	ProductType        string                                        `json:"productType"`
	ShortCode          string                                        `json:"shortCode"`
	TaggedUsers        []N8NInfluencerLatestPostsItemTaggedUsersItem `json:"taggedUsers"`
	Timestamp          string                                        `json:"timestamp"`
	Type               string                                        `json:"type"`
	Url                string                                        `json:"url"`
	VideoUrl           string                                        `json:"videoUrl"`
	VideoViewCount     float64                                       `json:"videoViewCount"`
}

type N8NInfluencerLatestPostsItemChildPostsItem struct {
	Alt              string                                                      `json:"alt"`
	Caption          string                                                      `json:"caption"`
	ChildPosts       []interface{}                                               `json:"childPosts"`
	CommentsCount    float64                                                     `json:"commentsCount"`
	DimensionsHeight float64                                                     `json:"dimensionsHeight"`
	DimensionsWidth  float64                                                     `json:"dimensionsWidth"`
	DisplayUrl       string                                                      `json:"displayUrl"`
	FirstComment     string                                                      `json:"firstComment"`
	Hashtags         []interface{}                                               `json:"hashtags"`
	Id               string                                                      `json:"id"`
	Images           []interface{}                                               `json:"images"`
	LatestComments   []interface{}                                               `json:"latestComments"`
	LikesCount       interface{}                                                 `json:"likesCount"`
	Mentions         []interface{}                                               `json:"mentions"`
	OwnerId          string                                                      `json:"ownerId"`
	OwnerUsername    string                                                      `json:"ownerUsername"`
	ShortCode        string                                                      `json:"shortCode"`
	TaggedUsers      []N8NInfluencerLatestPostsItemChildPostsItemTaggedUsersItem `json:"taggedUsers"`
	Timestamp        interface{}                                                 `json:"timestamp"`
	Type             string                                                      `json:"type"`
	Url              string                                                      `json:"url"`
	VideoUrl         string                                                      `json:"videoUrl"`
	VideoViewCount   float64                                                     `json:"videoViewCount"`
}

type N8NInfluencerLatestPostsItemChildPostsItemTaggedUsersItem struct {
	FullName      string `json:"full_name"`
	Id            string `json:"id"`
	IsVerified    bool   `json:"is_verified"`
	ProfilePicUrl string `json:"profile_pic_url"`
	Username      string `json:"username"`
}

type N8NInfluencerLatestPostsItemMusicInfo struct {
	ArtistName            string `json:"artist_name"`
	AudioId               string `json:"audio_id"`
	ShouldMuteAudio       bool   `json:"should_mute_audio"`
	ShouldMuteAudioReason string `json:"should_mute_audio_reason"`
	SongName              string `json:"song_name"`
	UsesOriginalAudio     bool   `json:"uses_original_audio"`
}

type N8NInfluencerLatestPostsItemTaggedUsersItem struct {
	FullName      string `json:"full_name"`
	Id            string `json:"id"`
	IsVerified    bool   `json:"is_verified"`
	ProfilePicUrl string `json:"profile_pic_url"`
	Username      string `json:"username"`
}

type N8NInfluencerRelatedProfilesItem struct {
	FullName      string `json:"full_name"`
	Id            string `json:"id"`
	IsPrivate     bool   `json:"is_private"`
	IsVerified    bool   `json:"is_verified"`
	ProfilePicUrl string `json:"profile_pic_url"`
	Username      string `json:"username"`
}
