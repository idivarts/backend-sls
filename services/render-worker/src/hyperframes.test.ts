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

test("no provider key is handed to the CLI", () => {
    // Media generation is a separate, metered step. A key that is absent from
    // this environment is a capability that cannot be billed by accident, so
    // this is enforcement rather than convention.
    const env = hyperframesEnv();
    for (const key of ["HEYGEN_API_KEY", "HEYGEN_ACCESS_TOKEN", "HYPERFRAMES_API_KEY", "ELEVENLABS_API_KEY"]) {
        assert.equal(env[key], undefined, `${key} must not reach the render subprocess`);
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
