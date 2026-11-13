/**
 * Resolving the media a film asked for.
 *
 * ── The shape of this ─────────────────────────────────────────────────────
 * The authoring model cannot fetch anything: it writes one HTML document in
 * a Lambda with no CLI. So it DECLARES what a scene needs —
 *
 *     <img data-media="image" data-media-intent="cold brew poured over ice">
 *     <audio data-media="bgm" data-media-intent="upbeat lo-fi, no vocals">
 *
 * — and this resolves each declaration with Hyperframes' own `media-use`,
 * which searches the HeyGen catalogue, generates where it must, and drops a
 * real file into the project's assets/ with a ledger entry. The declaration
 * is then rewritten into an ordinary src.
 *
 * ── Why here and not at authoring time ────────────────────────────────────
 * `media-use` is a CLI verb. The authoring job is Go, in a Lambda, with no
 * node and no Hyperframes. The worker already materialises the project,
 * already has the CLI, and already runs before both the preview and the
 * render — so a declaration is resolved once, on whichever of those comes
 * first, and the result is uploaded back so the other reuses it.
 *
 * ── Why the key is passed here and nowhere else ───────────────────────────
 * hyperframesEnv() builds the render subprocess's environment from an
 * allowlist precisely so a render cannot reach a paid API: "a render that
 * can reach a paid API is a render that can spend outside the wallet, with
 * no 402 in front of it". That argument still holds for `render`. Resolution
 * is the metered step, so the credential is added for THESE invocations
 * only, and a credit ceiling is written into the project first so the CLI
 * itself refuses to spend past what the wallet allows.
 */
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";

import { hyperframesEnv, runHyperframes } from "./hyperframes";
import { applyEdits, findElements, removeAttribute, setAttribute, type Edit } from "./html-edit";

/** The media kinds `media-use resolve --type` accepts. */
const MEDIA_TYPES = new Set(["bgm", "sfx", "image", "icon", "logo", "voice"]);

/**
 * How long one resolution may take.
 *
 * A generated image or track is a real round trip to a model, so this is
 * generous — but bounded, because the preview that triggered it is a user
 * watching a spinner.
 */
const RESOLVE_TIMEOUT_MS = 180_000;

/** One declaration found in the document. */
interface Declaration {
    type: string;
    intent: string;
    entity?: string;
    elementId: string;
    tagName: string;
}

export interface MediaResolution {
    /** Declarations found, whether or not they resolved. */
    found: number;
    /** Declarations that produced a file. */
    resolved: number;
    /** Project-relative paths written, for uploading back. */
    files: string[];
    /** One line per declaration that did not resolve, for the report. */
    problems: string[];
    /** True when index.html was rewritten. */
    changed: boolean;
}

/**
 * Write the ceiling the CLI enforces on itself.
 *
 * `.hyperframes/credit-limit.json` is Hyperframes' own mechanism: every paid
 * call refuses what would take the project past it. Writing it from the
 * org's wallet means the spend limit is enforced inside the tool rather than
 * only checked after the fact by us.
 */
export async function writeCreditLimit(projectDir: string, credits: number): Promise<void> {
    if (!Number.isFinite(credits) || credits < 0) return;
    const dir = join(projectDir, ".hyperframes");
    await mkdir(dir, { recursive: true });
    await writeFile(join(dir, "credit-limit.json"), JSON.stringify({ credits: Math.floor(credits) }), "utf8");
}

/** Every media declaration in the document, in order. */
export function readDeclarations(html: string): Declaration[] {
    const els = findElements(html, (el) => typeof el.attrs["data-media"] === "string");
    const out: Declaration[] = [];
    for (const el of els) {
        const type = (el.attrs["data-media"] || "").trim().toLowerCase();
        const intent = (el.attrs["data-media-intent"] || "").trim();
        // A declaration with no type we recognise, or no words to search on,
        // is left exactly as it is: better a film with a gap the author can
        // see than a silent call to a paid API on a guess.
        if (!MEDIA_TYPES.has(type) || intent === "") continue;
        out.push({
            type,
            intent,
            entity: (el.attrs["data-media-entity"] || "").trim() || undefined,
            elementId: el.attrs["id"] || "",
            tagName: el.tagName,
        });
    }
    return out;
}

/**
 * The project-relative path `media-use resolve --json` reported.
 *
 * Its envelope has changed shape between versions and is not documented as
 * stable, so several likely keys are tried before giving up — and giving up
 * is a problem reported, never a guess.
 */
export function pathFromResolveJSON(stdout: string, projectDir: string): string | null {
    let parsed: unknown;
    try {
        parsed = JSON.parse(stdout.trim().split("\n").filter(Boolean).pop() ?? "");
    } catch {
        return null;
    }
    const obj = parsed as Record<string, unknown>;
    const candidates = [obj?.path, obj?.file, obj?.relativePath, (obj?.asset as Record<string, unknown>)?.path];
    for (const c of candidates) {
        if (typeof c === "string" && c.trim() !== "") {
            let p = c.trim();
            if (p.startsWith(projectDir)) p = p.slice(projectDir.length);
            return p.replace(/^[/\\]+/, "");
        }
    }
    return null;
}

