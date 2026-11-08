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
     * design; "composition" renders an authored timeline; "ingest" looks at an
     * uploaded video and records what is in it.
     *
     * "frame" cuts one still out of a video and files it as its own media,
     * which is how a chained generation gets its start frame.
     *
     * "studio" renders a Design Studio: the worker reads the studio and its
     * scenes, converts them into a composition, and renders THAT. There is no
     * second renderer — the composition path already gets the frame-grid
     * arithmetic a cross-fade depends on right, and two implementations of it
     * is two places for a half-frame error to make a dissolve fade from
     * nothing.
     *
     * "video" is not the pre-composition path kept alive beside a new one — it
     * goes through the composition renderer too, as a one-clip composition
     * derived from the design in memory. The timeline is the only primitive;
     * this field only says where the job reads from and writes back to.
     */
    docType: "image" | "video" | "composition" | "ingest" | "frame" | "studio";
    /**
     * Re-render a revision that already has a renderUrl. Off by default so a
     * duplicate queue delivery is a no-op rather than a second charge.
     */
    force?: boolean;
    /** Opaque id (Batch job / Lambda request) recorded for support. */
    jobId?: string;
    /**
     * For an ingest or frame job: the video to look at. `revisionId` carries
     * the asset id in that case — one field for "the document this job is
     * about", so the status writers do not need a second shape.
     */
    sourceUrl?: string;
    /**
     * For a frame job: how far into the source to cut. Negative (the caller
     * sends -1) means the LAST frame, which the worker has to compute because
     * only it can probe the real length.
     */
    atMs?: number;
    /**
     * For a studio job: which studio. `revisionId` carries it too, for the
     * status writers, but naming it explicitly keeps the job readable.
     */
    studioId?: string;
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

/**
 * An ingest job: look at one uploaded video and record what is in it.
 *
 * Carries the asset rather than a revision, because ingest describes a piece
 * of FOOTAGE in the brand's library, not a design belonging to one post.
 */
export interface IngestJob {
    brandId: string;
    assetId: string;
    url: string;
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
