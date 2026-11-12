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
import { mkdir, writeFile } from "node:fs/promises";
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
 * sends whoever debugs it to the wrong place entirely.
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
