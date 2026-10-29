#!/usr/bin/env node
/**
 * Exercises hook laddering against deployed dev.
 *
 * What it is actually checking is not "did it return five strings" but whether
 * the five are DIFFERENT — a ladder of one archetype wearing five hats is the
 * failure mode worth catching, and it is the one a passing status code hides.
 * So it reports the distinct archetypes, the score spread across the five, and
 * flags any pair whose opening words match.
 *
 * Usage, from the backend-sls root, after ./scripts/viralitycheck/seed:
 *
 *   node scripts/viralitycheck/hooks.js [--content viralitycheck-reel]
 *                                       [--platform instagram] [--runs 1]
 */

const fs = require("fs");
const os = require("os");
const path = require("path");

const args = process.argv.slice(2);
const argOf = (n, d) => {
    const i = args.indexOf(n);
    return i >= 0 && args[i + 1] ? args[i + 1] : d;
};

const FIXTURE = argOf("--fixture", path.join(os.tmpdir(), "trendly-viralitycheck-fixture.json"));
const CONTENT_ID = argOf("--content", "viralitycheck-reel");
const PLATFORM = argOf("--platform", "instagram");
const FORMAT = argOf("--format", "reel");
const RUNS = parseInt(argOf("--runs", "1"), 10);
const BASE = argOf("--base", "https://be.trendly.now/dev");

async function ladder(fixture) {
    const res = await fetch(`${BASE}/api/ai/content/hooks`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${fixture.idToken}`,
        },
        body: JSON.stringify({
            brandId: fixture.brandId,
            contextId: CONTENT_ID,
            format: FORMAT,
            platform: PLATFORM,
        }),
    });
    const text = await res.text();
    if (!res.ok) throw new Error(`${res.status}: ${text.slice(0, 400)}`);
    return JSON.parse(text);
}

/** First three words, lowercased — enough to spot five rewrites of one hook. */
const opener = (s) => s.toLowerCase().split(/\s+/).slice(0, 3).join(" ");

(async () => {
    const fixture = JSON.parse(fs.readFileSync(FIXTURE, "utf8"));
    let failures = 0;

    for (let r = 0; r < RUNS; r++) {
        const t0 = Date.now();
        let out;
        try {
            out = await ladder(fixture);
        } catch (e) {
            process.stdout.write(`run ${r + 1}: FAILED ${e.message}\n`);
            failures++;
            continue;
        }
        const hooks = (out.ladder && out.ladder.hooks) || [];
        process.stdout.write(
            `\nrun ${r + 1}: ${hooks.length} hooks in ${((Date.now() - t0) / 1000).toFixed(1)}s` +
            `  (~${(out.usage && out.usage.total_tokens) || 0} tokens)\n`,
        );
        if (out.warning) process.stdout.write(`  WARNING: ${out.warning}\n`);

        hooks.forEach((h, i) => {
            process.stdout.write(
                `  ${String(h.score).padStart(3)}  [${(h.archetype || "?").padEnd(22)}] ${h.text}\n`,
            );
            if (h.onScreen) process.stdout.write(`       on-screen: ${h.onScreen}\n`);
            for (const n of h.notes || []) process.stdout.write(`       · ${n}\n`);
            for (const p of h.problems || []) process.stdout.write(`       ! ${p.severity}: ${p.message}\n`);
        });

        // Ranked, best first.
        const scores = hooks.map((h) => h.score);
        const sorted = [...scores].sort((a, b) => b - a);
        if (JSON.stringify(scores) !== JSON.stringify(sorted)) {
            process.stdout.write(`  FAIL: not returned best-first (${scores.join(", ")})\n`);
            failures++;
        }

        // Five of the same hook is the failure a 200 hides.
        const archetypes = new Set(hooks.map((h) => (h.archetype || "").toLowerCase()).filter(Boolean));
        const openers = hooks.map((h) => opener(h.text));
        const dupeOpeners = openers.length - new Set(openers).size;
        process.stdout.write(
            `  distinct archetypes ${archetypes.size}/${hooks.length}` +
            `, score range ${Math.min(...scores)}–${Math.max(...scores)}` +
            `, duplicate openers ${dupeOpeners}\n`,
        );
        if (hooks.length >= 3 && archetypes.size < 3) {
            process.stdout.write("  FAIL: the ladder collapsed onto one or two archetypes\n");
            failures++;
        }
        if (dupeOpeners > 0) {
            process.stdout.write("  FAIL: two hooks open with the same words\n");
            failures++;
        }
        if (hooks.length < 3) {
            process.stdout.write("  FAIL: too few hooks to choose between\n");
            failures++;
        }
    }

    process.stdout.write(failures === 0 ? "\nall checks passed\n" : `\n${failures} check(s) failed\n`);
    process.exit(failures === 0 ? 0 : 1);
})();
