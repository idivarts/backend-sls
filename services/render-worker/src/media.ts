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
 *
 * ── The one exception: voice ──────────────────────────────────────────────
 * `media-use` cannot resolve a voiceover on this account, and the reason is
 * a deprecated endpoint rather than a missing entitlement: its provider asks
 * for a "starfish" engine voice, which neither /v2 nor /v3 lists any more.
 * Voice therefore goes straight to HeyGen's /v3 TTS — see tts.ts, which
 * explains it in full — and hands back an ordinary project-relative path, so
 * nothing downstream knows the difference. Every other kind still goes
 * through the CLI, and should.
 */
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { isAbsolute, join } from "node:path";

import { hyperframesEnv, runHyperframes } from "./hyperframes";
import {
    applyEdits,
    findElements,
    setAttribute,
    type Edit,
} from "./html-edit";
import { TTS_MODEL_IS_FREE, synthesizeVoice } from "./tts";

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
        // ── Already served ───────────────────────────────────────────────
        // A resolved declaration keeps its intent now, because the script
        // panel edits a voiceover BY its intent — those are the words it
        // says, and a film that has forgotten them cannot show them in a
        // script or let anyone rewrite them. So what marks a declaration
        // done is data-media-resolved agreeing with the src, and a
        // voiceover whose words were changed has neither: the script
        // editor clears them, which is exactly how it asks for the line to
        // be spoken again.
        const resolved = (el.attrs["data-media-resolved"] || "").trim();
        if (resolved !== "" && resolved === (el.attrs["src"] || "").trim()) continue;
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

    // ── Nothing to fetch is not nothing to do ────────────────────────────
    // This used to return here, and that quietly made every later fix to
    // the captions unreachable: a film whose media is all resolved has no
    // declarations, so the caption pass below never ran — and a caption
    // track left stale by an earlier run could never catch up, however
    // many times it was rendered. Only a film with neither declarations
    // NOR caption tracks has nothing to do here.
    if (declarations.length === 0 && captionTracks(original).length === 0) return result;

    // ── Say what is happening ────────────────────────────────────────────
    // The first run of this resolved nothing and left no trace: problems go
    // back to the caller as warnings on a document, and the worker log — the
    // only place anyone looks when a preview misbehaves — said nothing at
    // all. A paid step that can fail silently is a step nobody can debug.
    console.log(
        `[media] ${declarations.length} declaration(s)` +
            (declarations.length > 0 ? `: ${declarations.map((d) => d.type).join(", ")}` : "") +
            `, ${captionTracks(original).length} caption track(s)` +
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
    // How long each voiceover turned out to be, so a line that does not fit
    // the slot it was written for can be said out loud rather than silently
    // cut in the finished film.
    const spokenSeconds = new Map<string, number>();
    // Declarations that could not be resolved, keyed the same way, so their
    // elements can be taken out rather than left pointing at nothing.
    const failed = new Set<string>();
    // The ids of voiceovers that failed, so a caption track following one
    // can say why it has nothing — without saying it about every track
    // whose audio simply has not changed.
    const failedVoiceIDs = new Set<string>();

    for (const d of declarations) {
        const key = `${d.type}\u0000${d.intent}\u0000${d.entity ?? ""}`;
        let rel = resolvedFor.get(key) ?? null;

        if (rel === null && d.type === "voice") {
            // ── Voice is the one kind media-use cannot serve ──────────────
            // Not an auth problem: its provider asks for a "starfish" engine
            // voice, an engine this account's catalogue does not have on
            // either /v2 or /v3, so it gives up before it ever calls TTS.
            // The same key works directly against /v3. See tts.ts.
            const startedAt = Date.now();
            const why = voiceRefusal(opts);
            if (why) {
                console.log(`[media] voice REFUSED: ${why}`);
                result.problems.push(`voice "${trim(d.intent)}": ${why}`);
                failed.add(key);
                if (d.elementId) failedVoiceIDs.add(d.elementId);
                continue;
            }
            try {
                const spoken = await synthesizeVoice({
                    projectDir,
                    apiKey: opts.heygenApiKey as string,
                    text: d.intent,
                    // A film may name the voice it wants — but only as a real
                    // HeyGen id. The authoring contract documents
                    // data-media-entity as the BRAND for a logo, so a model
                    // may well write data-media-entity="narrator" on a voice
                    // out of symmetry, and passing that through as a voice_id
                    // would turn a working voiceover into a 400. Anything
                    // that is not an id shape is ignored and the default
                    // voice speaks.
                    voiceId: voiceIdFromEntity(d.entity),
                });
                rel = spoken.path;
                resolvedFor.set(key, rel);
                result.files.push(rel);
                if (spoken.duration) spokenSeconds.set(key, spoken.duration);
                console.log(`[media] voice resolved to ${rel} in ${Date.now() - startedAt}ms`);
            } catch (err) {
                const msg = err instanceof Error ? err.message : String(err);
                console.log(`[media] voice FAILED after ${Date.now() - startedAt}ms: ${msg}`);
                result.problems.push(`voice "${trim(d.intent)}": ${msg}`);
                failed.add(key);
                if (d.elementId) failedVoiceIDs.add(d.elementId);
                continue;
            }
        }

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
        if (d.type === "voice" && d.elementId) {
            voiceFor.set(d.elementId, rel);
            const audioEl = findElements(original, (e) => e.attrs["id"] === d.elementId)[0];
            if (audioEl) {
                // ── Does it fit? ──────────────────────────────────────────
                // The renderer plays an audio element for its data-duration
                // and not a moment longer, so a line written longer than
                // its scene is cut off mid-sentence — and the next scene's
                // voiceover starts talking over the end of it. Seen on the
                // first real reel: nine words in a three-second scene came
                // out as "Ditch the sweet milk, experience pure". Nothing
                // here can fix it (lengthening the clip would push every
                // later scene, shortening the words is the author's call),
                // but it must not be silent. The authoring contract now
                // gives the model a word budget; this catches the times it
                // does not keep to it.
                const slot = durationSeconds(audioEl);
                const spokenFor = spokenSeconds.get(key);
                if (spokenFor !== undefined && slot !== undefined && slot > 0 && spokenFor > slot + 0.25) {
                    const why =
                        `${spokenFor.toFixed(1)}s of speech in a ${slot}s slot — ` +
                        `the film will cut it off, and the next line will talk over it`;
                    console.log(`[media] voice ${d.elementId} does not fit: ${why}`);
                    result.problems.push(`voice "${trim(d.intent)}": ${why}`);
                }
            }
        }
        result.resolved++;
    }

    // Falls through when nothing resolved: the caption pass below may
    // still have work, and returning here is what kept a stale track
    // stale.
    if (resolvedFor.size === 0 && failed.size === 0) {
        const rebuilt = await rebuildCaptions(projectDir, original, env, voiceFor, failedVoiceIDs, result);
        if (rebuilt.length === 0) return result;
        await writeFile(indexPath, applyEdits(original, rebuilt), "utf8");
        result.changed = true;
        console.log(`[media] rewrote index.html with ${rebuilt.length} caption edit(s)`);
        return result;
    }

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
        // ── What the film remembers about its own media ──────────────────
        // The declaration used to be STRIPPED here, so a second run could
        // not resolve it again and pay again. It is kept now, and
        // data-media-resolved is what stops the second run instead (see
        // readDeclarations) — because the words a voiceover says are the
        // only record of that line, and the script panel edits them. A
        // film that threw them away could show a script of its on-screen
        // text and nothing of what it SAYS.
        edits.push(setAttribute(el, "data-media-resolved", rel));
    }

    edits.push(...(await rebuildCaptions(projectDir, original, env, voiceFor, failedVoiceIDs, result)));

    if (edits.length === 0) return result;
    await writeFile(indexPath, applyEdits(original, edits), "utf8");
    result.changed = true;
    console.log(`[media] rewrote index.html with ${resolvedFor.size} asset(s)`);
    return result;
}

