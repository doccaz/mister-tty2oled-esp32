// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// Backup/restore of the browser-local art library (the IndexedDB `coreArt`
// store). The library only lives in one browser's storage - clearing site
// data wipes it, and it doesn't follow you to another machine - so this
// gives it a portable, human-checkable file: one JSON document with each
// core's exported JPEG as base64.

import { loadAllCoreArt, putCoreArtRecords, type CoreArtRecord } from "./db";

const FORMAT = "tty2oled-gc9a01-library";
const VERSION = 1;
const MAX_BACKUP_BYTES = 64 * 1024 * 1024; // sanity cap before parsing

interface BackupItem {
  coreId: string;
  updatedAt: number;
  type: string; // image/jpeg (editor exports) or image/png (auto-seeded from a local pack)
  data: string; // base64
}

interface BackupFile {
  format: typeof FORMAT;
  version: number;
  exportedAt: string;
  items: BackupItem[];
}

export interface RestoreResult {
  restored: CoreArtRecord[]; // records actually written
  skippedOlder: number; // already have the same or a newer version
  invalid: number; // entries that weren't usable and were ignored
}

async function blobToBase64(blob: Blob): Promise<string> {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let bin = "";
  const CHUNK = 0x8000;
  for (let i = 0; i < bytes.length; i += CHUNK) {
    bin += String.fromCharCode(...bytes.subarray(i, i + CHUNK));
  }
  return btoa(bin);
}

function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

/** Sniffs the real image type from magic bytes; null if it's neither JPEG nor PNG. */
function sniffImageType(b: Uint8Array): string | null {
  if (b.length >= 3 && b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return "image/jpeg";
  if (b.length >= 4 && b[0] === 0x89 && b[1] === 0x50 && b[2] === 0x4e && b[3] === 0x47) return "image/png";
  return null;
}

/** Serializes every saved core's art into a downloadable JSON blob. */
export async function exportLibrary(): Promise<{ blob: Blob; count: number }> {
  const all = await loadAllCoreArt();
  const items: BackupItem[] = [];
  for (const rec of all.values()) {
    items.push({
      coreId: rec.coreId,
      updatedAt: rec.updatedAt,
      type: rec.jpegBlob.type || "image/jpeg",
      data: await blobToBase64(rec.jpegBlob),
    });
  }
  items.sort((a, b) => a.coreId.localeCompare(b.coreId));
  const file: BackupFile = { format: FORMAT, version: VERSION, exportedAt: new Date().toISOString(), items };
  return { blob: new Blob([JSON.stringify(file)], { type: "application/json" }), count: items.length };
}

/**
 * Merges a backup file into the library. A core that's already saved is
 * only replaced when the backup's copy is strictly newer, so restoring an
 * old backup can't clobber work done since. Throws on a file that isn't a
 * backup at all; individual bad entries are counted and skipped.
 */
export async function restoreLibrary(file: File): Promise<RestoreResult> {
  if (file.size > MAX_BACKUP_BYTES) throw new Error("That file is too large to be a library backup.");
  let doc: unknown;
  try {
    doc = JSON.parse(await file.text());
  } catch {
    throw new Error("That file isn't valid JSON.");
  }
  const d = doc as Partial<BackupFile> | null;
  if (!d || d.format !== FORMAT || !Array.isArray(d.items)) {
    throw new Error("That file isn't a tty2oled-gc9a01 library backup.");
  }
  if (typeof d.version !== "number" || d.version > VERSION) {
    throw new Error(`Backup version ${String(d.version)} is newer than this app understands.`);
  }

  const existing = await loadAllCoreArt();
  const restored: CoreArtRecord[] = [];
  let skippedOlder = 0;
  let invalid = 0;

  for (const item of d.items as Partial<BackupItem>[]) {
    if (!item || typeof item.coreId !== "string" || !item.coreId || typeof item.data !== "string") {
      invalid++;
      continue;
    }
    let bytes: Uint8Array;
    try {
      bytes = base64ToBytes(item.data);
    } catch {
      invalid++;
      continue;
    }
    const type = sniffImageType(bytes);
    if (!type) {
      invalid++; // neither JPEG nor PNG
      continue;
    }
    const updatedAt = typeof item.updatedAt === "number" && Number.isFinite(item.updatedAt) ? item.updatedAt : Date.now();
    const have = existing.get(item.coreId);
    if (have && have.updatedAt >= updatedAt) {
      skippedOlder++;
      continue;
    }
    restored.push({ coreId: item.coreId, jpegBlob: new Blob([bytes as BlobPart], { type }), updatedAt });
  }

  await putCoreArtRecords(restored);
  return { restored, skippedOlder, invalid };
}
