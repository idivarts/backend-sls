/**
 * Editing an authored film's HTML without rewriting it.
 *
 * ── Why a parser, and why not its serializer ──────────────────────────────
 * A regex over HTML works for a month and then quietly corrupts someone's
 * design — it cannot tell an attribute from the same characters inside a
 * <script>, and these documents carry a GSAP timeline full of quotes and
 * angle brackets. So the document is parsed properly.
 *
 * But it is NOT serialized back. parse5's serializer rebuilds the document
 * from the tree, which normalises quoting, self-closing tags and whitespace
 * — a diff across the whole film for a one-attribute change, and a diff is
 * exactly what we must not produce in a file the user may be editing by hand
 * later. Instead the parse is used only to LOCATE things, and the edits are
 * spliced into the original string by byte offset.
 *
 * That is also the shape Phase 13's inspector needs: an element addressed
 * precisely, changed in place, everything around it untouched.
 */
import { parse, type DefaultTreeAdapterMap } from "parse5";

type Element = DefaultTreeAdapterMap["element"];
type Node = DefaultTreeAdapterMap["node"];

/** One located element, with everything needed to change it. */
export interface FoundElement {
    tagName: string;
    attrs: Record<string, string>;
    /** Byte offsets of the whole element, including its children. */
    start: number;
    end: number;
    /** Byte offsets of the start tag alone. */
    tagStart: number;
    tagEnd: number;
    /** Byte offsets of each attribute, for replacing one in place. */
    attrSpans: Record<string, { start: number; end: number }>;
}

/** A replacement of one span of the source. */
export interface Edit {
    start: number;
    end: number;
    text: string;
}

function isElement(n: Node): n is Element {
    return typeof (n as Element).tagName === "string";
}

/**
 * Every element the predicate accepts, in document order.
 */
export function findElements(html: string, accept: (el: FoundElement) => boolean): FoundElement[] {
    const doc = parse(html, { sourceCodeLocationInfo: true });
    const out: FoundElement[] = [];

    const walk = (node: Node): void => {
        if (isElement(node)) {
            const loc = node.sourceCodeLocation;
            // An element parse5 synthesised (an implied <tbody>, a <head>
            // the document never wrote) has no location. It is not in the
            // source, so it cannot be edited there.
            if (loc && loc.startTag) {
                const attrs: Record<string, string> = {};
                for (const a of node.attrs) attrs[a.name] = a.value;
                const attrSpans: Record<string, { start: number; end: number }> = {};
                for (const [name, l] of Object.entries(loc.attrs ?? {})) {
                    attrSpans[name] = { start: l.startOffset, end: l.endOffset };
                }
                const found: FoundElement = {
                    tagName: node.tagName,
                    attrs,
                    start: loc.startOffset,
                    end: loc.endOffset,
                    tagStart: loc.startTag.startOffset,
                    tagEnd: loc.startTag.endOffset,
                    attrSpans,
                };
                if (accept(found)) out.push(found);
            }
        }
        for (const child of (node as Element).childNodes ?? []) walk(child);
    };

    walk(doc as unknown as Node);
    return out;
}

/**
 * Apply edits to the source.
 *
 * Applied from the END backwards, so that each splice leaves every earlier
 * offset still valid. Overlapping edits are refused rather than silently
 * producing a document that is neither one change nor the other.
 */
export function applyEdits(html: string, edits: Edit[]): string {
    const sorted = [...edits].sort((a, b) => b.start - a.start);
    let last = Number.POSITIVE_INFINITY;
    let out = html;
    for (const e of sorted) {
        if (e.end > last) {
            throw new Error(`overlapping edits at ${e.start}-${e.end}`);
        }
        if (e.start < 0 || e.end > out.length || e.start > e.end) {
            throw new Error(`edit out of range: ${e.start}-${e.end}`);
        }
        out = out.slice(0, e.start) + e.text + out.slice(e.end);
        last = e.start;
    }
    return out;
}

/** The quoted form of one attribute, with the value escaped for a value position. */
export function attr(name: string, value: string): string {
    return `${name}="${value.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;")}"`;
}

/**
 * An edit that sets an attribute on an element — replacing it where it is
 * already there, and otherwise inserting it just inside the start tag so the
 * rest of the tag keeps its original spelling.
 */
export function setAttribute(el: FoundElement, name: string, value: string): Edit {
    const span = el.attrSpans[name];
    if (span) return { start: span.start, end: span.end, text: attr(name, value) };
    // After "<tag", which is 1 + the tag name. Inserting here rather than
    // before ">" keeps it clear of a self-closing slash.
    const at = el.tagStart + 1 + el.tagName.length;
    return { start: at, end: at, text: " " + attr(name, value) };
}

/** An edit that removes an attribute, leaving the tag otherwise untouched. */
export function removeAttribute(el: FoundElement, name: string, html: string): Edit | null {
    const span = el.attrSpans[name];
    if (!span) return null;
    // Take the whitespace in front of it too, or the tag collects double
    // spaces every time something is resolved.
    let start = span.start;
    while (start > el.tagStart && /\s/.test(html[start - 1])) start--;
    return { start, end: span.end, text: "" };
}
