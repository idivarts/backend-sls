/**
 * Image path entrypoint: an SQS-triggered Lambda running the same container as
 * the video worker.
 *
 * Images get Lambda rather than Batch because of billing shape, not capability:
 * Fargate bills a one-minute minimum AND bills the container image pull, so a
 * four-second carousel would cost like a sixty-second job and make the user
 * wait ~35 seconds before rendering even starts.
 *
 * batchSize is 1, so a thrown error fails exactly one message and SQS redrives
 * it; the DLQ catches anything that keeps failing.
 */
import { closeBrowser, getBrowser } from "./browser";
import { runPreview, type PreviewRequest, type PreviewResponse } from "./preview";
import { runJob } from "./render";
import type { RenderJob } from "./types";

interface SQSRecord {
    body: string;
    messageId: string;
}
interface SQSEvent {
    Records: SQSRecord[];
}

/**
 * The second thing this container does: a SYNCHRONOUS preview render, invoked
 * directly (RequestResponse) rather than through the queue, so the AI can look
 * at a design inside the chat turn that authored it.
 *
 * It shares the image path's warm Chromium, which is the whole reason it lives
 * here rather than in its own service — a cold browser would cost more than the
 * render. It is deployed as its own FUNCTION, though, so an inspection never
 * waits behind a queue of publish renders.
 */
interface PreviewEvent extends PreviewRequest {
    kind: "preview";
}

type WorkerEvent = SQSEvent | PreviewEvent;

function isPreview(event: WorkerEvent): event is PreviewEvent {
    return (event as PreviewEvent).kind === "preview";
}

/** The slice of the Lambda context object this worker actually reads. Supplied by lambda-bootstrap. */
export interface LambdaContext {
    awsRequestId?: string;
    invokedFunctionArn?: string;
    getRemainingTimeInMillis?: () => number;
}

export async function handler(event: WorkerEvent, context?: LambdaContext): Promise<unknown> {
    // A preview is answered inline: the caller is a chat turn waiting on it, so
    // a thrown error would surface as a dead tool call. Report it as data and
    // let the model read why it could not see its design.
    if (isPreview(event)) {
        try {
            return await runPreview(await getBrowser(), event);
        } catch (err) {
            console.error("[preview] failed:", err);
            const res: PreviewResponse = {
                ok: false,
                error: err instanceof Error ? err.message : String(err),
            };
            return res;
        }
    }

    for (const record of event.Records) {
        const job = JSON.parse(record.body) as RenderJob;
        await runJob({ ...job, jobId: job.jobId ?? context?.awsRequestId });
    }
    // The browser is deliberately NOT closed between invocations: a warm Lambda
    // reuses it and saves a second or two per render. It is closed only when the
    // execution environment is torn down.
    return { ok: true };
}

process.on("SIGTERM", () => {
    void closeBrowser();
});
