/**
 * Render a design to one PNG per slide.
 *
 * This is the whole image pipeline. There is no clone, no re-layout, no CSS
 * reimplementation: Chromium has already painted the page, and the screenshot
 * is what it painted. Soft shadows, blurs, blend modes, gradient text and real
 * web fonts are correct by default — the "render-safe CSS" list the AI used to
 * be constrained by existed only because the old rasterizer couldn't do them.
 *
 * Slides are captured as ELEMENTS rather than viewport clips. The carousel is a
 * flex strip far wider than the viewport, so every slide after the first sits
 * off-screen; an element screenshot scrolls its own target into view, while a
 * viewport clip would have returned blanks.
 */
import type { Page } from "playwright";

export async function captureSlides(
    page: Page,
    onSlide?: (done: number, total: number) => void
): Promise<Buffer[]> {
    const slides = await page.$$("[data-slide]");
    const targets = slides.length ? slides : [await page.$("body")];
    const out: Buffer[] = [];

    for (let i = 0; i < targets.length; i++) {
        const el = targets[i];
        if (!el) continue;
        out.push(await el.screenshot({ type: "png" }));
        onSlide?.(i + 1, targets.length);
    }
    return out;
}
