/** The unit of work: render one revision into publishable media. */
export interface RenderJob {
    brandId: string;
    contentId: string;
    /**
     * The revision to render AND the document that carries its render status.
     * A design revision for "image"/"video", a composition revision for
     * "composition" — one id, because the two cases never need both.
     */
    revisionId: string;
    /**
     * "image" renders one PNG per slide; "video" renders a single MP4 from one
     * design; "composition" renders an authored timeline.
     *
     * "video" is not the pre-composition path kept alive beside a new one — it
     * goes through the composition renderer too, as a one-clip composition
     * derived from the design in memory. The timeline is the only primitive;
     * this field only says where the job reads from and writes back to.
     */
    docType: "image" | "video" | "composition";
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
    /** Animation length for a video design; 0 or absent for an image. */
    durationMs?: number;
    renderUrl?: string;
    renderStatus?: string;
}

/**
 * Where a job's status is written. A design revision and a composition
 * revision carry identical render fields in two different subcollections, so
 * the only thing the writers need to know is which one.
 */
export interface RenderTarget {
    brandId: string;
    contentId: string;
    revisionId: string;
    kind: "design" | "composition";
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