/**
 * A HeyGen voice id, if that is what the entity is.
 *
 * Their ids are 32 lowercase hex characters. Anything else — a name, a tone,
 * an empty string — is not an id and is dropped, so the default voice speaks
 * rather than the request failing.
 */
export function voiceIdFromEntity(entity?: string): string | undefined {
    const e = (entity || "").trim().toLowerCase();
    return /^[0-9a-f]{32}$/.test(e) ? e : undefined;
}

/**
 * Why this run may not speak, or "" when it may.
 *
 * ── The cost rule, written once ───────────────────────────────────────────
 * The project's credit ceiling is enforced inside `media-use` by Hyperframes
 * itself. Bypassing it for voice means bypassing that enforcement too, so
 * the same rule is applied here instead — and applied to the only thing that
 * matters: whether the model we are about to call costs anything.
 *
 * While the model is free, an org with an exhausted wallet still gets its
 * voiceover, because refusing it would be punishing a user over a request
 * that bills nobody. The moment TTS_MODEL_IS_FREE goes false, a zero ceiling
 * refuses — which is what the entitlements rule asks for: pre-empt, don't
 * punish.
 */
function voiceRefusal(opts: { heygenApiKey?: string; creditCeiling?: number }): string {
    if (!opts.heygenApiKey) return "no HeyGen credential is available to this render";
    if (!TTS_MODEL_IS_FREE && (opts.creditCeiling ?? 0) <= 0) {
        return "this org has no media credit left, and the voice model is a charged one";
    }
    return "";
}

