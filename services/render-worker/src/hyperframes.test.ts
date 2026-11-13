import { strict as assert } from "node:assert";
import { mkdtemp, mkdir, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

import { hyperframesEnv, listProjectFiles, vendorGsap } from "./hyperframes";

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

test("a project with no gsap tag gets one, rather than rendering motionless", async () => {
    const dir = await project(`<!doctype html><html><head><meta charset="utf-8"></head><body></body></html>`);
    await vendorGsap(dir);
    const html = await readFile(join(dir, "index.html"), "utf8");
    assert.ok(html.includes("assets/vendor/gsap-3.15.0.min.js"));
    assert.equal(html.match(/gsap-3\.15\.0\.min\.js/g)?.length, 1, "exactly one tag");
});

test("vendoring twice does not duplicate the tag", async () => {
    const dir = await project(`<!doctype html><html><head></head><body></body></html>`);
    await vendorGsap(dir);
    await vendorGsap(dir);
    const html = await readFile(join(dir, "index.html"), "utf8");
    assert.equal(html.match(/gsap-3\.15\.0\.min\.js/g)?.length, 1, "exactly one tag after two runs");
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
