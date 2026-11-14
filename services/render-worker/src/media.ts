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
    // Element id → the voiceover file resolved for it, so a caption track
    // can find the audio it is meant to follow.
    const voiceFor = new Map<string, string>();
    // Declarations that could not be resolved, keyed the same way, so their
    // elements can be taken out rather than left pointing at nothing.
    const failed = new Set<string>();

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
                failed.add(key);
                continue;
            }
            rel = pathFromResolveJSON(run.stdout, projectDir);
            if (!rel) {
                console.log(`[media] ${d.type} returned no path. stdout: ${trim(run.stdout.replace(/\s+/g, " "))}`);
                result.problems.push(`${d.type} "${trim(d.intent)}": resolved but the reply named no file`);
                failed.add(key);
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
        if (d.type === "voice" && d.elementId) voiceFor.set(d.elementId, rel);
        result.resolved++;
    }

    if (resolvedFor.size === 0 && failed.size === 0) return result;

    // Re-find the elements against the ORIGINAL source, now that every path
    // is known, so all the offsets come from one parse.
    const els = findElements(original, (el) => typeof el.attrs["data-media"] === "string");
    for (const el of els) {
        const type = (el.attrs["data-media"] || "").trim().toLowerCase();
        const intent = (el.attrs["data-media-intent"] || "").trim();
        const entity = (el.attrs["data-media-entity"] || "").trim();
        const key = `${type}\u0000${intent}\u0000${entity}`;
        const rel = resolvedFor.get(key);
        if (!rel) {
            // ── A declaration nobody could serve takes its element with it ──
            // Left in place, an <audio data-media="voice"> that did not
            // resolve is an audio element with no src, and the gate refuses
            // the whole film over it: one unavailable provider and nothing
            // previews. Found on dev when voice turned out to need the
            // OAuth path rather than an API key, and a reel that was
            // otherwise complete — photograph, music, captions markup —
            // could not be seen at all.
            //
            // So the element goes. A film missing its soundtrack is a film;
            // a film that refuses to render is not. The problem is already
            // on the warnings list, so nothing is hidden by this.
            if (failed.has(key)) edits.push({ start: el.start, end: el.end, text: "" });
            continue;
        }

        edits.push(setAttribute(el, "src", rel));
        // The declaration has been honoured; leaving it would have the next
        // run resolve it again and pay again.
        for (const gone of ["data-media", "data-media-intent", "data-media-entity"]) {
            const e = removeAttribute(el, gone, original);
            if (e) edits.push(e);
        }
        edits.push(setAttribute(el, "data-media-resolved", rel));
    }

    // ── Captions ─────────────────────────────────────────────────────────
    // A container carrying data-captions="<audio id>" asks for a word-synced
    // track against that voiceover. Done here, in the same pass, because the
    // audio only exists now — and written as timed spans the runtime already
    // knows how to show, so there is no new dependency and no custom script.
    for (const el of findElements(original, (e) => typeof e.attrs["data-captions"] === "string")) {
        const forId = (el.attrs["data-captions"] || "").trim();
        const voicePath = voiceFor.get(forId);
        if (!voicePath) {
            // The attribute goes even though the captions cannot be made:
            // left in place it is a promise to a later run that the audio
            // exists, and the container is already empty.
            const drop = removeAttribute(el, "data-captions", original);
            if (drop) edits.push(drop);
            result.problems.push(`captions for "${forId}": that voiceover did not resolve`);
            continue;
        }
        const started = Date.now();
        const run = await runHyperframes(
            projectDir,
            ["transcribe", voicePath, "--dir", projectDir, "--json", "--optional"],
            { env, timeoutMs: RESOLVE_TIMEOUT_MS }
        );
        const words = run.code === 0 ? wordsFromTranscribeJSON(run.stdout) : [];
        if (words.length === 0) {
            const why = firstLine(run.stderr || run.stdout) || `no words (exit ${run.code})`;
            console.log(`[media] captions for ${forId} produced nothing after ${Date.now() - started}ms: ${why}`);
            result.problems.push(`captions for "${forId}": ${why}`);
            continue;
        }
        const offset = Number(el.attrs["data-start"] ?? "0") || 0;
        // The container's own children are replaced: whatever placeholder
        // the model put there was a stand-in for exactly this.
        edits.push({ start: el.tagEnd, end: el.end - closingTagLength(el), text: captionSpans(words, offset) });
        const e = removeAttribute(el, "data-captions", original);
        if (e) edits.push(e);
        console.log(`[media] captions for ${forId}: ${words.length} words in ${Date.now() - started}ms`);
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

/** One word of a transcript, as `hyperframes transcribe --json` reports it. */
export interface TranscriptWord {
    text: string;
    start: number;
    end: number;
}

/**
 * The words out of a transcribe envelope.
 *
 * Tolerant of where the array sits, for the same reason pathFromResolveJSON
 * is: the shape is not documented as stable, and a caption track is not
 * worth failing a film over.
 */
export function wordsFromTranscribeJSON(stdout: string): TranscriptWord[] {
    let parsed: unknown;
    try {
        parsed = JSON.parse(stdout.trim().split("\n").filter(Boolean).pop() ?? "");
    } catch {
        return [];
    }
    const obj = parsed as Record<string, unknown>;
    const candidates = [parsed, obj?.words, obj?.transcript, (obj?.result as Record<string, unknown>)?.words];
    for (const c of candidates) {
        if (!Array.isArray(c)) continue;
        const words = c
            .map((w) => w as Record<string, unknown>)
            .filter((w) => typeof w?.text === "string" && typeof w?.start === "number" && typeof w?.end === "number")
            .map((w) => ({ text: String(w.text), start: Number(w.start), end: Number(w.end) }));
        if (words.length > 0) return words;
    }
    return [];
}

/**
 * The markup for a word-synced caption track.
 *
 * ── Why our own spans and not a registry caption component ────────────────
 * Hyperframes ships fifteen caption components, and they are good. They are
 * also styled their way, installed with `hyperframes add`, and configured
 * through an API we would have to pin a version against. Our films already
 * style text from the brand kit, and the runtime ALREADY times any element
 * carrying `.clip` with data-start and data-duration — that is how every
 * scene in these films works.
 *
 * So a caption is one span per word, timed by the same mechanism as
 * everything else in the document. No new dependency, no custom script, and
 * the words inherit the brand's font and colour like the rest of the film.
 *
 * `offsetS` shifts the transcript onto the film's clock: a voiceover that
 * starts four seconds in has a transcript that starts at zero.
 */
export function captionSpans(words: TranscriptWord[], offsetS = 0): string {
    return words
        .map((w) => {
            const start = Math.max(0, w.start + offsetS);
            // A word the transcriber gave no width to would be timed to
            // vanish on the frame it appears, which reads as a flicker.
            const duration = Math.max(0.08, w.end - w.start);
            return (
                `<span class="clip" data-start="${start.toFixed(2)}" ` +
                `data-duration="${duration.toFixed(2)}">${escapeText(w.text)}</span>`
            );
        })
        .join(" ");
}

function escapeText(s: string): string {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}


/**
 * How many characters the element's closing tag takes.
 *
 * `end` covers the whole element, so replacing its children means stopping
 * short of `</tag>`. A void element has no closing tag and no children to
 * replace, which is why this returns zero for one.
 */
function closingTagLength(el: { tagName: string; start: number; end: number; tagEnd: number }): number {
    const close = `</${el.tagName}>`.length;
    // A self-closed or void element: the start tag IS the element.
    return el.end - el.tagEnd >= close ? close : 0;
}

/**
 * Keep the player's page in step with the film.
 *
 * ── The bug this exists for ───────────────────────────────────────────────
 * `preview.html` is what the PLAYER loads, and it is written once, at
 * authoring time, as index.html plus two script tags the renderer does not
 * need (the Hyperframes runtime and GSAP). Media resolution happens later,
 * in this worker, and rewrites index.html.
 *
 * So the film the renderer produced had its photograph and its soundtrack,
 * and the film the user watched in the app did not — same design, two
 * documents, one of them a snapshot from before the media existed. Reported
 * from the app as "it doesn't load any media", with the preview filmstrip
 * beside it showing the photograph perfectly, because the stills are
 * captured from the worker's copy.
 *
 * Rather than duplicate the Go that builds that page, the script tags are
 * lifted off the existing preview.html and re-applied to the new index.html:
 * whatever the backend put there is preserved, including a version bump, and
 * there is only ever one definition of what those tags are.
 */
export async function syncPlayerPage(projectDir: string): Promise<boolean> {
    const previewPath = join(projectDir, "preview.html");
    let previous: string;
    try {
        previous = await readFile(previewPath, "utf8");
    } catch {
        // A project authored before the player existed has no page to sync.
        return false;
    }
    const indexHTML = await readFile(join(projectDir, "index.html"), "utf8");

    // The tags preview.html adds: the shared player assets, which live under
    // a versioned prefix and are the only scripts index.html does not carry.
    const tags = findElements(previous, (el) => {
        const src = el.attrs["src"] ?? "";
        return el.tagName === "script" && src.includes("/design-player/");
    }).map((el) => previous.slice(el.start, el.end));

    if (tags.length === 0) return false;
    await writeFile(previewPath, injectIntoHead(indexHTML, tags.join("\n    ")), "utf8");
    return true;
}

/**
 * Put a snippet immediately after <head>, falling back to <body> and then
 * the top of the document.
 *
 * The same rule as hfproject.InjectIntoHead in the backend, because this
 * rebuilds the page that function first wrote. Early is always safe for
 * these: they define what the rest of the document uses.
 */
function injectIntoHead(html: string, snippet: string): string {
    const head = /<head[^>]*>/i.exec(html);
    if (head) return html.slice(0, head.index + head[0].length) + "\n    " + snippet + html.slice(head.index + head[0].length);
    const body = /<body[^>]*>/i.exec(html);
    if (body) return html.slice(0, body.index + body[0].length) + "\n    " + snippet + html.slice(body.index + body[0].length);
    return snippet + "\n" + html;
}