/**
 * An element's data-start in seconds, or undefined when it has none.
 *
 * Undefined rather than zero, so a caller can tell "starts at the beginning"
 * from "does not say", and fall back to something that does.
 */
export function startSeconds(el?: { attrs: Record<string, string | undefined> }): number | undefined {
    const raw = el?.attrs["data-start"];
    if (raw === undefined) return undefined;
    const n = Number(raw);
    return Number.isFinite(n) ? n : undefined;
}

/** An element's data-duration in seconds, or undefined when it has none. */
export function durationSeconds(el?: { attrs: Record<string, string | undefined> }): number | undefined {
    const raw = el?.attrs["data-duration"];
    if (raw === undefined) return undefined;
    const n = Number(raw);
    return Number.isFinite(n) ? n : undefined;
}

/**
 * Every caption track in the document.
 *
 * Two ways to be one: it says so, or it carries the class the authoring
 * contract gives them. The second matters because a track written before
 * this code existed LOST its declaration — the first version of captions
 * removed data-captions once the words were in — and a track that cannot
 * be recognised can never be rebuilt.
 */
function captionTracks(html: string) {
    return findElements(
        html,
        (e) =>
            typeof e.attrs["data-captions"] === "string" ||
            (e.attrs["class"] || "").split(/\s+/).includes("caption-track")
    );
}

/**
 * The voiceover a track with no declaration follows: the one that starts
 * at the same moment.
 *
 * ── Why this inference is safe, where most would not be ──────────────────
 * A caption track is written to sit on its voiceover — same data-start,
 * same data-duration — because that is the only way the words can line up
 * with the speech. So an exact match on the start time is not a guess
 * about intent; it is reading the timing the author already wrote.
 *
 * And it only answers when exactly ONE voiceover starts there. Two, and
 * there is a real question about which, so it declines rather than
 * picking — a caption track rebuilt from the wrong voice would be worse
 * than a stale one, because a stale one at least matches something that
 * was once true.
 */
function followsByTime(html: string, track: { attrs: Record<string, string | undefined> }): string {
    const at = (track.attrs["data-start"] || "").trim();
    if (at === "") return "";
    const sameStart = findElements(
        html,
        (e) =>
            e.tagName === "audio" &&
            (e.attrs["data-media"] || "").trim().toLowerCase() === "voice" &&
            (e.attrs["data-start"] || "").trim() === at &&
            (e.attrs["id"] || "") !== ""
    );
    return sameStart.length === 1 ? sameStart[0].attrs["id"] || "" : "";
}

/**
 * The audio a voiceover element currently points at, or "".
 *
 * Only a RESOLVED one answers: a declaration still waiting to be spoken
 * has an empty src, and building captions from nothing would write an
 * empty track over the words already on screen.
 */
function currentVoiceSrc(html: string, elementID: string): string {
    const found = findElements(html, (e) => (e.attrs["id"] || "") === elementID);
    for (const el of found) {
        const src = (el.attrs["src"] || "").trim();
        if (src !== "") return src;
    }
    return "";
}

/**
 * Build or rebuild every caption track in the document.
 *
 * Extracted so it can run on BOTH paths: the one where media was just
 * resolved, and the one where nothing needed resolving at all. That
 * second path is where a stale track lives — a film whose voiceover was
 * rewritten and re-spoken by an earlier run has no declarations left, so
 * for a long time it never reached this code and its captions stayed on
 * the old words for ever.
 */
