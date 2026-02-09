-- ============================================================================
-- PostgreSQL Schema for Social Media Analytics
-- ============================================================================

-- ============================================================================
-- Socials Table
-- ============================================================================

CREATE TABLE socials (
    -- Primary Key
    id VARCHAR(36) PRIMARY KEY,
    
    -- Basic Profile Information
    username VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    bio TEXT,
    profile_pic TEXT,
    profile_pic_hd TEXT,
    category VARCHAR(100),
    
    -- Social Platform Details
    social_type VARCHAR(50) NOT NULL,
    profile_verified BOOLEAN DEFAULT FALSE,
    
    -- Follower/Following Metrics
    follower_count BIGINT DEFAULT 0,
    following_count BIGINT DEFAULT 0,
    content_count BIGINT DEFAULT 0,
    
    -- Analytics/Metrics
    views_count BIGINT DEFAULT 0,
    engagement_count BIGINT DEFAULT 0,
    engagement_rate REAL DEFAULT 0.0,
    average_views REAL DEFAULT 0.0,
    average_likes REAL DEFAULT 0.0,
    average_comments REAL DEFAULT 0.0,
    
    -- Links (JSONB array of objects with structure: {title, url, link_type})
    links JSONB DEFAULT '[]'::JSONB,
    
    -- AI-Deduced Fields
    gender VARCHAR(50),
    location VARCHAR(255),
    niches TEXT[] DEFAULT '{}',  -- PostgreSQL text array for better querying
    quality_score INTEGER CHECK (quality_score IS NULL OR (quality_score >= 0 AND quality_score <= 100)),
    
    -- Metadata
    added_by VARCHAR(255),
    creation_time BIGINT,
    last_update_time BIGINT,
    
    -- Enhanced Profile
    external_id VARCHAR(255),
    
    -- Unique Constraint
    CONSTRAINT unique_social_profile UNIQUE (username, social_type)
);

-- Comments for documentation
COMMENT ON TABLE socials IS 'Social media profiles with analytics and AI-deduced metadata';
COMMENT ON COLUMN socials.id IS 'UUID generated from social_type + username';
COMMENT ON COLUMN socials.social_type IS 'e.g., instagram, twitter, etc.';
COMMENT ON COLUMN socials.links IS 'JSONB array of link objects: [{title, url, link_type}]';
COMMENT ON COLUMN socials.gender IS 'Deduced from name, bio, and pronouns';
COMMENT ON COLUMN socials.location IS 'Deduced from bio and posts location';
COMMENT ON COLUMN socials.niches IS 'Array of niches deduced from posts, hashtags, and bio';
COMMENT ON COLUMN socials.quality_score IS 'Quality score: 60=Average, 75=Good, 90=Very Good, 100=Excellent';
COMMENT ON COLUMN socials.creation_time IS 'Unix timestamp in milliseconds';
COMMENT ON COLUMN socials.last_update_time IS 'Unix timestamp in milliseconds';
COMMENT ON COLUMN socials.external_id IS 'External platform user ID';

-- Indexes for common queries
CREATE INDEX idx_socials_username_social_type ON socials(username, social_type);
CREATE INDEX idx_socials_social_type ON socials(social_type);
CREATE INDEX idx_socials_creation_time ON socials(creation_time);
CREATE INDEX idx_socials_last_update_time ON socials(last_update_time);
CREATE INDEX idx_socials_follower_count ON socials(follower_count);
CREATE INDEX idx_socials_external_id ON socials(external_id);

-- GIN index for array operations on niches (enables fast ANY/ALL queries)
CREATE INDEX idx_socials_niches ON socials USING GIN(niches);

-- GIN index for JSONB operations on links
CREATE INDEX idx_socials_links ON socials USING GIN(links);

-- ============================================================================
-- Instagram Posts Table
-- ============================================================================

