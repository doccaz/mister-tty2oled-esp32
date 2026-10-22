// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// Browsable local copy of the reference project's community marquee packs
// (converted from the original .gsc/4bpp-grayscale and .xbm/1bpp text
// format to PNG - see the library/ folder and the conversion notes in this
// project's CLAUDE.md). Served from
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
// Which art "set" the user prefers. "color" prefers a full-color render but
// falls back to grayscale so every matchable core still has art; "gray" uses
// the original grayscale packs only (ignores color).
export type ArtSource = "color" | "gray";

// Higher rank wins; <0 excludes the entry entirely for that source.
function rank(kind: ImportEntry["kind"], source: ArtSource): number {
  if (source === "gray") return kind === "color" ? -1 : kind === "gsc" ? 2 : 1;
  return kind === "color" ? 3 : kind === "gsc" ? 2 : 1; // color: prefer color, then gsc, then xbm
}

export function findAutoMatch(
  entries: ImportEntry[],
  coreId: string,
  source: ArtSource = "color",
): ImportEntry | undefined {
  const pick = (name: string): ImportEntry | undefined => {
    let best: ImportEntry | undefined;
    let bestRank = 0; // entries start at rank >=1, so 0 means "nothing yet"
    for (const e of entries) {
      if (e.name.toLowerCase() !== name) continue;
      const r = rank(e.kind, source);
      if (r > bestRank) {
        best = e;
        bestRank = r;
      }
    }
    return best;
  };

  const exact = pick(coreId.toLowerCase());
  if (exact) return exact;

  for (const alias of CORE_ALIASES[coreId] ?? []) {
    const hit = pick(alias.toLowerCase());
    if (hit) return hit;
  }
  return undefined;
}