async function rebuildCaptions(
    projectDir: string,
    original: string,
    env: NodeJS.ProcessEnv,
    voiceFor: Map<string, string>,
    failedVoiceIDs: Set<string>,
    result: MediaResolution
): Promise<Edit[]> {
    const out: Edit[] = [];
    // ── Captions ─────────────────────────────────────────────────────────
    // A container carrying data-captions="<audio id>" asks for a word-synced
    // track against that voiceover. Done here, in the same pass, because the
    // audio only exists now — and written as timed spans the runtime already
    // knows how to show, so there is no new dependency and no custom script.
    for (const el of captionTracks(original)) {
        const forId = (el.attrs["data-captions"] || "").trim() || followsByTime(original, el);
        if (forId === "") continue;
        // ── Which audio this track should be built from ──────────────────
        // The voiceover resolved in THIS run if there was one, and
        // otherwise whatever the element already points at. The second
        // half matters: a track can be stale while its audio is
        // perfectly resolved and untouched — that is exactly the state a
        // film is left in when the words were rewritten by one run and
        // the captions were never rebuilt by it. Asking only about this
        // run's resolutions meant such a track could never catch up.
        const voicePath = voiceFor.get(forId) ?? currentVoiceSrc(original, forId);
        if (!voicePath) {
            // Nothing to do: either that voiceover did not resolve this
            // run, or it was already resolved and has not changed. The
            // attribute STAYS either way — it is the only record of which
            // audio this track follows, and a track that has forgotten
            // cannot be rebuilt when the line is rewritten.
            if (failedVoiceIDs.has(forId)) {
                result.problems.push(`captions for "${forId}": that voiceover did not resolve`);
            }
            continue;
        }
        // ── Already built for THIS audio ─────────────────────────────────
        // A caption track is rebuilt when the words change and not
        // otherwise. What marks it done is the audio it was built from:
        // rewriting a voiceover gives it a new file, so this disagrees
        // and the words are transcribed again.
        //
        // Found on dev: the first version of this removed data-captions
        // once the words were in, so a rewritten line came back spoken
        // correctly with the OLD caption still on screen — the film
        // saying one thing and showing another.
        if ((el.attrs["data-captions-for"] || "").trim() === voicePath) continue;
        const started = Date.now();
        const run = await runHyperframes(
            projectDir,
            [
                "transcribe",
                voicePath,
                "--dir",
                projectDir,
                "--json",
                // Skip rather than fail when whisper is unavailable: a film
                // without captions is still a film.
                "--optional",
                // ── Three flags that are all about not surprising anyone ──
                // whisper, not auto: auto prefers Parakeet when installed,
                // and which engine ran decides how the words are shaped.
                "--engine",
                "whisper",
                // base.en, not the CLI's small.en default — that is the model
                // baked into the image, and asking for another would make a
                // render download 490 MB mid-job.
                "--model",
                "base.en",
                // And never build a runtime in here. Given no whisper-cli the
                // CLI clones and COMPILES whisper.cpp into its cache, which
                // inside a render reads as a hang. With this, a missing
                // runtime is a skipped caption track and a line in the log.
                "--no-runtime-install",
            ],
            { env, timeoutMs: RESOLVE_TIMEOUT_MS }
        );
        // ── Where the words actually are ──────────────────────────────────
        // Three surprises, all found by running the real CLI in the image:
        //
        //  1. stdout carries ONLY the summary — {"ok":true,…,"wordCount":6}.
        //     The {"type":"words",…} line goes to STDERR, with the progress
        //     lines. Reading stdout alone finds a transcript of nothing.
        //  2. The summary names a transcriptPath, and the file it names is
        //     the better source: its timings are tightened per word, where
        //     the streamed line's last word ends at 2.84s on a 2.69s clip —
        //     captions that outlast their own audio.
        //  3. So the file is preferred and the streams are the fallback,
        //     rather than the other way round.
        const words = run.code === 0 ? await transcribedWords(projectDir, run.stdout, run.stderr) : [];
        if (words.length === 0) {
            const why = firstLine(run.stderr || run.stdout) || `no words (exit ${run.code})`;
            console.log(`[media] captions for ${forId} produced nothing after ${Date.now() - started}ms: ${why}`);
            result.problems.push(`captions for "${forId}": ${why}`);
            continue;
        }
        // ── Whose clock the transcript is on ─────────────────────────────
        // The transcript starts at zero because the WAV does. What it has to
        // be shifted onto is the moment the AUDIO starts in the film, which
        // is the audio element's data-start — not the caption container's.
        // Those are usually the same and need not be: a caption layer
        // spanning the whole reel (data-start="0" data-duration="12") around
        // a voiceover that begins at four seconds is a perfectly natural way
        // to author it, and reading the container there would run every word
        // four seconds ahead of the voice.
        // Looked up fresh rather than carried in: this runs on a path
        // where nothing was resolved and there is no map of elements from
        // a loop that did not happen.
        const voiceEl = findElements(original, (e) => (e.attrs["id"] || "") === forId)[0];
        const offset = startSeconds(voiceEl) ?? startSeconds(el) ?? 0;
        // The container's own children are replaced: whatever placeholder
        // the model put there was a stand-in for exactly this.
        out.push({ start: el.tagEnd, end: el.end - closingTagLength(el), text: captionSpans(words, offset) });
        out.push(setAttribute(el, "data-captions-for", voicePath));
        // Re-declared where it had lost the link, so the next run does not
        // have to work it out again.
        if ((el.attrs["data-captions"] || "").trim() === "") {
            out.push(setAttribute(el, "data-captions", forId));
        }
        console.log(`[media] captions for ${forId}: ${words.length} words in ${Date.now() - started}ms`);
    }

    return out;
}

