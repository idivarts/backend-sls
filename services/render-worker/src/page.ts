/**
 * Load a design revision's HTML into a page that is ready to be captured.
 *
 * The HTML is served over loopback rather than pushed in with `setContent`, so
 * the document has a real origin: relative URLs resolve, web fonts load under
 * normal rules, and the page behaves exactly as it would in the app's frame.
 *
 * Note what is NOT here: any CORS handling. The old client renderer had to
 * re-fetch every image itself, which is why a missing CloudFront CORS header
 * silently dropped the brand logo from exports. A screenshot reads painted
 * pixels, so cross-origin assets simply work.
 */
import { createServer, Server } from "node:http";
import type { AddressInfo } from "node:net";
import type { Browser, Page } from "playwright";
import {
    measureDesign,
    prepareForCapture,
    slideRects,
    waitForPaint,
} from "../../../shared-libs/design/design-runtime";
import { config } from "./config";

export interface LoadedDesign {
    page: Page;
    close: () => Promise<void>;
    measurement: Awaited<ReturnType<typeof measure>>;
    rects: { x: number; y: number; width: number; height: number }[];
}

type Measurement = {
    count: number;
    offsets: number[];
    total: number;
    width: number;
    height: number;
};

async function measure(page: Page): Promise<Measurement> {
    return page.evaluate(measureDesign);
}

function serve(html: string): Promise<{ url: string; server: Server }> {
    return new Promise((resolve) => {
        const server = createServer((_req, res) => {
            res.writeHead(200, { "content-type": "text/html; charset=utf-8" });
            res.end(html);
        });
        server.listen(0, "127.0.0.1", () => {
            const { port } = server.address() as AddressInfo;
            resolve({ url: `http://127.0.0.1:${port}/`, server });
        });
    });
}

export async function loadDesign(
    browser: Browser,
    html: string,
    width: number,
    height: number
): Promise<LoadedDesign> {
    const { url, server } = await serve(html);

    const context = await browser.newContext({
        viewport: { width, height },
        deviceScaleFactor: config.scale,
        // The design decides its own colours; a forced scheme would repaint a
        // design the user never saw that way.
        colorScheme: "light",
        reducedMotion: "no-preference",
    });
    const page = await context.newPage();

    await page.goto(url, { waitUntil: "load", timeout: 60_000 });

    // Strip editor chrome and the preview's display transform BEFORE measuring,
    // so the boxes we read are the design's real layout, not a scaled-down one.
    await page.evaluate(prepareForCapture);
    await page.evaluate(waitForPaint, config.imageTimeoutMs);

    const measurement = await measure(page);
    const rects = await page.evaluate(slideRects);

    return {
        page,
        measurement,
        rects,
        close: async () => {
            await context.close().catch(() => undefined);
            await new Promise<void>((done) => server.close(() => done()));
        },
    };
}