/**
 * Resolve every declaration in the project's index.html.
 *
 * Non-fatal throughout. A film whose music could not be found is still a
 * film, and failing the whole job over it would mean a brief that mentioned
 * a song could never be previewed at all.
 */
export async function resolveProjectMedia(
    projectDir: string,
    opts: { heygenApiKey?: string; creditCeiling?: number } = {}
): Promise<MediaResolution> {
    const indexPath = join(projectDir, "index.html");
    const original = await readFile(indexPath, "utf8");
    const declarations = readDeclarations(original);
    const result: MediaResolution = { found: declarations.length, resolved: 0, files: [], problems: [], changed: false };
    if (declarations.length === 0) return result;

    // ── Say what is happening ────────────────────────────────────────────
    // The first run of this resolved nothing and left no trace: problems go
    // back to the caller as warnings on a document, and the worker log — the
    // only place anyone looks when a preview misbehaves — said nothing at
    // all. A paid step that can fail silently is a step nobody can debug.
    console.log(
        `[media] ${declarations.length} declaration(s): ` +
            declarations.map((d) => `${d.type}`).join(", ") +
            ` · credential ${opts.heygenApiKey ? "present" : "ABSENT"}` +
            (typeof opts.creditCeiling === "number" ? ` · ceiling ${opts.creditCeiling}` : "")
    );

    if (typeof opts.creditCeiling === "number") {
        await writeCreditLimit(projectDir, opts.creditCeiling);
    }

    const env = hyperframesEnv();
    if (opts.heygenApiKey) env.HEYGEN_API_KEY = opts.heygenApiKey;

    // Resolved first, then spliced in one pass: an edit shifts every later
    // offset, and re-parsing between each would make the offsets of the
    // declarations we already looked at meaningless.
    const edits: Edit[] = [];
    const resolvedFor = new Map<string, string>();

    for (const d of declarations) {
        const key = `${d.type}\u0000${d.intent}\u0000${d.entity ?? ""}`;
        let rel = resolvedFor.get(key) ?? null;

        if (rel === null) {
            const args = ["media-use", "resolve", "--type", d.type, "--intent", d.intent, "--project", projectDir, "--json"];

            if (d.entity) args.push("--entity", d.entity);
            const startedAt = Date.now();
            const run = await runHyperframes(projectDir, args, { env, timeoutMs: RESOLVE_TIMEOUT_MS });
            if (run.code !== 0) {
                const why = firstLine(run.stderr || run.stdout) || `exit ${run.code}`;
                console.log(`[media] ${d.type} FAILED after ${Date.now() - startedAt}ms: ${why}`);
                result.problems.push(`${d.type} "${trim(d.intent)}": ${why}`);
                continue;
            }
            rel = pathFromResolveJSON(run.stdout, projectDir);
            if (!rel) {
                console.log(`[media] ${d.type} returned no path. stdout: ${trim(run.stdout.replace(/\s+/g, " "))}`);
                result.problems.push(`${d.type} "${trim(d.intent)}": resolved but the reply named no file`);
                continue;
            }
            // The whole envelope, trimmed. Which provider answered and what
            // it cost are the two things nobody can see from outside, and
            // the budget for this work is small enough that guessing from
            // response times is not good enough.
            console.log(
                `[media] ${d.type} resolved to ${rel} in ${Date.now() - startedAt}ms · ` +
                    trim(run.stdout.replace(/\s+/g, " "), 400)
            );
            resolvedFor.set(key, rel);
            result.files.push(rel);
        }
        result.resolved++;
    }

    if (resolvedFor.size === 0) return result;

    // Re-find the elements against the ORIGINAL source, now that every path
    // is known, so all the offsets come from one parse.
    const els = findElements(original, (el) => typeof el.attrs["data-media"] === "string");
    for (const el of els) {
        const type = (el.attrs["data-media"] || "").trim().toLowerCase();
        const intent = (el.attrs["data-media-intent"] || "").trim();
        const entity = (el.attrs["data-media-entity"] || "").trim();
        const rel = resolvedFor.get(`${type}\u0000${intent}\u0000${entity}`);
        if (!rel) continue;

        edits.push(setAttribute(el, "src", rel));
        // The declaration has been honoured; leaving it would have the next
        // run resolve it again and pay again.
        for (const gone of ["data-media", "data-media-intent", "data-media-entity"]) {
            const e = removeAttribute(el, gone, original);
            if (e) edits.push(e);
        }
        edits.push(setAttribute(el, "data-media-resolved", rel));
    }

    if (edits.length === 0) return result;
    await writeFile(indexPath, applyEdits(original, edits), "utf8");
    result.changed = true;
    console.log(`[media] rewrote index.html with ${resolvedFor.size} asset(s)`);
    return result;
}

function firstLine(s: string): string {
    return (s || "").split("\n").map((l) => l.trim()).filter(Boolean)[0] ?? "";
}

function trim(s: string, max = 60): string {
    return s.length > max ? s.slice(0, max - 3) + "…" : s;
}
