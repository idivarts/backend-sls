import assert from "node:assert/strict";
import { mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";

import {
    DEFAULT_VOICE_ID,
    audioURLFromTTS,
    isTransientStatus,
    retryDelayMs,
    synthesizeVoice,
    voiceFileName,
} from "./tts";

/** A reply shaped like the one the dev key actually returned. */
function ttsOK(url = "https://resource2.heygen.ai/text_to_speech/x.wav", duration = 2.1256916099773244) {
    return new Response(JSON.stringify({ data: { audio_url: url, duration } }), {
        status: 200,
        headers: { "content-type": "application/json" },
    });
}

function wav(bytes = 2048): Response {
    const buf = Buffer.alloc(bytes);
    buf.write("RIFF", 0, "ascii");
    return new Response(buf, { status: 200 });
}

describe("audioURLFromTTS", () => {
    it("reads the envelope HeyGen actually sends", () => {
        assert.deepEqual(audioURLFromTTS({ data: { audio_url: "https://x/y.wav", duration: 2.5 } }), {
            url: "https://x/y.wav",
            duration: 2.5,
        });
    });

    it("tolerates a flatter envelope, and a missing duration", () => {
        assert.deepEqual(audioURLFromTTS({ audio_url: "https://x/y.wav" }), {
            url: "https://x/y.wav",
            duration: undefined,
        });
    });

    it("is null rather than throwing when there is no audio", () => {
        assert.equal(audioURLFromTTS({ data: { duration: 1 } }), null);
        assert.equal(audioURLFromTTS(null), null);
        assert.equal(audioURLFromTTS({ data: { audio_url: "  " } }), null);
    });
});

describe("isTransientStatus", () => {
    it("retries a shared-capacity 429 and a gateway blip", () => {
        assert.equal(isTransientStatus(429), true);
        assert.equal(isTransientStatus(500), true);
        assert.equal(isTransientStatus(503), true);
    });

    it("does not retry what will fail identically next time", () => {
        for (const s of [400, 401, 403, 404, 422]) assert.equal(isTransientStatus(s), false);
    });
});

describe("retryDelayMs", () => {
    it("obeys Retry-After in seconds", () => {
        assert.equal(retryDelayMs(2, "3"), 3000);
    });

    it("obeys Retry-After as an HTTP date, and never waits longer than 30s", () => {
        const soon = new Date(Date.now() + 2000).toUTCString();
        const d = retryDelayMs(2, soon);
        assert.ok(d > 500 && d <= 2000, `expected about 2s, got ${d}`);
        assert.equal(retryDelayMs(2, "99999"), 30_000);
    });

    it("backs off without a header, bounded", () => {
        assert.equal(retryDelayMs(2, null), 1000);
        assert.equal(retryDelayMs(3, null), 2000);
        assert.equal(retryDelayMs(9, null), 8000);
    });

    it("ignores a header it cannot read", () => {
        assert.equal(retryDelayMs(2, "whenever"), 1000);
    });
});

describe("voiceFileName", () => {
    it("is stable for the same line and voice, so a re-run reuses the file", () => {
        const a = voiceFileName("Hit the 2 PM wall?", DEFAULT_VOICE_ID);
        const b = voiceFileName("Hit the 2 PM wall?", DEFAULT_VOICE_ID);
        assert.equal(a, b);
        assert.match(a, /^assets\/voice\/[0-9a-f]{16}\.wav$/);
    });

    it("differs when the words or the voice differ", () => {
        assert.notEqual(
            voiceFileName("one", DEFAULT_VOICE_ID),
            voiceFileName("two", DEFAULT_VOICE_ID)
        );
        assert.notEqual(voiceFileName("one", DEFAULT_VOICE_ID), voiceFileName("one", "deadbeef"));
    });
});

describe("synthesizeVoice", () => {
    it("posts to /v3, downloads the audio and writes it into the project", async () => {
        const dir = await mkdtemp(join(tmpdir(), "tts-"));
        const calls: Array<{ url: string; body?: unknown }> = [];
        const fetchImpl = (async (url: string | URL | Request, init?: RequestInit) => {
            const u = String(url);
            calls.push({ url: u, body: init?.body ? JSON.parse(String(init.body)) : undefined });
            return u.startsWith("https://api.heygen.com") ? ttsOK() : wav();
        }) as unknown as typeof fetch;

        const out = await synthesizeVoice({
            projectDir: dir,
            apiKey: "k",
            text: "  Hit the 2 PM wall?  ",
            fetchImpl,
        });

        assert.equal(out.path, voiceFileName("Hit the 2 PM wall?", DEFAULT_VOICE_ID));
        assert.equal(out.voiceId, DEFAULT_VOICE_ID);
        assert.ok(Math.abs((out.duration ?? 0) - 2.1256916099773244) < 1e-9);

        assert.equal(calls[0].url, "https://api.heygen.com/v3/models/audio/tts");
        const body = calls[0].body as Record<string, unknown>;
        // The text is sent trimmed, and the language is explicit: the file
        // name is derived from the trimmed text, so sending the untrimmed
        // form would mean the audio and its name disagree.
        assert.equal(body.text, "Hit the 2 PM wall?");
        assert.equal(body.voice_id, DEFAULT_VOICE_ID);
        assert.equal(body.language, "en");

        const written = await readFile(join(dir, out.path));
        assert.equal(written.subarray(0, 4).toString("ascii"), "RIFF");
    });

    it("uses the voice it was given", async () => {
        const dir = await mkdtemp(join(tmpdir(), "tts-"));
        const voiceId = "31001ed1e3e54c79a88458cb03ca123a";
        const fetchImpl = (async (url: string | URL | Request) =>
            String(url).startsWith("https://api.heygen.com") ? ttsOK() : wav()) as unknown as typeof fetch;
        const out = await synthesizeVoice({ projectDir: dir, apiKey: "k", text: "x", voiceId, fetchImpl });
        assert.equal(out.voiceId, voiceId);
        assert.equal(out.path, voiceFileName("x", voiceId));
    });

    it("retries a 429 and honours its Retry-After", async () => {
        const dir = await mkdtemp(join(tmpdir(), "tts-"));
        const slept: number[] = [];
        let attempts = 0;
        const fetchImpl = (async (url: string | URL | Request) => {
            if (!String(url).startsWith("https://api.heygen.com")) return wav();
            attempts++;
            if (attempts === 1) {
                return new Response("rate_limit_exceeded", { status: 429, headers: { "retry-after": "2" } });
            }
            return ttsOK();
        }) as unknown as typeof fetch;

        const out = await synthesizeVoice({
            projectDir: dir,
            apiKey: "k",
            text: "x",
            fetchImpl,
            sleep: async (ms) => void slept.push(ms),
        });
        assert.equal(attempts, 2);
        assert.deepEqual(slept, [2000]);
        assert.ok(out.path.endsWith(".wav"));
    });

    it("gives up at once on a rejected key, without retrying", async () => {
        const dir = await mkdtemp(join(tmpdir(), "tts-"));
        let attempts = 0;
        const fetchImpl = (async () => {
            attempts++;
            return new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 });
        }) as unknown as typeof fetch;

        await assert.rejects(
            synthesizeVoice({ projectDir: dir, apiKey: "bad", text: "x", fetchImpl }),
            /HTTP 401/
        );
        assert.equal(attempts, 1);
    });

    it("reports an empty download rather than writing a zero-byte film asset", async () => {
        const dir = await mkdtemp(join(tmpdir(), "tts-"));
        const fetchImpl = (async (url: string | URL | Request) =>
            String(url).startsWith("https://api.heygen.com")
                ? ttsOK()
                : new Response(Buffer.alloc(0), { status: 200 })) as unknown as typeof fetch;
        await assert.rejects(
            synthesizeVoice({ projectDir: dir, apiKey: "k", text: "x", fetchImpl }),
            /came back empty/
        );
    });

    it("refuses an empty line before it calls anything", async () => {
        let called = false;
        const fetchImpl = (async () => {
            called = true;
            return ttsOK();
        }) as unknown as typeof fetch;
        await assert.rejects(
            synthesizeVoice({ projectDir: tmpdir(), apiKey: "k", text: "   ", fetchImpl }),
            /nothing to say/
        );
        assert.equal(called, false);
    });
});
