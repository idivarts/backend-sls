#!/usr/bin/env node
/**
 * Scores a script against the deployed dev rubric, repeatedly, and reports
 * how much the number moves.
 *
 * Stability is the acceptance criterion that matters for Phase 2. A score
 * that wanders by fifteen points between identical runs is worse than no
 * score: people notice the wobble, stop trusting the number, and the fixes
 * attached to it get ignored with it. The target is ±4 overall across five
 * runs at a pinned judge model.
 *
 * Usage, from the backend-sls root, after ./scripts/viralitycheck/seed:
 *
 *   node scripts/viralitycheck/score.js [--script <file.md>] [--runs 5]
 *                                       [--format reel] [--platform instagram]
 *
 * With no --script it scores the seeded fixture content by id, which also
 * exercises the saved-doc fallback path.
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
const SCRIPT_FILE = argOf("--script", null);
const RUNS = parseInt(argOf("--runs", "5"), 10);
const FORMAT = argOf("--format", "reel");
const PLATFORM = argOf("--platform", "instagram");
const CONTENT_ID = argOf("--content", "viralitycheck-reel");
const BASE = argOf("--base", "https://be.trendly.now/dev");

async function score(fixture, body) {
    const res = await fetch(`${BASE}/api/ai/content/score`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${fixture.idToken}`,
        },
        body: JSON.stringify(body),
    });
    const text = await res.text();
    if (!res.ok) throw new Error(`${res.status}: ${text.slice(0, 400)}`);
    return JSON.parse(text);
}

function bar(n, width = 20) {
    const filled = Math.round((n / 100) * width);
    return "█".repeat(filled) + "░".repeat(width - filled);
}

(async () => {
    const fixture = JSON.parse(fs.readFileSync(FIXTURE, "utf8"));
    const body = {
        brandId: fixture.brandId,
        contextId: CONTENT_ID,
        format: FORMAT,
        platform: PLATFORM,
    };
    if (SCRIPT_FILE) body.script = fs.readFileSync(SCRIPT_FILE, "utf8");

    const overalls = [];
    const perDim = {};
    let tokens = 0;

    for (let i = 0; i < RUNS; i++) {
        const t0 = Date.now();
        let out;
        try {
            out = await score(fixture, body);
        } catch (e) {
            process.stdout.write(`run ${i + 1}: FAILED ${e.message}\n`);
            continue;
        }
        const s = out.score;
        overalls.push(s.overall);
        tokens += (out.usage && (out.usage.total_tokens || out.usage.totalTokens)) || 0;

        const dimStr = s.dimensions
            .map((d) => {
                (perDim[d.label] = perDim[d.label] || []).push(d.score);
                return `${d.label} ${String(d.score).padStart(3)}`;
            })
            .join("  ");
        process.stdout.write(
            `run ${i + 1}: ${String(s.overall).padStart(3)} ${bar(s.overall)} ${s.band.padEnd(12)} ${dimStr}  (${((Date.now() - t0) / 1000).toFixed(1)}s)\n`,
        );
        if (s.cap) process.stdout.write(`        capped at ${s.cap.at}: ${s.cap.reason}\n`);
        if (i === 0) {
            for (const f of s.fixes) process.stdout.write(`        fix ${f.criterion} (${f.dimension}): ${f.message}\n`);
            process.stdout.write(`        rubric ${s.rubricVersion}  judge ${s.judgeModel}\n`);
        }
    }

    if (overalls.length === 0) {
        process.stdout.write("\nno successful runs\n");
        process.exit(1);
    }

    const lo = Math.min(...overalls);
    const hi = Math.max(...overalls);
    const spread = hi - lo;
    process.stdout.write(`\n${"─".repeat(72)}\n`);
    process.stdout.write(`overall ${lo}–${hi}, spread ${spread} (target ≤ 8, i.e. ±4)\n`);
    for (const [label, vals] of Object.entries(perDim)) {
        const s = Math.max(...vals) - Math.min(...vals);
        process.stdout.write(`  ${label.padEnd(8)} ${Math.min(...vals)}–${Math.max(...vals)}  spread ${s}\n`);
    }
    process.stdout.write(`~${tokens} tokens across ${overalls.length} runs\n`);
    process.exit(spread <= 8 ? 0 : 1);
})();
