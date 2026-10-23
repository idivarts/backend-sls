/** The unit of work: render one design revision into publishable media. */
export interface RenderJob {
    brandId: string;
    contentId: string;
    revisionId: string;
    /** "image" renders one PNG per slide; "video" renders a single MP4. */
    docType: "image" | "video";
    /**
     * Re-render a revision that already has a renderUrl. Off by default so a
     * duplicate queue delivery is a no-op rather than a second charge.
     */
    force?: boolean;
    /** Opaque id (Batch job / Lambda request) recorded for support. */
    jobId?: string;
}

/** What the worker found in Firestore and is about to render. */
export interface DesignRevision {
    html: string;
    width: number;
    height: number;
    slideCount: number;
    docType: "image" | "video";
    renderUrl?: string;
    renderStatus?: string;
}

/** Soundtrack to mux into a video render. */
export interface ContentAudio {
    musicUrl?: string;
    voiceoverUrl?: string;
    musicVolume?: number;
    voiceoverVolume?: number;
    duckMusic?: boolean;
}

export interface RenderResult {
    /** Public CDN urls, slide order for images; one entry for video. */
    urls: string[];
    /** Poster frame for a video render. */
    posterUrl?: string;
}
