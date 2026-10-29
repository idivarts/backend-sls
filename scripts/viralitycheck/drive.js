#!/usr/bin/env node
/**
 * Drives the deployed dev websocket and checks what the model actually writes.
 *
 * Phase 0 of the Virality Engine changes the system prompt, and nothing about
 * that is visible to a unit test or a green deploy. This connects to dev as a
 * real authenticated manager, asks for a script on each fixture content, and
 * runs the Phase 0 acceptance list over the result.
 *
 * The checks are mechanical on purpose. "I read the scripts and they look
 * better" is not a verification anyone can disagree with later; a throat-clear
 * regex that either fires or does not is.
 *
 * Usage, from the backend-sls root, after ./scripts/viralitycheck/seed has run:
 *
 *   NODE_PATH=../trendly-brands/node_modules node scripts/viralitycheck/drive.js
 *
 * Optional: --fixture <path>  --out <dir>  --only <contentId>
 */

const fs = require("fs");
const os = require("os");
const path = require("path");
const WebSocket = require("ws");

const args = process.argv.slice(2);
const argOf = (name, fallback) => {
    const i = args.indexOf(name);
    return i >= 0 && args[i + 1] ? args[i + 1] : fallback;
};

const FIXTURE = argOf("--fixture", path.join(os.tmpdir(), "trendly-viralitycheck-fixture.json"));
const OUT_DIR = argOf("--out", path.join(os.tmpdir(), "viralitycheck-runs", new Date().toISOString().replace(/[:.]/g, "-")));
const ONLY = argOf("--only", null);

/**
 * The Phase 0 acceptance list, as checks.
 *
 * `fatal` marks the ones that are the whole point of the phase — a run with a
 * fatal failure has not met acceptance, whatever the prose looks like.
 */
