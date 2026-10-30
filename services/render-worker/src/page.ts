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
    type DesignMeasurement,
    prepareForCapture,
    slideRects,
    waitForPaint,
} from "../../../shared-libs/design/design-runtime";
import { config } from "./config";

export interface LoadedDesign {
    page: Page;
    /** True when this design was loaded for transparent capture. */
    transparent: boolean;
    close: () => Promise<void>;
    measurement: Awaited<ReturnType<typeof measure>>;
    rects: { x: number; y: number; width: number; height: number }[];
}

// The measurement type is NOT restated here. It used to be a hand-written copy
// of DesignMeasurement, which silently went stale the moment the runtime grew
// per-scene durations and transitions — a local type that merely LOOKS like the
// real one hides exactly the fields a caller needs.
async function measure(page: Page): Promise<DesignMeasurement> {
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
    height: number,
    // Overrides config.scale. The preview path renders small on purpose, so
    // Chromium paints once at the size we want instead of painting full size
    // and throwing most of the pixels away.
    scale?: number,
    // Capture with no page background, so the design composites over whatever
    // is beneath it. An overlay — captions, a lower third, a logo bug — is just
    // a design captured this way, which is what lets it inherit the brand's
    // fonts, the palette and the editor for free.
    transparent = false
): Promise<LoadedDesign> {
    const { url, server } = await serve(html);

    const context = await browser.newContext({
        viewport: { width, height },
        deviceScaleFactor: scale ?? config.scale,
        // The design decides its own colours; a forced scheme would repaint a
        // design the user never saw that way.
        colorScheme: "light",
        reducedMotion: "no-preference",
    });
    const page = await context.newPage();

    await page.goto(url, { waitUntil: "load", timeout: 60_000 });

    // omitBackground alone is not enough. It drops the BROWSER's base canvas,
    // but html/body keep whatever background the document gave them — and the
    // design prompt has every scene paint one. Clearing them is what actually
    // leaves the type and the logo on transparent pixels.
    if (transparent) {
        await page.evaluate(() => {
            for (const el of [document.documentElement, document.body]) {
                el.style.background = "transparent";
                el.style.backgroundColor = "transparent";
            }
            document.querySelectorAll("[data-slide]").forEach((n) => {
                (n as HTMLElement).style.background = "transparent";
                (n as HTMLElement).style.backgroundColor = "transparent";
            });
        });
    }

    // Strip editor chrome and the preview's display transform BEFORE measuring,
    // so the boxes we read are the design's real layout, not a scaled-down one.
    await page.evaluate(prepareForCapture);
    await page.evaluate(waitForPaint, config.imageTimeoutMs);

    const measurement = await measure(page);
    const rects = await page.evaluate(slideRects);

    return {
        page,
        transparent,
        measurement,
        rects,
        close: async () => {
            await context.close().catch(() => undefined);
            await new Promise<void>((done) => server.close(() => done()));
        },
    };
}
