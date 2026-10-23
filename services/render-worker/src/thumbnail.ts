/**
 * Make a small WebP copy of a rendered slide.
 *
 * A rendered slide is a 1080px PNG, and the surfaces that show it most — the
 * slide strip, the content calendar — draw it at roughly 128px. Shipping
 * megabytes to paint a thumbnail is the most wasteful thing the app does with
 * rendered media, and it is wasteful on exactly the screens people scroll.
 *
 * Best-effort by design: a thumbnail is an optimisation, so a failure here
 * returns null and the caller falls back to the full render rather than failing
 * a job the user is waiting on.
 */
import sharp from "sharp";

/** Wide enough for a 2x-density 128-200px tile without being a second render. */
const THUMB_WIDTH = 480;

export async function makeThumbnail(png: Buffer): Promise<Buffer | null> {
    try {
        return await sharp(png)
            .resize({ width: THUMB_WIDTH, withoutEnlargement: true })
            .webp({ quality: 78 })
            .toBuffer();
    } catch (err) {
        console.warn("[render] thumbnail failed, falling back to the full render:", err);
        return null;
    }
}
