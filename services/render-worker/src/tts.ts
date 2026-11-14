/**
 * Speaking a voiceover, by calling HeyGen's text-to-speech directly.
 *
 * ── Why this bypasses media-use, when nothing else does ───────────────────
 * Every other media kind — image, icon, logo, bgm, sfx — resolves through
 * Hyperframes' own `media-use`, and should keep doing so: it searches the
 * free catalogue first, freezes what it finds, keeps a ledger, and enforces
 * the project's credit limit. None of that is worth reimplementing.
 *
 * Voice is the one kind it cannot serve on this account, and the reason is
 * not auth. `media-use/scripts/lib/voice-provider.mjs` asks the HeyGen CLI
 * for `voice list --engine starfish --limit 1`, on the comment that "HeyGen
 * TTS requires a starfish-engine voice". That engine does not exist on this
 * account's catalogue:
 *
 *     /v2/voices → 2602 voices, every one with allowed_engines: []
 *     /v3/voices →   20 voices, all available_engines: ["orca"]
 *
 * so the list comes back empty, `defaultVoiceId()` returns null, and the
 * provider gives up BEFORE it ever calls TTS. The symptom was "no provider
 * could resolve voice", which reads like an entitlement problem and is
 * really a deprecated-endpoint mismatch — HeyGen's own /v2 responses now
 * carry a sunset warning pointing at /v3.
 *
 * The same API key that fails through the CLI works perfectly against /v3
 * directly: verified live on the dev key, two generations, a real 187 KB
 * RIFF/PCM 16-bit mono 44.1 kHz WAV each time, with the wallet unchanged at
 * $13.48 before and after.
 *
 * So: voice only, direct, and the rest of the pipeline untouched. The file
 * this produces is handed back as an ordinary project-relative path, which
 * is exactly what `media-use resolve --json` yields, so captions, the
 * upload-back and the failure-removes-its-own-element degradation all keep
 * working with no change.
 *
 * ── Build nothing new on /v2 ──────────────────────────────────────────────
 * Its sunset date (2026-10-31) has already passed. /v3 is the only surface
 * this file touches.
 */
import { createHash } from "node:crypto";
import { mkdir, stat, writeFile } from "node:fs/promises";
import { join } from "node:path";

/**
 * The TTS model.
 *
 * ⚠️ `heygen-voice-1-turbo-free` is free only until **30 November 2026**.
 * After that date `heygen-voice-1-turbo` is charged per request. This is
 * deliberately one constant with one flag beside it, so switching costs one
 * line and the cost rules below follow automatically.
 *
 * Turbo Free also shares capacity across all HeyGen users, so a 429 can
 * arrive well inside our own workspace rate limit, and HeyGen's docs decline
 * to guarantee its latency or availability. Fine for dev; production should
 * move to the paid model.
 */
export const TTS_MODEL = "heygen-voice-1-turbo-free";

/** Whether {@link TTS_MODEL} costs credits. Flip it with the model. */
export const TTS_MODEL_IS_FREE = true;

/** The date after which the free model stops being free. */
export const TTS_FREE_UNTIL = "2026-11-30";

const TTS_ENDPOINT = "https://api.heygen.com/v3/models/audio/tts";

/**
 * The voice used when the film does not name one.
 *
 * HeyGen's documented public starter voice. It is not in the
 * `/v3/models/audio/voices?type=public` listing — which is itself worth
 * knowing — but it generates, and it was one of the two verified live.
 */
export const DEFAULT_VOICE_ID = "83548de556df48ba8c09c42a57c51d85";

/** How long one synthesis may take, including retries. */
const TTS_TIMEOUT_MS = 60_000;

/** How many times a transient failure is retried. */
const TTS_MAX_ATTEMPTS = 3;

export interface VoiceSynthesis {
    /** Project-relative path to the WAV written. */
    path: string;
    /** Seconds, as HeyGen reported them. */
    duration?: number;
    /** The voice that spoke it. */
    voiceId: string;
}

