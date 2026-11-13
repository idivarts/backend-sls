/**
 * Upload a finished render and hand back the URL the app will store.
 *
 * The worker writes straight to the bucket with its own IAM role — no presigned
 * round-trip, which is what the browser needed only because it had no
 * credentials. The returned URL is the CloudFront one, matching exactly what
 * `/s3/v1/attachments` hands the app today, so nothing downstream can tell a
 * server render from a client one by its URL shape.
 */
import { GetObjectCommand, ListObjectsV2Command, PutObjectCommand, S3Client } from "@aws-sdk/client-s3";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { config } from "./config";

let client: S3Client | null = null;

const s3 = () => (client ??= new S3Client({ region: config.region }));

const stamp = () => `${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;

export async function upload(
    body: Buffer,
    extension: "png" | "mp4" | "jpg" | "webp",
    contentType: string
): Promise<string> {
    const filename = `file_${Math.floor(Date.now() / 1000)}_design_${stamp()}.${extension}`;
    const key = `uploads/${filename}`;
    await s3().send(
        new PutObjectCommand({
            Bucket: config.s3Bucket(),
            Key: key,
            Body: body,
            ContentType: contentType,
            // Renders are immutable: a new render is a new key, so they can be
            // cached hard. This is what makes a published post's media cheap to
            // serve and stops a stale CDN copy ever shadowing a re-render.
            CacheControl: "public, max-age=31536000, immutable",
        })
    );
    return `${config.cdnBase()}/${key}`;
}

/**
 * Root-level objects that are NOT part of the composition.
 *
 * The prefix holds two different things: the authored project, and the player
 * pages generated from it (pkg/hfproject/player.go). Both are HTML at the
 * root, and preview.html is index.html with two script tags — so the CLI
 * discovers two entry points and refuses the project outright:
 *
 *     Multiple root-level HTML files with data-composition-id:
 *     index.html, preview.html. The runtime may discover both as entry
 *     points, causing duplicate audio playback.
 *
 * which broke every preview and render the moment the player shipped. The
 * renderer wants the project alone, so the player pages are left in the
 * bucket and never materialised. Matched at the root only: a scene legitimately
 * called compositions/player.html is a scene.
 */
export const PLAYER_ARTEFACTS = new Set(["preview.html", "player.html"]);

/**
 * Download every object under a prefix into a local directory, preserving the
 * relative layout.
 *
 * This is how a Hyperframes project becomes something the CLI can be pointed
 * at: the project is a DIRECTORY — index.html, compositions/*.html, assets/ —
 * not a document, so it lives in the bucket and is materialised per job.
 *
 * The object count is returned so the caller can refuse to render a project
 * that arrived incomplete. A half-materialised project does not fail: it
 * renders a film with missing scenes, which looks like a design problem and
 * sends whoever debugs it to the wrong place entirely. Skipped player pages
 * are not counted — they were never part of the project.
 */
export async function downloadPrefix(prefix: string, destDir: string): Promise<number> {
    const bucket = config.s3Bucket();
    const normalised = prefix.endsWith("/") ? prefix : `${prefix}/`;
    let token: string | undefined;
    let count = 0;

    do {
        const page = await s3().send(
            new ListObjectsV2Command({ Bucket: bucket, Prefix: normalised, ContinuationToken: token })
        );
        for (const obj of page.Contents ?? []) {
            if (!obj.Key || obj.Key.endsWith("/")) continue;
            const relative = obj.Key.slice(normalised.length);
            if (!relative) continue;
            if (PLAYER_ARTEFACTS.has(relative)) continue;

            // A key is attacker-influenced only as far as whoever wrote the
            // project, but `..` in one would write outside the job directory,
            // so it is refused rather than normalised.
            if (relative.split("/").some((seg) => seg === ".." || seg === "")) {
                throw new Error(`refusing a project key with a traversal segment: ${obj.Key}`);
            }

            const target = join(destDir, relative);
            await mkdir(dirname(target), { recursive: true });
            const got = await s3().send(new GetObjectCommand({ Bucket: bucket, Key: obj.Key }));
            const body = await got.Body?.transformToByteArray();
            if (!body) throw new Error(`empty body for ${obj.Key}`);
            await writeFile(target, Buffer.from(body));
            count++;
        }
        token = page.IsTruncated ? page.NextContinuationToken : undefined;
    } while (token);

    return count;
}

/**
 * Put selected project files back where the project came from.
 *
 * Media resolution happens in the worker, on whichever of the preview or the
 * render runs first, and what it produces is a paid artefact. Leaving it in
 * the job's temp directory would mean the other one resolves it again and
 * pays again, so the changed files go back to the prefix and the next job
 * materialises them like any other part of the project.
 *
 * Paths are project-relative, and a traversal segment is refused for the
 * same reason downloadPrefix refuses one: it would write outside the prefix
 * this design owns.
 */
export async function uploadProjectFiles(prefix: string, dir: string, relativePaths: string[]): Promise<number> {
    const bucket = config.s3Bucket();
    const normalised = prefix.endsWith("/") ? prefix : `${prefix}/`;
    let written = 0;

    for (const rel of new Set(relativePaths)) {
        if (!rel || rel.split("/").some((seg) => seg === ".." || seg === "")) {
            throw new Error(`refusing to upload a path with a traversal segment: ${rel}`);
        }
        let body: Buffer;
        try {
            body = await readFile(join(dir, rel));
        } catch {
            // Reported by the caller as a media problem; a file the CLI said
            // it wrote and did not is not worth failing a render over.
            continue;
        }
        await s3().send(
            new PutObjectCommand({
                Bucket: bucket,
                Key: normalised + rel,
                Body: body,
                ContentType: contentTypeFor(rel),
            })
        );
        written++;
    }
    return written;
}

/** What CloudFront should serve a project file as. */
function contentTypeFor(rel: string): string {
    const ext = rel.slice(rel.lastIndexOf(".")).toLowerCase();
    switch (ext) {
        case ".html":
            return "text/html; charset=utf-8";
        case ".css":
            return "text/css; charset=utf-8";
        case ".js":
        case ".mjs":
            return "text/javascript; charset=utf-8";
        case ".json":
            return "application/json";
        case ".svg":
            return "image/svg+xml";
        case ".png":
            return "image/png";
        case ".jpg":
        case ".jpeg":
            return "image/jpeg";
        case ".webp":
            return "image/webp";
        case ".gif":
            return "image/gif";
        case ".mp3":
            return "audio/mpeg";
        case ".wav":
            return "audio/wav";
        case ".m4a":
            return "audio/mp4";
        case ".mp4":
            return "video/mp4";
        case ".webm":
            return "video/webm";
        case ".woff2":
            return "font/woff2";
        case ".woff":
            return "font/woff";
        default:
            return "application/octet-stream";
    }
}
