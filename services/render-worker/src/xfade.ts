/**
 * Which `xfade` transitions THIS FFmpeg can draw.
 *
 * ── Why this is probed and not a constant ───────────────────────────────────
 * The studio offers seven transitions and every one of them used to render as a
 * plain cross-fade, because the graph had `transition=fade` written into it.
 * xfade has had forty-odd named transitions for years, so the fix is to pass the
 * name through — but a name the local build does not know is not a degraded
 * render, it is a FAILED one: FFmpeg rejects the option and the whole encode
 * dies.
 *
 * And the build is not something this code can know. The container installs
 * Debian's `ffmpeg`, which tracks whatever the base image ships; a developer
 * running the worker locally has something else again; `hblur` arrived in 5.0
 * and the `wind` transitions later still. Hardcoding a list would make the
 * image's FFmpeg version a silent dependency of the feature, discovered by a
 * user whose render failed.
 *
 * So the worker asks. `ffmpeg -h filter=xfade` lists the names its own build
 * supports, once per process, and anything unrecognised falls back to a plain
 * cross-fade — which is exactly what the old behaviour was, so the worst case
 * is the behaviour we are replacing rather than a broken render.
 */
import { spawn } from "node:child_process";

/** Parsed once per process. A cold start pays one cheap subprocess. */
let supported: Set<string> | null = null;

/** The transition every build has, and the fallback for anything it lacks. */
export const PLAIN_CROSSFADE = "fade";

function parseNames(help: string): Set<string> {
    const names = new Set<string>();
    // Lines look like:  "     wiperight       2            ..FV....... wipe right transition"
    // Anchored on the numeric enum value so option names elsewhere in the help
    // text (duration, offset, expr) cannot be mistaken for transitions.
    for (const line of help.split("\n")) {
        const m = /^\s{4,}([a-z][a-z0-9]*)\s+-?\d+\s+\.\./.exec(line);
        if (m) names.add(m[1]);
    }
    return names;
}

export async function supportedTransitions(): Promise<Set<string>> {
    if (supported) return supported;
    supported = await new Promise<Set<string>>((resolve) => {
        try {
            // Bare name from PATH, as every other ffmpeg call in the worker does.
            const p = spawn("ffmpeg", ["-hide_banner", "-h", "filter=xfade"]);
            let out = "";
            // FFmpeg writes filter help to stdout, but has moved between the
            // two across versions, so both are read.
            p.stdout.on("data", (d) => (out += String(d)));
            p.stderr.on("data", (d) => (out += String(d)));
            p.on("error", () => resolve(new Set([PLAIN_CROSSFADE])));
            p.on("close", () => {
                const names = parseNames(out);
                // An empty parse means the help format changed, not that the
                // build has no transitions. Falling back to the one name that
                // has always existed keeps renders working either way.
                resolve(names.size > 0 ? names : new Set([PLAIN_CROSSFADE]));
            });
        } catch {
            resolve(new Set([PLAIN_CROSSFADE]));
        }
    });
    return supported;
}

/**
 * The transition name to put in the graph: the one asked for if this build has
 * it, a plain cross-fade otherwise.
 */
export function resolveTransition(style: string | undefined, available: Set<string>): string {
    if (!style) return PLAIN_CROSSFADE;
    return available.has(style) ? style : PLAIN_CROSSFADE;
}

/** Exposed for tests; the parse is the part worth asserting on. */
export const __parseNames = parseNames;