const CHECKS = [
    {
        id: "no-throat-clearing",
        fatal: true,
        why: "the opening must state the subject, not greet",
        // Applied to the first ~120 chars of the hook only: "hi" inside the
        // body of a script about customer greetings is not a hook problem.
        scope: "hookOpening",
        pattern: /\b(hey (guys|everyone|there)|hi (guys|everyone|there)|what'?s up|in this (video|one)|let me tell you|so basically|today (i|we)'?(m|re|ll)? ?(going to|gonna)?|welcome back)\b/i,
    },
    {
        id: "no-engagement-bait",
        fatal: true,
        why: "Meta demotes it and LinkedIn suppresses it",
        scope: "full",
        pattern: /\b(like (this )?if|tag someone who|comment ['"]?yes['"]?|share if you|double tap if|drop a ['"]?\w+['"]? (in the comments|below) if)\b/i,
    },
    {
        id: "no-follow-for-more",
        fatal: false,
        why: "almost never the ask the surface rewards",
        scope: "cta",
        pattern: /\bfollow (me |us )?for more\b/i,
    },
    {
        id: "has-hook-section",
        fatal: true,
        why: "the generator is asked for an explicit hook",
        scope: "full",
        pattern: /##\s*hook/i,
        expect: true,
    },
    {
        id: "has-cta-section",
        fatal: true,
        why: "exactly one ask, and it has to be identifiable",
        scope: "full",
        pattern: /##\s*(cta|call to action)/i,
        expect: true,
    },
];

/** Pull the named markdown section out of a script. */
function section(script, heading) {
    const re = new RegExp(`##\\s*${heading}[^\\n]*\\n([\\s\\S]*?)(?=\\n##\\s|$)`, "i");
    const m = script.match(re);
    return m ? m[1].trim() : "";
}

function scopeText(script, scope) {
    if (scope === "full") return script;
    if (scope === "cta") return section(script, "(cta|call to action)") || script.slice(-400);
    if (scope === "hookOpening") {
        const hook = section(script, "hook");
        return (hook || script).slice(0, 160);
    }
    return script;
}

function runChecks(script) {
    return CHECKS.map((c) => {
        const text = scopeText(script, c.scope);
        const found = c.pattern.test(text);
        const pass = c.expect === true ? found : !found;
        return { id: c.id, fatal: c.fatal, why: c.why, pass, evidence: found ? (text.match(c.pattern) || [""])[0] : "" };
    });
}

/** One script generation over the live dev socket. */
function generate(fixture, item) {
    return new Promise((resolve) => {
        const url = `wss://${fixture.wsHost}?token=${encodeURIComponent(fixture.idToken)}`;
        const ws = new WebSocket(url);
        let out = "";
        let usage = null;
        let problems = [];
        let failure = null;

        // The socket can go quiet if a lambda dies mid-stream; without this the
        // run would hang forever instead of reporting a failed case.
        const timer = setTimeout(() => {
            failure = failure || "timeout after 180s";
            try { ws.close(); } catch { /* already gone */ }
        }, 180_000);

        ws.on("open", () => {
            ws.send(JSON.stringify({
                type: "content_gen",
                task: "script",
                brandId: fixture.brandId,
                payload: {
                    brandId: fixture.brandId,
                    contextId: item.contentId,
                    videoType: item.videoType,
                    topic: item.topic,
                    keyMessage: item.keyMessage,
                    tone: item.tone,
                    title: item.topic,
                    platform: item.platform,
                    format: item.format,
                },
            }));
        });

        ws.on("message", (raw) => {
            let msg;
            try { msg = JSON.parse(raw.toString()); } catch { return; }
            if (msg.type === "token") out += msg.delta || "";
            else if (msg.type === "done") { usage = msg.usage || null; problems = msg.problems || []; clearTimeout(timer); ws.close(); }
            else if (msg.type === "error") { failure = msg.message || "error"; clearTimeout(timer); ws.close(); }
            else if (msg.type === "upgrade_required") { failure = `upgrade_required (${msg.reason || msg.task})`; clearTimeout(timer); ws.close(); }
        });

        ws.on("error", (e) => { failure = failure || String(e.message || e); });
        ws.on("close", () => { clearTimeout(timer); resolve({ script: out, usage, problems, failure }); });
    });
}

const CASES = [
    {
        contentId: "viralitycheck-reel", platform: "instagram", format: "reel", videoType: "Instagram Reel",
        topic: "Why our cold brew takes 18 hours", keyMessage: "Slow steeping is what removes the bitterness", tone: "warm and direct",
    },
    {
        contentId: "viralitycheck-short", platform: "youtube", format: "reel", videoType: "YouTube Short",
        topic: "The grinder mistake costing you flavour", keyMessage: "Blade grinders destroy extraction consistency", tone: "practical",
    },
    {
        contentId: "viralitycheck-linkedin", platform: "linkedin", format: "video", videoType: "LinkedIn video",
        topic: "What we learned opening a second location", keyMessage: "The second site cost 40% more than the first because of staffing, not rent", tone: "candid and professional",
    },
    {
        contentId: "viralitycheck-twitter", platform: "twitter", format: "video", videoType: "X video",
        topic: "Our espresso pricing, broken down", keyMessage: "A 220 rupee espresso has 31 rupees of coffee in it", tone: "transparent",
    },
];

(async () => {
    const fixture = JSON.parse(fs.readFileSync(FIXTURE, "utf8"));
    fs.mkdirSync(OUT_DIR, { recursive: true });

    const cases = ONLY ? CASES.filter((c) => c.contentId === ONLY) : CASES;
    const results = [];
    let totalTokens = 0;

    for (const item of cases) {
        process.stdout.write(`\n── ${item.platform} · ${item.contentId}\n`);
        const { script, usage, problems, failure } = await generate(fixture, item);

        if (failure) {
            process.stdout.write(`   FAILED: ${failure}\n`);
            results.push({ ...item, failure });
            continue;
        }

        const checks = runChecks(script);
        const fatalFails = checks.filter((c) => !c.pass && c.fatal);
        const softFails = checks.filter((c) => !c.pass && !c.fatal);
        totalTokens += (usage && (usage.total_tokens || usage.totalTokens)) || 0;

        fs.writeFileSync(path.join(OUT_DIR, `${item.contentId}.md`), script);
        for (const c of checks) {
            const mark = c.pass ? "pass" : c.fatal ? "FAIL" : "warn";
            process.stdout.write(`   [${mark}] ${c.id}${c.evidence && !c.pass ? ` → "${c.evidence}"` : ""}\n`);
        }
        // What the BACKEND validator said about the same script. The regex
        // checks above are this script's own opinion; these come from the
        // deployed lambda, so a disagreement between them is the interesting
        // signal — usually a false positive the handcrafted unit-test fixtures
        // could never have surfaced.
        if (problems.length === 0) {
            process.stdout.write(`   [pass] backend validator: no problems\n`);
        } else {
            for (const p of problems) {
                process.stdout.write(`   [${p.severity === "warn" ? "warn" : "FAIL"}] backend ${p.id}: ${p.message}\n`);
            }
        }

        process.stdout.write(`   ${script.split(/\s+/).length} words, ${(usage && (usage.total_tokens || usage.totalTokens)) || "?"} tokens\n`);

        results.push({ ...item, checks, problems, words: script.split(/\s+/).length, usage, fatalFails: fatalFails.length, softFails: softFails.length });
    }

    fs.writeFileSync(path.join(OUT_DIR, "summary.json"), JSON.stringify({ ranAt: new Date().toISOString(), results }, null, 2));

    const failed = results.filter((r) => r.failure || r.fatalFails > 0);
    process.stdout.write(`\n${"─".repeat(60)}\n`);
    process.stdout.write(`${results.length - failed.length}/${results.length} cases met the Phase 0 acceptance list\n`);
    process.stdout.write(`~${totalTokens} tokens spent\n`);
    process.stdout.write(`transcripts: ${OUT_DIR}\n`);
    process.exit(failed.length === 0 ? 0 : 1);
})();