/**
 * The audio URL out of a TTS reply.
 *
 * Kept separate and tolerant for the same reason `pathFromResolveJSON` is:
 * an envelope that moves between versions should degrade to a reported
 * problem, not to a crash in the middle of a render.
 */
export function audioURLFromTTS(body: unknown): { url: string; duration?: number } | null {
    const obj = body as Record<string, unknown>;
    const data = (obj?.data ?? obj) as Record<string, unknown>;
    const url = data?.audio_url ?? data?.audioUrl ?? data?.url;
    if (typeof url !== "string" || url.trim() === "") return null;
    const duration = typeof data?.duration === "number" ? data.duration : undefined;
    return { url: url.trim(), duration };
}

/**
 * Whether a reply is worth trying again.
 *
 * 429 because Turbo Free's capacity is shared and a limit can be hit that is
 * nothing to do with us; 5xx because a gateway blip should not cost a film
 * its voiceover. Everything else — a bad voice id, a rejected key, text over
 * the limit — will fail identically on a second attempt, so it is reported
 * at once instead.
 */
export function isTransientStatus(status: number): boolean {
    return status === 429 || status === 408 || status >= 500;
}

/**
 * How long to wait before attempt `attempt` (1-based, so the first retry is
 * attempt 2).
 *
 * `Retry-After` wins when the server sends one — it knows when capacity
 * frees up and we do not. Its seconds form is what HeyGen sends; the HTTP
 * date form is accepted too rather than silently treated as zero.
 */
export function retryDelayMs(attempt: number, retryAfter?: string | null): number {
    if (retryAfter) {
        const seconds = Number(retryAfter.trim());
        if (Number.isFinite(seconds) && seconds >= 0) return Math.min(seconds * 1000, 30_000);
        const when = Date.parse(retryAfter);
        if (Number.isFinite(when)) return Math.min(Math.max(0, when - Date.now()), 30_000);
    }
    // 1s, 2s, 4s — bounded, because a preview is a user watching a spinner.
    return Math.min(1000 * 2 ** Math.max(0, attempt - 2), 8000);
}

/**
 * Where a voiceover is written inside the project.
 *
 * Named from a hash of everything that determines the audio, so the same
 * line spoken by the same voice lands on the same file: a re-run overwrites
 * rather than accumulating, and two scenes asking for the same line share
 * one request.
 */
export function voiceFileName(text: string, voiceId: string, model = TTS_MODEL): string {
    const digest = createHash("sha1").update(`${model}\u0000${voiceId}\u0000${text}`).digest("hex").slice(0, 16);
    return `assets/voice/${digest}.wav`;
}

/**
 * Speak `text` and write the result into the project.
 *
 * Throws on failure, with a message fit to put in front of whoever wrote the
 * brief. The caller turns that into a reported problem and drops the
 * element, exactly as it does for a failed `media-use` resolution.
 */
