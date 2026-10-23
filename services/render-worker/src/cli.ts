/**
 * Video path entrypoint: the AWS Batch container command.
 *
 * Batch passes the job as a single JSON argument or via RENDER_JOB, runs it
 * once, and the task exits — which is what keeps Fargate costing nothing when
 * no renders are queued. A non-zero exit lets Batch's retryStrategy take over,
 * including after a Spot interruption.
 */
import { closeBrowser } from "./browser";
import { runJob } from "./render";
import type { RenderJob } from "./types";

async function main() {
    const raw = process.argv[2] || process.env.RENDER_JOB;
    if (!raw) throw new Error("no render job: pass JSON as argv[2] or RENDER_JOB");
    const job = JSON.parse(raw) as RenderJob;
    const started = Date.now();
    await runJob({ ...job, jobId: job.jobId ?? process.env.AWS_BATCH_JOB_ID });
    console.log(`[render] ${job.revisionId} done in ${Date.now() - started}ms`);
}

main()
    .then(async () => {
        await closeBrowser();
        process.exit(0);
    })
    .catch(async (err) => {
        console.error(err);
        await closeBrowser();
        process.exit(1);
    });
