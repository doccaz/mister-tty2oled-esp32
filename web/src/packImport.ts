// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// "Import folder": turns files the user picks from their own disk into
// ImportEntry rows, entirely in the browser - nothing is uploaded and
// nothing is hosted. Understands the original tty2oled `.gsc` (4bpp
// grayscale) / `.xbm` (1bpp) text files, decoded exactly like
// tools/convert_library.py does for the local pack build, plus ordinary
// PNG/JPEG/WebP/GIF images. This is what lets the hosted copy of the app
// (which ships without the third-party art packs) use a pack you already
// have, with no Python/Pillow build step.

import type { ImportEntry } from "./importLibrary";

const W = 256;
const H = 64;
const MAX_FILES = 5000;
const MAX_TEXT_BYTES = 1024 * 1024; // .gsc is ~42KB of text; anything near 1MB isn't one
const MAX_IMAGE_BYTES = 20 * 1024 * 1024;
const IMAGE_EXTS = new Set(["png", "jpg", "jpeg", "webp", "gif"]);

export interface FolderImportResult {
  entries: ImportEntry[];
  gscXbm: number;
  images: number;
  skipped: { name: string; reason: string }[];
  ignored: number; // files that aren't art at all (readme, .ino, ...)
}

const HEX_RE = /0[xX]([0-9a-fA-F]{2})/g;
const COMMENT_RE = /\/\*[\s\S]*?\*\//g;

function extOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i < 0 ? "" : name.slice(i + 1).toLowerCase();
}

function stemOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i < 0 ? name : name.slice(0, i);
}

/** Pulls the hex byte literals out of a C-array text file (see convert_library.py's parse_bytes). */
function parseHexBytes(text: string): Uint8Array {
  const clean = text.replace(COMMENT_RE, ""); // some headers embed a commented-out byte preview
  const out: number[] = [];
  for (const m of clean.matchAll(HEX_RE)) out.push(parseInt(m[1], 16));
  return Uint8Array.from(out);
}

/** 256x64 RGBA pixels from a 1bpp (XBM, LSB first) or 4bpp (GSC, high nibble first) payload. */
function decodeToImageData(buf: Uint8Array, kind: "gsc" | "xbm"): ImageData {
  const img = new ImageData(W, H);
  const px = img.data;
  const set = (x: number, y: number, v: number) => {
    const o = (y * W + x) * 4;
    px[o] = px[o + 1] = px[o + 2] = v;
    px[o + 3] = 255;
  };
  if (kind === "xbm") {
    const lineBytes = W / 8;
    for (let y = 0; y < H; y++) {
      for (let xb = 0; xb < lineBytes; xb++) {
        const byte = buf[xb + y * lineBytes];
        for (let bit = 0; bit < 8; bit++) set(xb * 8 + bit, y, (byte >> bit) & 1 ? 255 : 0);
      }
    }
  } else {
    const lineBytes = W / 2;
    for (let y = 0; y < H; y++) {
      for (let xb = 0; xb < lineBytes; xb++) {
        const byte = buf[xb + y * lineBytes];
        set(xb * 2, y, ((byte >> 4) & 0xf) * 17);
        set(xb * 2 + 1, y, (byte & 0xf) * 17);
      }
    }
  }
  return img;
}

/**
 * Decodes one .gsc/.xbm file to a PNG blob. Throws a human-readable reason
 * if it isn't a usable 256x64 payload. A minority of files declare
 * `_bits[8198]`/`[2054]`: 6 extra leading metadata bytes before the real
 * 8192/2048-byte payload, which are dropped here as in the Python converter.
 */
export async function decodeLegacyFile(file: File, canvas: HTMLCanvasElement): Promise<{ png: Blob; kind: "gsc" | "xbm" }> {
  let buf = parseHexBytes(await file.text());
  if (buf.length === 8192 + 6 || buf.length === 2048 + 6) buf = buf.subarray(6);
  let kind: "gsc" | "xbm";
  if (buf.length === 8192) kind = "gsc";
  else if (buf.length === 2048) kind = "xbm";
  else throw new Error(`unexpected size ${buf.length} bytes (expected 8192 or 2048)`);

  canvas.width = W;
  canvas.height = H;
  canvas.getContext("2d")!.putImageData(decodeToImageData(buf, kind), 0, 0);
  const png = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"));
  if (!png) throw new Error("couldn't encode PNG");
  return { png, kind };
}

/** Parent directory of a picked file ("pics/GSC"), or a generic label when none is known. */
function packOf(file: File): string {
  const rel = (file as File & { webkitRelativePath?: string }).webkitRelativePath || "";
  const i = rel.lastIndexOf("/");
  return i > 0 ? rel.slice(0, i) : "imported";
}

/**
 * Turns a folder's worth of picked files into pack entries. Each entry's
 * `url` is an object URL into memory (valid until the page closes).
 * `onProgress` is called every so often so a big folder doesn't look hung.
 */
export async function importFolder(files: File[], onProgress?: (done: number, total: number) => void): Promise<FolderImportResult> {
  const res: FolderImportResult = { entries: [], gscXbm: 0, images: 0, skipped: [], ignored: 0 };
  const canvas = document.createElement("canvas");
  const picked = files.slice(0, MAX_FILES);
  if (files.length > MAX_FILES) res.skipped.push({ name: `(${files.length - MAX_FILES} more files)`, reason: `only the first ${MAX_FILES} files are read` });

  let done = 0;
  for (const file of picked) {
    const ext = extOf(file.name);
    try {
      if (ext === "gsc" || ext === "xbm") {
        if (file.size > MAX_TEXT_BYTES) throw new Error("too large to be a marquee file");
        const { png, kind } = await decodeLegacyFile(file, canvas);
        res.entries.push({ pack: packOf(file), name: stemOf(file.name), kind, png: "", url: URL.createObjectURL(png) });
        res.gscXbm++;
      } else if (IMAGE_EXTS.has(ext)) {
        if (file.size > MAX_IMAGE_BYTES) throw new Error("image is larger than 20 MB");
        res.entries.push({ pack: packOf(file), name: stemOf(file.name), kind: "color", png: "", url: URL.createObjectURL(file) });
        res.images++;
      } else {
        res.ignored++;
      }
    } catch (err) {
      res.skipped.push({ name: file.name, reason: err instanceof Error ? err.message : String(err) });
    }
    if (++done % 50 === 0) {
      onProgress?.(done, picked.length);
      await new Promise((r) => setTimeout(r, 0)); // let the UI repaint
    }
  }
  onProgress?.(picked.length, picked.length);
  return res;
}
