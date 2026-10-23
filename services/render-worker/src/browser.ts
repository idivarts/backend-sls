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

/**
 * Lambda only. Fargate runs the multi-process browser happily — it renders
 * video all day — so these are NOT applied there, where the extra isolation is
 * worth having and costs nothing.
 *
 * On Lambda the browser launched fine and then died creating a page:
 *     browserContext.newPage: Target crashed
 * which is the renderer process failing to spawn, not the browser failing to
 * start. --no-zygote is the one that matters: the zygote Chromium forks
 * renderers from relies on IPC that Lambda's sandbox does not reliably allow.
 *
 * Deliberately NOT --single-process. It is widely repeated as mandatory here,
 * but it is also a documented source of crashes of its own, and it would be a
 * much bigger behavioural change than the problem warrants.
 */
const LAMBDA_ARGS = [
    "--no-zygote",
    "--disable-setuid-sandbox",
    "--disable-gpu",
];

let browser: Browser | null = null;

export async function getBrowser(): Promise<Browser> {
    if (browser && browser.isConnected()) return browser;

    const onLambda = !!process.env.AWS_LAMBDA_FUNCTION_NAME;

    // Lambda mounts everything read-only except /tmp, and Chromium writes under
    // $HOME as it starts (.config, .cache, .pki). Defensive rather than the
    // known cause — launch itself was succeeding — but the constraint is real.
    if (onLambda) process.env.HOME = "/tmp";

    browser = await chromium.launch({
        headless: true,
        args: onLambda ? [...ARGS, ...LAMBDA_ARGS] : ARGS,
    });

    // A crashed renderer otherwise leaves no trace but a Playwright assertion
    // thrown from whatever call happened to be in flight.
    browser.on("disconnected", () => {
        console.error("[render] chromium disconnected");
        browser = null;
    });
    return browser;
}

export async function closeBrowser(): Promise<void> {
    if (browser) {
        await browser.close().catch(() => undefined);
        browser = null;
    }
}
