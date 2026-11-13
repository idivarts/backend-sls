import { strict as assert } from "node:assert";
import { mkdtemp, mkdir, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

import { hyperframesEnv, listProjectFiles, patchGsapTag, vendorGsap } from "./hyperframes";

async function project(indexHtml: string): Promise<string> {
    const dir = await mkdtemp(join(tmpdir(), "hf-test-"));
    await writeFile(join(dir, "index.html"), indexHtml, "utf8");
    return dir;
}

test("telemetry is off on every invocation", () => {
    const env = hyperframesEnv();
    assert.equal(env.HYPERFRAMES_NO_TELEMETRY, "1");
    assert.equal(env.DO_NOT_TRACK, "1");
});

test("no provider key is handed to the CLI, even when the task has one", () => {
    // The first version of this test only checked that the keys were absent,
    // which passed vacuously because the local environment has none. It would
    // have gone on passing after Phase 4 adds HEYGEN_API_KEY to
    // provider.environment, while every render subprocess quietly inherited a
    // billable credential. So the keys are INJECTED here and the allowlist has
    // to drop them.
    const injected = {
        HEYGEN_API_KEY: "hg-should-not-leak",
        HEYGEN_ACCESS_TOKEN: "hg-token-should-not-leak",
        HYPERFRAMES_API_KEY: "hf-should-not-leak",
        ELEVENLABS_API_KEY: "el-should-not-leak",
        OPENROUTER_API_KEY: "or-should-not-leak",
        AWS_SECRET_ACCESS_KEY: "aws-should-not-leak",
    };
    const saved: Record<string, string | undefined> = {};
    for (const [k, v] of Object.entries(injected)) {
        saved[k] = process.env[k];
        process.env[k] = v;
    }
    try {
        const env = hyperframesEnv();
        for (const key of Object.keys(injected)) {
            assert.equal(env[key], undefined, `${key} must not reach the render subprocess`);
        }
        // and the allowlist still gives the subprocess what it needs to run
        assert.ok(env.PATH, "PATH must survive the allowlist");
        assert.equal(env.HYPERFRAMES_NO_TELEMETRY, "1");
    } finally {
        for (const [k, v] of Object.entries(saved)) {
            if (v === undefined) delete process.env[k];
            else process.env[k] = v;
        }
    }
});

test("a CDN gsap tag is rewritten to the vendored copy", async () => {
    const dir = await project(
        `<!doctype html><html><head>\n<script src="https://cdn.jsdelivr.net/npm/gsap@3.15.0/dist/gsap.min.js"></script>\n</head><body></body></html>`
    );
    await vendorGsap(dir);
    const html = await readFile(join(dir, "index.html"), "utf8");

    assert.ok(!html.includes("cdn.jsdelivr.net"), "the CDN tag must be gone");
    assert.ok(html.includes("assets/vendor/gsap-3.15.0.min.js"), "the local tag must be present");

    const files = await listProjectFiles(dir);
    assert.ok(files.includes("assets/vendor/gsap-3.15.0.min.js"), "gsap must be copied into the project");
});

// These two used an index.html with an empty body that never mentioned gsap,
// and asserted a tag was added anyway. That was the old unconditional
// behaviour; injection is now keyed on actual use, which is what the CLI's own
// check asks about ("Composition USES GSAP but no GSAP script is loaded"). A
// document that animates nothing does not need 72 KB. So the fixture is now a
// document that really does use GSAP — the realistic case, and the one where
// a missing tag costs a charged render.
test("a project with no gsap tag gets one, rather than rendering motionless", async () => {
    const dir = await project(
        `<!doctype html><html><head><meta charset="utf-8"></head><body>` +
            `<script>window.__timelines["main"] = gsap.timeline({ paused: true });</script>` +
            `</body></html>`
    );
    await vendorGsap(dir);
    const html = await readFile(join(dir, "index.html"), "utf8");
    assert.ok(html.includes("assets/vendor/gsap-3.15.0.min.js"));
    assert.equal(html.match(/gsap-3\.15\.0\.min\.js/g)?.length, 1, "exactly one tag");
});

test("vendoring twice does not duplicate the tag", async () => {
    const dir = await project(
        `<!doctype html><html><head></head><body><script>gsap.timeline()</script></body></html>`
    );
    await vendorGsap(dir);
    await vendorGsap(dir);
    const html = await readFile(join(dir, "index.html"), "utf8");
    assert.equal(html.match(/gsap-3\.15\.0\.min\.js/g)?.length, 1, "exactly one tag after two runs");
});

test("a project that animates nothing is not given GSAP it cannot use", async () => {
    const dir = await project(`<!doctype html><html><head></head><body><div>still</div></body></html>`);
    await vendorGsap(dir);
    const html = await readFile(join(dir, "index.html"), "utf8");
    assert.ok(!html.includes("gsap-3.15.0.min.js"), html);
});

test("listProjectFiles ignores the CLI's own artifact directory", async () => {
    const dir = await project(`<!doctype html><html><head></head><body></body></html>`);
    await mkdir(join(dir, "compositions"), { recursive: true });
    await writeFile(join(dir, "compositions", "hook.html"), "<template></template>", "utf8");
    await mkdir(join(dir, ".hyperframes", "artifacts"), { recursive: true });
    await writeFile(join(dir, ".hyperframes", "artifacts", "noise.json"), "{}", "utf8");

    const files = await listProjectFiles(dir);
    assert.deepEqual(files, ["compositions/hook.html", "index.html"]);
});

test("a non-existent ffmpeg path is dropped rather than passed on", () => {
    // Setting the variable to a path that does not exist is worse than not
    // setting it: the CLI trusts it and then tells the user to install an
    // ffmpeg that is already on PATH. Our image puts it in /usr/bin, theirs
    // in /usr/local/bin, so an absolute default is wrong somewhere by
    // construction.
    const env = hyperframesEnv({ ffmpegPath: "/nope/ffmpeg", ffprobePath: "/nope/ffprobe" });
    assert.equal(env.HYPERFRAMES_FFMPEG_PATH, undefined);
    assert.equal(env.HYPERFRAMES_FFPROBE_PATH, undefined);
});

test("an ffmpeg path that exists is passed through", () => {
    const real = process.execPath; // any file that definitely exists
    const env = hyperframesEnv({ ffmpegPath: real });
    assert.equal(env.HYPERFRAMES_FFMPEG_PATH, real);
});

test("the CLI is told where a browser is", () => {
    // Hyperframes downloads chrome-headless-shell from chrome-for-testing,
    // which publishes no linux-arm64 build. On our Graviton worker it gives
    // up with "Chrome Headless Shell is not available for Linux ARM64" and
    // suggests apt-get. The image already has Playwright's Chromium, so the
    // only thing missing was telling the CLI about it — and nothing in a
    // unit test would have caught that, so this at least locks in that the
    // variable is populated wherever a browser can be resolved.
    const env = hyperframesEnv();
    const resolvable = (() => {
        try {
            // eslint-disable-next-line @typescript-eslint/no-var-requires
            const { chromium } = require("playwright");
            const p = chromium.executablePath();
            return !!p && require("node:fs").existsSync(p);
        } catch {
            return false;
        }
    })();
    if (resolvable) {
        assert.ok(env.HYPERFRAMES_BROWSER_PATH, "a resolvable Chromium must be passed to the CLI");
    } else {
        assert.equal(env.HYPERFRAMES_BROWSER_PATH, undefined,
            "an unresolvable browser must be left unset, not pointed at a missing path");
    }
});

test("an explicit browser path wins over discovery", () => {
    const prev = process.env.HYPERFRAMES_BROWSER_PATH;
    process.env.HYPERFRAMES_BROWSER_PATH = "/explicit/chrome";
    try {
        assert.equal(hyperframesEnv().HYPERFRAMES_BROWSER_PATH, "/explicit/chrome");
    } finally {
        if (prev === undefined) delete process.env.HYPERFRAMES_BROWSER_PATH;
        else process.env.HYPERFRAMES_BROWSER_PATH = prev;
    }
});

const LOCAL_GSAP = "assets/vendor/gsap-3.15.0.min.js";

/**
 * The bug this function exists for. A project whose three compositions each
 * called gsap.timeline() with no script tag anywhere failed a real dev render
 * with "Composition uses GSAP but no GSAP script is loaded" — and the user was
 * charged ~2000 tokens for the nothing it produced. The run before it had
 * passed, because THAT model output happened to include a CDN tag.
 *
 * Patching index.html alone was the first version. A composition is a separate
 * document the runtime mounts in its own context, so the tag never reached it.
 */
test("GSAP is injected into a composition that uses it without loading it", () => {
    const html = [
        "<!doctype html><html><head><title>Hook</title></head><body>",
        '  <div class="scene"></div>',
        "  <script>const tl = gsap.timeline({ paused: true });</script>",
        "</body></html>",
    ].join("\n");
    const out = patchGsapTag(html, "compositions/hook.html");
    assert.ok(out.includes(`<script src="../${LOCAL_GSAP}"></script>`), out);
    // Ahead of the code that uses it, or it is no better than absent.
    assert.ok(out.indexOf("../" + LOCAL_GSAP) < out.indexOf("gsap.timeline"));
});

/**
 * A composition one directory down cannot reach "assets/..." — that resolves
 * to "compositions/assets/...", a 404, and the check then fails with the same
 * message as having no tag at all.
 */
test("the vendored path walks up out of a subdirectory", () => {
    const out = patchGsapTag("<head></head><script>gsap.to(x)</script>", "compositions/a/b/deep.html");
    assert.ok(out.includes(`src="../../../${LOCAL_GSAP}"`), out);
});

test("the vendored path is bare at the project root", () => {
    const out = patchGsapTag("<head></head><script>gsap.timeline()</script>", "index.html");
    assert.ok(out.includes(`src="${LOCAL_GSAP}"`), out);
    assert.ok(!out.includes("../"));
});

test("a CDN tag is rewritten, not left as a network dependency", () => {
    const html = '<head><script src="https://cdn.jsdelivr.net/npm/gsap@3.12/dist/gsap.min.js"></script></head>';
    const out = patchGsapTag(html, "index.html");
    assert.ok(!out.includes("jsdelivr"));
    assert.ok(out.includes(`src="${LOCAL_GSAP}"`));
});

test("a document already loading GSAP locally is left alone", () => {
    const html = `<head><script src="../${LOCAL_GSAP}"></script></head><script>gsap.to(x)</script>`;
    assert.equal(patchGsapTag(html, "compositions/hook.html"), html);
});

// 72 KB per document is not free, and the check does not ask for GSAP in a
// composition that does not animate.
test("a document that never uses GSAP is not given it", () => {
    const html = "<head></head><body><div>static card</div></body>";
    assert.equal(patchGsapTag(html, "compositions/static.html"), html);
});

test("a word merely containing gsap is not mistaken for a call", () => {
    const html = "<head></head><body><p>gsapless design</p></body>";
    assert.equal(patchGsapTag(html, "index.html"), html);
});

// A missing </head> was the old code's silent no-op: the replace just missed,
// and the render failed as if nothing had been done.
test("injection falls back to body when there is no head", () => {
    const out = patchGsapTag("<body><script>gsap.timeline()</script></body>", "index.html");
    assert.ok(out.includes(`src="${LOCAL_GSAP}"`), out);
    assert.ok(out.indexOf(LOCAL_GSAP) < out.indexOf("gsap.timeline"));
});

test("injection falls back to prepending for a bare fragment", () => {
    const out = patchGsapTag("<div></div><script>gsap.to(x)</script>", "compositions/frag.html");
    assert.ok(out.startsWith(`<script src="../${LOCAL_GSAP}"></script>`), out);
});

// The end-to-end shape: vendorGsap must reach every document, which is what
// the index.html-only version did not do.
test("vendorGsap patches every composition, not just index.html", async () => {
    const dir = await mkdtemp(join(tmpdir(), "hf-gsap-"));
    await writeFile(join(dir, "index.html"), "<head></head><script>gsap.timeline()</script>", "utf8");
    await mkdir(join(dir, "compositions"), { recursive: true });
    await writeFile(join(dir, "compositions", "hook.html"), "<head></head><script>gsap.timeline()</script>", "utf8");
    await writeFile(join(dir, "compositions", "outro.html"), "<head></head><script>gsap.to(x, {})</script>", "utf8");

    await vendorGsap(dir);

    assert.ok((await readFile(join(dir, "index.html"), "utf8")).includes(`src="${LOCAL_GSAP}"`));
    for (const f of ["hook.html", "outro.html"]) {
        const html = await readFile(join(dir, "compositions", f), "utf8");
        assert.ok(html.includes(`src="../${LOCAL_GSAP}"`), `${f} was not patched: ${html}`);
    }
    // And the file it points at is actually there.
    const files = await listProjectFiles(dir);
    assert.ok(files.includes(LOCAL_GSAP), files.join(","));
});
