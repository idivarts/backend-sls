/**
 * Upload a finished render and hand back the URL the app will store.
 *
 * The worker writes straight to the bucket with its own IAM role — no presigned
 * round-trip, which is what the browser needed only because it had no
 * credentials. The returned URL is the CloudFront one, matching exactly what
 * `/s3/v1/attachments` hands the app today, so nothing downstream can tell a
 * server render from a client one by its URL shape.
 */
import { PutObjectCommand, S3Client } from "@aws-sdk/client-s3";
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
