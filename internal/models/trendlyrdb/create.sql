CREATE TABLE `socials` (
    -- Primary Key
    `id` VARCHAR(36) PRIMARY KEY COMMENT 'UUID generated from social_type + username',
    
    -- Basic Profile Information
    `username` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) DEFAULT NULL,
    `bio` TEXT DEFAULT NULL,
    `profile_pic` TEXT DEFAULT NULL,
    `profile_pic_hd` TEXT DEFAULT NULL,
    `category` VARCHAR(100) DEFAULT NULL,
    
    -- Social Platform Details
    `social_type` VARCHAR(50) NOT NULL COMMENT 'e.g., instagram, twitter, etc.',
    `profile_verified` BOOLEAN DEFAULT FALSE,
    
    -- Follower/Following Metrics
    `follower_count` BIGINT DEFAULT 0,
    `following_count` BIGINT DEFAULT 0,
    `content_count` BIGINT DEFAULT 0,
    
    -- Analytics/Metrics
    `views_count` BIGINT DEFAULT 0,
    `engagement_count` BIGINT DEFAULT 0,
    `engagement_rate` FLOAT DEFAULT 0.0,
    `average_views` FLOAT DEFAULT 0.0,
    `average_likes` FLOAT DEFAULT 0.0,
    `average_comments` FLOAT DEFAULT 0.0,
    
    -- Links (JSON array of {title, url, link_type})
    `links` JSON DEFAULT NULL,
    
    -- AI-Deduced Fields
    `gender` VARCHAR(50) DEFAULT NULL COMMENT 'Deduced from name, bio, and pronouns',
    `location` VARCHAR(255) DEFAULT NULL COMMENT 'Deduced from bio and posts location',
    `niches` JSON DEFAULT NULL COMMENT 'Array of niches deduced from posts, hashtags, and bio',
    `quality_score` INT DEFAULT NULL COMMENT 'Quality score: 60=Average, 75=Good, 90=Very Good, 100=Excellent',
    
    -- Metadata
    `added_by` VARCHAR(255) DEFAULT NULL,
    `creation_time` BIGINT DEFAULT NULL COMMENT 'Unix timestamp in milliseconds',
    `last_update_time` BIGINT DEFAULT NULL COMMENT 'Unix timestamp in milliseconds',
    
    -- Enhanced Profile
    `external_id` VARCHAR(255) DEFAULT NULL COMMENT 'External platform user ID',
    
    -- Indexes for common queries
    INDEX `idx_username_social_type` (`username`, `social_type`),
    INDEX `idx_social_type` (`social_type`),
    INDEX `idx_creation_time` (`creation_time`),
    INDEX `idx_last_update_time` (`last_update_time`),
    INDEX `idx_follower_count` (`follower_count`),
    INDEX `idx_external_id` (`external_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Social media profiles with analytics and AI-deduced metadata';

-- Add unique constraint on username + social_type combination
ALTER TABLE `socials` ADD UNIQUE KEY `unique_social_profile` (`username`, `social_type`);

-- Add check constraint for quality_score range (MySQL 8.0.16+)
ALTER TABLE `socials` ADD CONSTRAINT `chk_quality_score` 
    CHECK (`quality_score` IS NULL OR (`quality_score` >= 0 AND `quality_score` <= 100));

-- ============================================================================
-- Instagram Posts Table
-- ============================================================================

CREATE TABLE `instagram_posts` (
    -- Primary Key
    `id` VARCHAR(255) PRIMARY KEY COMMENT 'Instagram post ID',
    
    -- Foreign Key
    `social_id` VARCHAR(36) NOT NULL COMMENT 'References socials.id',
    
    -- Basic Post Information
    `post_location` VARCHAR(255) DEFAULT NULL,
    `type` VARCHAR(50) DEFAULT NULL COMMENT 'e.g., image, video, carousel',
    `short_code` VARCHAR(50) DEFAULT NULL COMMENT 'Instagram shortcode for the post',
    `caption` TEXT DEFAULT NULL,
    `url` TEXT DEFAULT NULL COMMENT 'Post URL',
    `display_url` TEXT DEFAULT NULL COMMENT 'Main display image URL',
    `video_url` TEXT DEFAULT NULL,
    
    -- Engagement Metrics
    `likes_count` BIGINT DEFAULT 0,
    `comments_count` BIGINT DEFAULT 0,
    `video_view_count` BIGINT DEFAULT 0,
    `video_play_count` BIGINT DEFAULT 0,
    `video_duration` DOUBLE DEFAULT NULL COMMENT 'Video duration in seconds',
    
    -- Timestamp and Location
    `timestamp` VARCHAR(50) DEFAULT NULL COMMENT 'Post timestamp',
    `location_name` VARCHAR(255) DEFAULT NULL,
    `location_id` VARCHAR(100) DEFAULT NULL,
    `is_pinned` BOOLEAN DEFAULT FALSE,
    
    -- Enhanced Fields
    `alt` TEXT DEFAULT NULL COMMENT 'Alt text for accessibility',
    `images` JSON DEFAULT NULL COMMENT 'Array of image URLs for carousel posts',
    `is_comments_disabled` BOOLEAN DEFAULT FALSE,
    `audio_url` TEXT DEFAULT NULL,
    `music_info` JSON DEFAULT NULL COMMENT 'Object: {artist_name, song_name, uses_original_audio, audio_id}',
    `hashtags` JSON DEFAULT NULL COMMENT 'Array of hashtags used in the post',
    `mentions` JSON DEFAULT NULL COMMENT 'Array of mentioned usernames',
    `tagged_users` JSON DEFAULT NULL COMMENT 'Array of User objects: {full_name, id, is_private, is_verified, profile_pic_url, username}',
    `first_comment` TEXT DEFAULT NULL,
    `latest_comments` JSON DEFAULT NULL COMMENT 'Array of Comment objects: {id, text, owner_username, owner_profile_pic_url, timestamp, likes_count}',
    `child_posts` JSON DEFAULT NULL COMMENT 'Array of nested InstagramPost objects for carousel items',
    
    -- Indexes for common queries
    INDEX `idx_social_id` (`social_id`),
    INDEX `idx_short_code` (`short_code`),
    INDEX `idx_timestamp` (`timestamp`),
    INDEX `idx_type` (`type`),
    INDEX `idx_likes_count` (`likes_count`),
    INDEX `idx_location_id` (`location_id`),
    
    -- Foreign Key Constraint
    CONSTRAINT `fk_instagram_posts_social_id` 
        FOREIGN KEY (`social_id`) 
        REFERENCES `socials`(`id`) 
        ON DELETE CASCADE 
        ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Instagram posts with engagement metrics and enhanced metadata';

-- Add unique constraint on short_code if needed
-- ALTER TABLE `instagram_posts` ADD UNIQUE KEY `unique_short_code` (`short_code`);

-- ============================================================================
-- Social Niches Junction Table
-- ============================================================================

CREATE TABLE `social_niches` (
    -- Composite Primary Key
    `social_id` VARCHAR(36) NOT NULL COMMENT 'References socials.id',
    `niche` VARCHAR(100) NOT NULL COMMENT 'Niche category (e.g., fashion, tech, fitness)',
    
    PRIMARY KEY (`social_id`, `niche`),
    
    -- Indexes
    INDEX `idx_niche` (`niche`),
    
    -- Foreign Key Constraint
    CONSTRAINT `fk_social_niches_social_id` 
        FOREIGN KEY (`social_id`) 
        REFERENCES `socials`(`id`) 
        ON DELETE CASCADE 
        ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Junction table for social profiles and their niches (many-to-many relationship)';