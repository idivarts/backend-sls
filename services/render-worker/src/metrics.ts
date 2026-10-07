/**
 * Operational telemetry for a render, emitted as CloudWatch Embedded Metric
 * Format on stdout.
 *
 * EMF rather than a metrics SDK or a Sentry client: the worker runs in Lambda
 * and in Fargate, both of which already ship stdout to CloudWatch, so printing
 * one JSON line per render gives real metrics (dashboards, alarms on failure
 * rate, p95 duration) with no dependency, no network call on the hot path, and
 * nothing to fail when the metrics backend is unreachable.
 *
 * The one alarm worth wiring first is RenderFailed > 0 sustained: everything
 * else degrades visibly in the app, but a worker failing every job looks, from
 * inside the product, exactly like nobody rendering anything.
 */
const NAMESPACE = "Trendly/Render";

type Dimensions = Record<string, string>;

function emit(metrics: Record<string, number>, dimensions: Dimensions, extra: Record<string, unknown> = {}) {
    const names = Object.keys(metrics);
    if (!names.length) return;
    const line = {
        _aws: {
            Timestamp: Date.now(),
            CloudWatchMetrics: [
                {
                    Namespace: NAMESPACE,
                    // Dimensions are kept deliberately low-cardinality: docType
                    // and outcome only. A brandId dimension would multiply the
                    // metric count by the number of customers and bill for it.
                    Dimensions: [Object.keys(dimensions)],
                    Metrics: names.map((name) => ({
                        Name: name,
                        Unit: name.endsWith("Ms") ? "Milliseconds" : "Count",
                    })),
                },
            ],
        },
        ...dimensions,
        ...metrics,
        ...extra,
    };
    console.log(JSON.stringify(line));
}

export function renderSucceeded(opts: {
    docType: "image" | "video";
    durationMs: number;
    outputs: number;
    bytes: number;
    revisionId: string;
}) {
    emit(
        {
            RenderSucceeded: 1,
            RenderDurationMs: opts.durationMs,
            RenderOutputs: opts.outputs,
            RenderBytes: opts.bytes,
        },
        { DocType: opts.docType, Outcome: "success" },
        { revisionId: opts.revisionId }
    );
}

export function renderFailed(opts: {
    docType: "image" | "video";
    durationMs: number;
    revisionId: string;
    reason: string;
}) {
    emit(
        { RenderFailed: 1, RenderDurationMs: opts.durationMs },
        { DocType: opts.docType, Outcome: "failure" },
        // The reason rides as a log FIELD, not a dimension — it is unbounded
        // text, and a dimension per distinct error message would be a bill.
        { revisionId: opts.revisionId, reason: opts.reason }
    );
}