CREATE TABLE instagram_posts (
    -- Primary Key
    id VARCHAR(255) PRIMARY KEY,
    
    -- Foreign Key
    social_id VARCHAR(36) NOT NULL REFERENCES socials(id) ON DELETE CASCADE ON UPDATE CASCADE,
    
    -- Basic Post Information
    post_location VARCHAR(255),
    type VARCHAR(50),
    short_code VARCHAR(50),
    caption TEXT,
    url TEXT,
    display_url TEXT,
    video_url TEXT,
    
    -- Engagement Metrics
    likes_count BIGINT DEFAULT 0,
    comments_count BIGINT DEFAULT 0,
    video_view_count BIGINT DEFAULT 0,
    video_play_count BIGINT DEFAULT 0,
    video_duration DOUBLE PRECISION,
    
    -- Timestamp and Location
    timestamp VARCHAR(50),
    location_name VARCHAR(255),
    location_id VARCHAR(100),
    is_pinned BOOLEAN DEFAULT FALSE,
    
    -- Enhanced Fields
    alt TEXT,
    images TEXT[] DEFAULT '{}',  -- PostgreSQL text array for image URLs
    is_comments_disabled BOOLEAN DEFAULT FALSE,
    audio_url TEXT,
    music_info JSONB,  -- Object: {artist_name, song_name, uses_original_audio, audio_id}
    hashtags TEXT[] DEFAULT '{}',  -- PostgreSQL text array for hashtags
    mentions TEXT[] DEFAULT '{}',  -- PostgreSQL text array for mentioned usernames
    tagged_users JSONB DEFAULT '[]'::JSONB,  -- Array of User objects
    first_comment TEXT,
    latest_comments JSONB DEFAULT '[]'::JSONB,  -- Array of Comment objects
    child_posts JSONB DEFAULT '[]'::JSONB  -- Array of nested InstagramPost objects for carousel items
);

-- Comments for documentation
COMMENT ON TABLE instagram_posts IS 'Instagram posts with engagement metrics and enhanced metadata';
COMMENT ON COLUMN instagram_posts.id IS 'Instagram post ID';
COMMENT ON COLUMN instagram_posts.social_id IS 'References socials.id';
COMMENT ON COLUMN instagram_posts.type IS 'e.g., image, video, carousel';
COMMENT ON COLUMN instagram_posts.short_code IS 'Instagram shortcode for the post';
COMMENT ON COLUMN instagram_posts.video_duration IS 'Video duration in seconds';
COMMENT ON COLUMN instagram_posts.timestamp IS 'Post timestamp';
COMMENT ON COLUMN instagram_posts.alt IS 'Alt text for accessibility';
COMMENT ON COLUMN instagram_posts.images IS 'Array of image URLs for carousel posts';
COMMENT ON COLUMN instagram_posts.music_info IS 'JSONB object: {artist_name, song_name, uses_original_audio, audio_id}';
COMMENT ON COLUMN instagram_posts.hashtags IS 'Array of hashtags used in the post';
COMMENT ON COLUMN instagram_posts.mentions IS 'Array of mentioned usernames';
COMMENT ON COLUMN instagram_posts.tagged_users IS 'JSONB array of User objects: [{full_name, id, is_private, is_verified, profile_pic_url, username}]';
COMMENT ON COLUMN instagram_posts.latest_comments IS 'JSONB array of Comment objects: [{id, text, owner_username, owner_profile_pic_url, timestamp, likes_count}]';
COMMENT ON COLUMN instagram_posts.child_posts IS 'JSONB array of nested InstagramPost objects for carousel items';

-- Indexes for common queries
CREATE INDEX idx_instagram_posts_social_id ON instagram_posts(social_id);
CREATE INDEX idx_instagram_posts_short_code ON instagram_posts(short_code);
CREATE INDEX idx_instagram_posts_timestamp ON instagram_posts(timestamp);
CREATE INDEX idx_instagram_posts_type ON instagram_posts(type);
CREATE INDEX idx_instagram_posts_likes_count ON instagram_posts(likes_count);
CREATE INDEX idx_instagram_posts_location_id ON instagram_posts(location_id);

-- GIN indexes for array operations
CREATE INDEX idx_instagram_posts_images ON instagram_posts USING GIN(images);
CREATE INDEX idx_instagram_posts_hashtags ON instagram_posts USING GIN(hashtags);
CREATE INDEX idx_instagram_posts_mentions ON instagram_posts USING GIN(mentions);

-- GIN indexes for JSONB operations
CREATE INDEX idx_instagram_posts_music_info ON instagram_posts USING GIN(music_info);
CREATE INDEX idx_instagram_posts_tagged_users ON instagram_posts USING GIN(tagged_users);
CREATE INDEX idx_instagram_posts_latest_comments ON instagram_posts USING GIN(latest_comments);
CREATE INDEX idx_instagram_posts_child_posts ON instagram_posts USING GIN(child_posts);

-- Optional: Add unique constraint on short_code if needed
-- CREATE UNIQUE INDEX unique_short_code ON instagram_posts(short_code);