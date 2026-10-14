// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// Browsable local copy of the reference project's community marquee packs
// (converted from the original .gsc/4bpp-grayscale and .xbm/1bpp text
// format to PNG - see /home/erico/Projetos/mister-tty2oled-gc9a01/library/
// and the conversion notes in this project's CLAUDE.md). Served from
// web/public/library so it's a plain static fetch, no bundler import of
// 500+ files.

import { CORE_ALIASES } from "./aliases";

export interface ImportEntry {
  pack: string;
  name: string;
  kind: "gsc" | "xbm" | "color";
  png: string; // relative path under /library/ (empty for entries imported from a user folder)
  url?: string; // object URL for entries imported in-browser from a user folder (see packImport.ts)
}

let cache: ImportEntry[] | null = null;
let packLibraryMissing = false;

export const PACK_LIBRARY_MISSING_MESSAGE =
  "The community art pack isn't included in this copy of the app (those fan-made packs aren't redistributed). " +
  "Use \"Import folder\" to load a folder of your own (.gsc, .xbm, PNG or JPEG), or import a single image per core; " +
  "to browse the bundled packs, run the app locally with the library built (see the README).";

/** Called once the user has imported their own folder: there is a pack to browse after all. */
export function markPackLibraryAvailable(): void {
  packLibraryMissing = false;
}

/** True once a load found no pack index at all (e.g. the hosted copy), as opposed to a transient failure. */
export function isPackLibraryMissing(): boolean {
  return packLibraryMissing;
}

export async function loadImportIndex(): Promise<ImportEntry[]> {
  if (cache) return cache;
  const res = await fetch(`${import.meta.env.BASE_URL}library/index.json`);
  if (res.status === 404) {
    packLibraryMissing = true;
    throw new Error(PACK_LIBRARY_MISSING_MESSAGE);
  }
  if (!res.ok) throw new Error(`Failed to load library/index.json: ${res.status}`);
  cache = (await res.json()) as ImportEntry[];
  return cache;
}

export function entryUrl(entry: ImportEntry): string {
  if (entry.url) return entry.url;
  return `${import.meta.env.BASE_URL}library/${entry.png}`;
}

/**
 * Finds a local pack entry matching a core id: first an exact filename
 * match (case-insensitive - "NES", "SNES", "C64", ...), then any of that
 * core's known alternate spellings (aliases.ts - "GAMEBOY2P" for
 * "Gameboy", "GBA2P" for "GBA", etc). Lets the library grid/editor
 * auto-fill without requiring the user to manually browse+assign every
 * core that already has an obvious match.
 */
export function findAutoMatch(entries: ImportEntry[], coreId: string): ImportEntry | undefined {
  const target = coreId.toLowerCase();
  const exact = entries.find((e) => e.name.toLowerCase() === target);
  if (exact) return exact;

  const aliases = CORE_ALIASES[coreId] ?? [];
  for (const alias of aliases) {
    const aliasTarget = alias.toLowerCase();
    const hit = entries.find((e) => e.name.toLowerCase() === aliasTarget);
    if (hit) return hit;
  }
  return undefined;
}