function firstLine(s: string): string {
    return (s || "").split("\n").map((l) => l.trim()).filter(Boolean)[0] ?? "";
}

function trim(s: string, max = 60): string {
    return s.length > max ? s.slice(0, max - 3) + "…" : s;
}

/**
 * The transcript file the CLI says it wrote, if it said.
 *
 * Only the summary line carries it, and the summary is the one line that
 * reliably arrives on stdout.
 */
export function transcriptPathFromJSON(stdout: string): string | null {
    for (const line of stdout.split("\n")) {
        const trimmed = line.trim();
        if (trimmed === "") continue;
        try {
            const obj = JSON.parse(trimmed) as Record<string, unknown>;
            const p = obj?.transcriptPath;
            if (typeof p === "string" && p.trim() !== "") return p.trim();
        } catch {
            continue;
        }
    }
    return null;
}

/** The transcript, from the file it was written to or from the streams. */
async function transcribedWords(projectDir: string, stdout: string, stderr: string): Promise<TranscriptWord[]> {
    const path = transcriptPathFromJSON(stdout);
    if (path) {
        const abs = isAbsolute(path) ? path : join(projectDir, path);
        try {
            const words = wordsFromTranscribeJSON(await readFile(abs, "utf8"));
            if (words.length > 0) return words;
        } catch {
            // Fall through to the streams: a transcript we cannot read is a
            // reason to look elsewhere, not a reason to drop the captions.
        }
    }
    return wordsFromTranscribeJSON(`${stdout}\n${stderr}`);
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
    // ── Every line, not the last one ──────────────────────────────────────
    // `transcribe --json` streams NDJSON: progress lines, then one
    // {"type":"words","words":[…]} line, then a summary. The summary is last
    // and carries no words — only `wordCount` and a transcriptPath — so
    // reading the final line (which is right for `resolve --json`) finds
    // nothing. Caught by running the real CLI against a real voiceover in
    // the image; the unit test that followed uses that exact output.
    // The whole text first, then line by line. Both shapes are real: the
    // CLI STREAMS NDJSON, one object per line, while the transcript FILE it
    // writes is a single pretty-printed array — so a line-by-line reader
    // alone silently finds nothing in the file, which is the better source.
    for (const candidateText of [stdout, ...stdout.split("\n")]) {
        const trimmed = candidateText.trim();
        if (trimmed === "") continue;
        let parsed: unknown;
        try {
            parsed = JSON.parse(trimmed);
        } catch {
            continue;
        }
        const obj = parsed as Record<string, unknown>;
        const candidates = [parsed, obj?.words, obj?.transcript, (obj?.result as Record<string, unknown>)?.words];
        for (const c of candidates) {
            if (!Array.isArray(c)) continue;
            const words = c
                .map((w) => w as Record<string, unknown>)
                .filter(
                    (w) => typeof w?.text === "string" && typeof w?.start === "number" && typeof w?.end === "number"
                )
                .map((w) => ({ text: String(w.text), start: Number(w.start), end: Number(w.end) }));
            if (words.length > 0) return words;
        }
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
