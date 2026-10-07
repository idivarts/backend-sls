/**
 * The browser the renders happen in.
 *
 * One Chromium per process, reused across every job that process handles: a
 * launch costs a second or two, and on Batch one task may drain several jobs.
 *
 * The flags are all about DETERMINISM — the same HTML must produce the same
 * pixels on every machine, every run. Subpixel antialiasing and font hinting
 * are the two that actually bite: left on, the same text renders differently
 * depending on the host, which turns a golden-frame test into a coin flip.
 */
import { chromium, Browser } from "playwright";

const ARGS = [
    // Containers get a 64 MB /dev/shm; without this Chromium crashes on any
    // non-trivial page. Not optional, not tunable.
    "--disable-dev-shm-usage",
    "--no-sandbox",
    // Determinism: no hinting, no subpixel AA, one colour profile.
    "--font-render-hinting=none",
    "--disable-lcd-text",
    "--force-color-profile=srgb",
    "--hide-scrollbars",
    // A render is a background tab by definition. Without these, Chromium
    // throttles timers and rAF, which stalls animation seeking.
    "--disable-background-timer-throttling",
    "--disable-renderer-backgrounding",
    "--disable-backgrounding-occluded-windows",
    "--autoplay-policy=no-user-gesture-required",
];

let browser: Browser | null = null;

export async function getBrowser(): Promise<Browser> {
    if (browser && browser.isConnected()) return browser;
    browser = await chromium.launch({ headless: true, args: ARGS });
    return browser;
}

export async function closeBrowser(): Promise<void> {
    if (browser) {
        await browser.close().catch(() => undefined);
        browser = null;
    }
}
