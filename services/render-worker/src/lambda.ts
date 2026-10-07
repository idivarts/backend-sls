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
import { closeBrowser } from "./browser";
import { runJob } from "./render";
import type { RenderJob } from "./types";

interface SQSRecord {
    body: string;
    messageId: string;
}
interface SQSEvent {
    Records: SQSRecord[];
}

/** The slice of the Lambda context object this worker actually reads. Supplied by lambda-bootstrap. */
export interface LambdaContext {
    awsRequestId?: string;
    invokedFunctionArn?: string;
    getRemainingTimeInMillis?: () => number;
}

export async function handler(event: SQSEvent, context?: LambdaContext) {
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