export async function synthesizeVoice(opts: {
    projectDir: string;
    apiKey: string;
    text: string;
    voiceId?: string;
    language?: string;
    /** `fetch` injected for tests. */
    fetchImpl?: typeof fetch;
    /** Sleep injected for tests, so a backoff does not make the suite slow. */
    sleep?: (ms: number) => Promise<void>;
}): Promise<VoiceSynthesis> {
    const text = opts.text.trim();
    if (text === "") throw new Error("nothing to say");
    const voiceId = (opts.voiceId || "").trim() || DEFAULT_VOICE_ID;
    const doFetch = opts.fetchImpl ?? fetch;
    const nap = opts.sleep ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));

    if (!TTS_MODEL_IS_FREE) {
        console.log(`[tts] ${TTS_MODEL} is a CHARGED model — this request costs credits`);
    } else if (Date.now() > Date.parse(`${TTS_FREE_UNTIL}T23:59:59Z`)) {
        console.log(
            `[tts] WARNING: ${TTS_MODEL} was free only until ${TTS_FREE_UNTIL}; ` +
                `this request may now be charged. Review TTS_MODEL in tts.ts.`
        );
    }

    // ── Already spoken? ──────────────────────────────────────────────────
    // The file name is a hash of the model, the voice and the words, so a
    // file sitting there IS this request's answer. Worth checking because
    // the same line is often re-resolved — a film re-authored with the hook
    // unchanged, a project materialised twice — and the model stops being
    // free on 30 November 2026. Free today, so this saves time rather than
    // money; the day the model changes it saves both.
    const existing = join(opts.projectDir, voiceFileName(text, voiceId));
    const already = await statOrNull(existing);
    if (already && already.size > 0) {
        console.log(`[tts] reusing ${voiceFileName(text, voiceId)} (${already.size} bytes), nothing spoken`);
        return { path: voiceFileName(text, voiceId), voiceId };
    }

    let lastWhy = "";
    // Carried from one attempt to the next, because a 429's Retry-After is
    // only useful to the sleep that follows it. Local, not module-level: two
    // films resolving at once must not read each other's backoff.
    let lastRetryAfter: string | null = null;
    for (let attempt = 1; attempt <= TTS_MAX_ATTEMPTS; attempt++) {
        if (attempt > 1) await nap(retryDelayMs(attempt, lastRetryAfter));
        // ── A reset socket is as transient as a 502 ───────────────────────
        // fetch REJECTS on a transport failure rather than resolving with a
        // status, so without this the backoff below — written for exactly
        // this kind of blip — is skipped, and a DNS hiccup inside the task's
        // VPC costs a film its voiceover and its captions. Retried on the
        // same terms as a 5xx; each attempt is a fresh POST, so there is
        // nothing half-done to clean up.
        let res: Response;
        try {
            res = await doFetch(TTS_ENDPOINT, {
                method: "POST",
                headers: { "X-Api-Key": opts.apiKey, "Content-Type": "application/json" },
                body: JSON.stringify({
                    model: TTS_MODEL,
                    voice_id: voiceId,
                    text,
                    language: opts.language || "en",
                }),
                signal: AbortSignal.timeout(TTS_TIMEOUT_MS),
            });
        } catch (err) {
            lastRetryAfter = null;
            lastWhy = err instanceof Error ? err.message : String(err);
            if (attempt < TTS_MAX_ATTEMPTS) {
                console.log(`[tts] attempt ${attempt} never reached HeyGen: ${lastWhy}`);
                continue;
            }
            throw new Error(lastWhy);
        }

        if (!res.ok) {
            lastRetryAfter = res.headers.get("retry-after");
            lastWhy = `HTTP ${res.status} ${trimForLog(await safeText(res))}`;
            if (isTransientStatus(res.status) && attempt < TTS_MAX_ATTEMPTS) {
                console.log(`[tts] attempt ${attempt} transient: ${lastWhy}`);
                continue;
            }
            throw new Error(lastWhy);
        }

        const found = audioURLFromTTS(await res.json().catch(() => null));
        if (!found) throw new Error("the reply named no audio");

        const audio = await doFetch(found.url, { signal: AbortSignal.timeout(TTS_TIMEOUT_MS) });
        if (!audio.ok) throw new Error(`downloading the audio: HTTP ${audio.status}`);
        const bytes = Buffer.from(await audio.arrayBuffer());
        if (bytes.length === 0) throw new Error("the audio came back empty");

        const rel = voiceFileName(text, voiceId);
        const abs = join(opts.projectDir, rel);
        await mkdir(join(opts.projectDir, "assets", "voice"), { recursive: true });
        await writeFile(abs, bytes);
        console.log(
            `[tts] spoke ${text.length} chars as ${voiceId} → ${rel} ` +
                `(${bytes.length} bytes${found.duration ? `, ${found.duration.toFixed(2)}s` : ""})`
        );
        return { path: rel, duration: found.duration, voiceId };
    }
    throw new Error(lastWhy || "no attempt succeeded");
}

async function statOrNull(path: string) {
    try {
        return await stat(path);
    } catch {
        return null;
    }
}

async function safeText(res: Response): Promise<string> {
    try {
        return await res.text();
    } catch {
        return "";
    }
}

function trimForLog(s: string): string {
    const one = (s || "").replace(/\s+/g, " ").trim();
    return one.length > 200 ? `${one.slice(0, 197)}…` : one;
}
