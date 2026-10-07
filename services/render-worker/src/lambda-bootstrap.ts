/**
 * Lambda bootstrap — the long-poll loop against the Lambda Runtime API.
 *
 * This is the job AWS's `aws-lambda-ric` package does, done here in ~90 lines
 * instead. The RIC is not used because it compiles curl AND aws-lambda-cpp from
 * source on install (cmake, autoconf, node-gyp), which pulls a C++ toolchain
 * into an image whose whole point is Chromium + FFmpeg, and adds minutes to
 * every CI build. RIC v4 also resolves the handler from `_HANDLER` relative to
 * `LAMBDA_TASK_ROOT`, neither of which points at our code in a container image.
 *
 * The protocol is four HTTP calls (https://docs.aws.amazon.com/lambda/latest/dg/runtimes-api.html):
 *   GET  /invocation/next          long-polls until there is work
 *   POST /invocation/{id}/response hands back the result
 *   POST /invocation/{id}/error    reports a failed invocation
 *   POST /init/error               reports a failure before the loop starts
 *
 * Go lambdas in this repo get the same loop from aws-lambda-go; this is the
 * Node equivalent, and it is the container's ENTRYPOINT for the image path.
 */
import * as http from "node:http";
import { handler, LambdaContext } from "./lambda";

const API = process.env.AWS_LAMBDA_RUNTIME_API;
const BASE = "/2018-06-01/runtime";

// keepAlive matters: the loop reuses one socket for the whole life of the
// execution environment instead of reconnecting on every invocation.
const agent = new http.Agent({ keepAlive: true, maxSockets: 1 });

interface Reply {
    status: number;
    headers: http.IncomingHttpHeaders;
    body: string;
}

function call(method: string, path: string, body?: string): Promise<Reply> {
    const [host, port] = String(API).split(":");
    return new Promise((resolve, reject) => {
        const req = http.request(
            {
                host,
                port: port ? Number(port) : 80,
                path,
                method,
                agent,
                headers: body
                    ? { "content-type": "application/json", "content-length": Buffer.byteLength(body) }
                    : {},
            },
            (res) => {
                let data = "";
                res.setEncoding("utf8");
                res.on("data", (chunk) => (data += chunk));
                res.on("end", () => resolve({ status: res.statusCode ?? 0, headers: res.headers, body: data }));
            },
        );
        req.on("error", reject);
        // Explicitly no socket timeout. /invocation/next blocks until Lambda has
        // work, which on a quiet queue is many minutes — this is the one reason
        // the loop uses node:http rather than fetch, whose undici default would
        // abort the poll after 300s.
        req.setTimeout(0);
        if (body) req.write(body);
        req.end();
    });
}

function errorBody(err: unknown): string {
    const e = err instanceof Error ? err : new Error(String(err));
    return JSON.stringify({
        errorType: e.name || "Error",
        errorMessage: e.message,
        stackTrace: (e.stack ?? "")
            .split("\n")
            .slice(1)
            .map((line) => line.trim()),
    });
}

async function loop() {
    for (;;) {
        const next = await call("GET", `${BASE}/invocation/next`);
        const requestId = String(next.headers["lambda-runtime-aws-request-id"] ?? "");
        // Nothing to answer — don't post a response keyed on an empty id.
        if (!requestId) continue;

        const trace = next.headers["lambda-runtime-trace-id"];
        if (typeof trace === "string") process.env._X_AMZN_TRACE_ID = trace;
        const deadline = Number(next.headers["lambda-runtime-deadline-ms"] ?? 0);

        const context: LambdaContext = {
            awsRequestId: requestId,
            invokedFunctionArn: String(next.headers["lambda-runtime-invoked-function-arn"] ?? ""),
            getRemainingTimeInMillis: () => Math.max(0, deadline - Date.now()),
        };

        try {
            const result = await handler(JSON.parse(next.body || "{}"), context);
            await call("POST", `${BASE}/invocation/${requestId}/response`, JSON.stringify(result ?? null));
        } catch (err) {
            // One message per invocation (batchSize: 1), so reporting the error
            // fails exactly this render and lets SQS redrive it.
            console.error("[render] invocation failed:", err);
            await call("POST", `${BASE}/invocation/${requestId}/error`, errorBody(err));
        }
    }
}

async function start() {
    if (!API) {
        throw new Error(
            "AWS_LAMBDA_RUNTIME_API is not set — lambda-bootstrap only runs inside Lambda. " +
                "For a one-off render use cli.js instead.",
        );
    }
    await loop();
}

start().catch(async (err) => {
    console.error("[render] bootstrap failed:", err);
    if (API) {
        // Best effort: if the loop never got going, Lambda wants to hear it here
        // so the init failure shows up on the function instead of as a bare exit.
        try {
            await call("POST", `${BASE}/init/error`, errorBody(err));
        } catch {
            /* the runtime API is what's broken; nothing left to report to */
        }
    }
    process.exit(1);
});
