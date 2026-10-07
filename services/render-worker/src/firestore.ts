/**
 * Firestore access for the worker.
 *
 * This is the Node mirror of `internal/models/trendlymodels/content_design.go`.
 * The repo's standing rule is that every collection's reads and writes live
 * behind one model — the worker runs in Node, so it cannot call the Go
 * wrappers, and this file is the agreed second implementation. It exists to
 * keep that exception CONTAINED: nothing else in the worker may touch
 * `getFirestore()` directly, and every field name here must match the Go
 * struct tags. Change one, change both.
 */
import { cert, getApps, initializeApp } from "firebase-admin/app";
import { FieldValue, getFirestore, Firestore } from "firebase-admin/firestore";
import { config } from "./config";
import type { ContentAudio, DesignRevision } from "./types";

export const RenderStatus = {
    Queued: "queued",
    Rendering: "rendering",
    Done: "done",
    Failed: "failed",
} as const;

let db: Firestore | null = null;

/**
 * Lazily initialise the Admin SDK against the DATABASE the stage points at.
 * dev and prod are two named databases inside one Firebase project, selected by
 * FIRESTORE_DATABASE_ID — the same variable the Go backend and the apps read.
 */
function firestore(): Firestore {
    if (db) return db;
    if (!getApps().length) {
        const inline = process.env.FIREBASE_SERVICE_ACCOUNT_JSON;
        initializeApp(inline ? { credential: cert(JSON.parse(inline)) } : undefined);
    }
    db = getFirestore(config.firestoreDatabaseId);
    return db;
}

const revisionRef = (brandId: string, contentId: string, revisionId: string) =>
    firestore().doc(`brands/${brandId}/contents/${contentId}/designs/${revisionId}`);

const contentRef = (brandId: string, contentId: string) =>
    firestore().doc(`brands/${brandId}/contents/${contentId}`);

export async function getRevision(
    brandId: string,
    contentId: string,
    revisionId: string
): Promise<DesignRevision | null> {
    const snap = await revisionRef(brandId, contentId, revisionId).get();
    if (!snap.exists) return null;
    const d = snap.data() as Record<string, unknown>;
    return {
        html: String(d.html ?? ""),
        width: Number(d.width ?? 1080),
        height: Number(d.height ?? 1350),
        slideCount: Math.max(Number(d.slideCount ?? 1), 1),
        docType: d.docType === "video" ? "video" : "image",
        renderUrl: d.renderUrl ? String(d.renderUrl) : undefined,
        renderStatus: d.renderStatus ? String(d.renderStatus) : undefined,
    };
}

/** The soundtrack lives on the CONTENT, not the revision — it outlives edits. */
export async function getContentAudio(
    brandId: string,
    contentId: string
): Promise<ContentAudio | undefined> {
    const snap = await contentRef(brandId, contentId).get();
    if (!snap.exists) return undefined;
    const audio = (snap.data() as Record<string, unknown>).audio as ContentAudio | undefined;
    if (!audio || (!audio.musicUrl && !audio.voiceoverUrl)) return undefined;
    return audio;
}

export async function markStarted(
    brandId: string,
    contentId: string,
    revisionId: string,
    jobId?: string
): Promise<void> {
    await revisionRef(brandId, contentId, revisionId).set(
        {
            renderStatus: RenderStatus.Rendering,
            renderStartedAt: Date.now(),
            ...(jobId ? { renderJobId: jobId } : {}),
        },
        { merge: true }
    );
}

/** Called roughly every 5% of frames — one field, so the write stays cheap. */
export async function setProgress(
    brandId: string,
    contentId: string,
    revisionId: string,
    progress: number
): Promise<void> {
    const clamped = Math.min(Math.max(progress, 0), 1);
    await revisionRef(brandId, contentId, revisionId).set(
        { renderProgress: clamped },
        { merge: true }
    );
}

/**
 * `reason` is shown to the user verbatim, so it must read as a sentence. The
 * stack trace belongs in the logs, not in the app.
 */
export async function markFailed(
    brandId: string,
    contentId: string,
    revisionId: string,
    reason: string
): Promise<void> {
    await revisionRef(brandId, contentId, revisionId).set(
        {
            renderStatus: RenderStatus.Failed,
            renderError: reason,
            renderProgress: FieldValue.delete(),
        },
        { merge: true }
    );
}

/**
 * Record the finished render, revision first and content second.
 *
 * Order matters: `renderUrl` on the revision is what the app reads as "this
 * exact design has been exported", and `attachments` on the content is what
 * actually publishes. Writing the revision first means a crash between the two
 * leaves a design marked rendered but unpublished — visible and retryable —
 * rather than attachments that claim to match a design they don't.
 */
export async function setResult(
    brandId: string,
    contentId: string,
    revisionId: string,
    urls: string[],
    docType: "image" | "video",
    posterUrl: string | undefined,
    scale: number,
    /** Per-slide thumbnail urls, index-aligned with `urls`. May be sparse. */
    thumbUrls: (string | undefined)[] = []
): Promise<void> {
    const cover = urls[0];
    await revisionRef(brandId, contentId, revisionId).set(
        {
            renderStatus: RenderStatus.Done,
            renderUrl: cover,
            renderedAt: Date.now(),
            renderScale: scale,
            renderError: FieldValue.delete(),
            renderProgress: FieldValue.delete(),
            ...(posterUrl ? { posterUrl } : {}),
        },
        { merge: true }
    );

    // One attachment per slide, in slide order — several image attachments are
    // what the publish pipeline reads as a carousel.
    const attachments =
        docType === "video"
            ? [{ type: "video", playUrl: cover, ...(posterUrl ? { thumbUrl: posterUrl } : {}) }]
            : urls.map((u, i) => ({
                  type: "image",
                  imageUrl: u,
                  ...(thumbUrls[i] ? { thumbUrl: thumbUrls[i] } : {}),
              }));

    await contentRef(brandId, contentId).set(
        {
            designRef: { renderUrl: cover },
            attachments,
            updatedAt: Date.now(),
        },
        { merge: true }
    );
}
