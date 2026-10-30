# Planned: full-color marquee support (moved from the root CLAUDE.md)

## Planned: full-color marquee support (deferred)

Current priority is **hardware bring-up** (firmware flashed + verified on
the real ESP32-C3/GC9A01, driven end-to-end by the web app's gallery).
Full-color art sourcing is explicitly **deferred** until that's working -
tracked here so it isn't lost.

Findings from investigating "is there an existing color equivalent of our
grayscale pack" (2026-08-12):

- **tty2tft** (ojaksch/venice1200, same community/author as tty2oled) is
  the closest sibling project — built specifically for color TFT displays.
  Worth borrowing its **conventions**, not its assets:
  - JPG, non-progressive, ~60% quality, ~20–35KB/file target, 320×240 or
    480×320 depending on display variant.
  - Per-game matching by MAME setname, falling back to **progressively
    shortening the name** on a miss (`jackalfixed` → `jackalfixe` → … →
    `jackal`), plus `_alt1`..`_alt5` variants diced at random - a better
    arcade-matching strategy than our current exact/alias-only approach
    in `aliases.ts`/`findAutoMatch`, worth adopting regardless of art
    source.
  - License: CC BY-NC-SA 4.0 (non-commercial, share-alike, attribution
    required). Repo archived/unmaintained since 2025-04.
- **No bundled/downloadable color art pack exists** for tty2tft - despite
  `PICTURE_REPOSITORY_URL=https://www.tty2tft.de` appearing in its ini,
  that's for optional WiFi *streaming* from a self-hosted server, not a
  distributable archive (and the site 403s automated fetches). Users are
  expected to source their own JPGs and organize them into its SD-card
  folder convention by hand.
- Broader (non-MiSTer-specific) color marquee sources exist if we go
  looking outside this community: Progetto SNAPS, LaunchBox/EmuMovies
  "MAME Marquees" packs, Pixelcade's remastered marquee library. Each
  would need its own size/license/format check before importing - none
  of this has been vetted yet.

When this work resumes: reuse tty2tft's fallback-matching pattern in the
web app's arcade auto-match, and revisit which external source (if any)
to pull real color art from - that choice was explicitly not made yet.

### Follow-up research (2026-08-13): still no public art catalog, still unresolved

Re-checked with the WiFi/WS work now in place, since a device-side pull
feature would only be worth it if a real source existed to point it at.
Confirmed via the actual tty2tft source (`Code/MiSTer_tty2tft.ino`,
GitHub) and forum research, not just recollection:

- tty2tft is small/niche by GitHub metrics (13 stars, 3 forks, archived
  since 2025-04-23) but had real community discussion (9+ forum pages) -
  GitHub stars undersell MiSTer-community project usage generally.
- Its actual mechanism: config from a plain-text `wifi.txt` on the SD
  card (SSID/pass/base URL/timeout/country, one per line); on `CMDCOR`,
  it checks the SD card first (same shortened-name/`_alt` matching as
  above), and only falls back to an `HTTPClient` GET
  (`baseURL + folderjpg + dirletter + filename`) when nothing local
  matches - fetched images are cached permanently to SD, so it's a
  "local library with lazy-fill fallback," not continuous streaming. A
  separate always-on FTP server (hardcoded `esp32`/`esp32`) handles bulk
  library management. `www.tty2tft.de` (the default repository URL in
  its config) still 403s even a browser-UA `curl` - can't confirm what,
  if anything, it actually serves.
- Still **no real public catalog**: EmuMovies is subscription-gated, MAME
  project marquee art is a legitimate free raw source but not
  tty2tft-packaged, and a community member's "700 new pictures" for
  `tty2rpi` was mentioned in a forum thread with no download link found.
  The devs are explicit that they don't manage/bundle art - users source
  and convert it themselves.

**Design sketch for a device-side "pull on unknown core" feature**
(discussed, not built - contingent on a catalog existing, which it
doesn't yet):
- Trigger: the existing bare-corename fallback branch in
  `protocol_dispatch_line()` (not `CMDCOR`/`CMDAPD`, which are already
  committed to "raw picture bytes follow immediately" by the legacy
  protocol grammar - a stock `tty2oled.sh` only sends those when it
  already has local art to send, so it never naturally emits a "no art,
  please look it up" signal on its own; this would need either a
  modified script - already anticipated as a future step, see "Known
  gaps" below - or serve the web app's own flow rather than a real
  MiSTer).
- Catalog: an ordered list of base URLs in NVS (one delimited
  `Preferences` string), tried in order via `HTTPClient` GET, same
  `colorBuf`/`display_draw_jpeg()` draw path already used for `CMDCORC`.
  No new RAM design needed - the fetch streams into the same static
  buffer.
- **Blocking requirement, not optional polish**: without an on-device
  persistent cache, "a core we don't know" is meaningless - the device
  has zero memory of anything shown across boots/pushes today, so every
  single core selection would pay a network round-trip forever, which is
  a bad experience for something as frequent as MiSTer core-switching.
  LittleFS (flash headroom is fine, ~52% free) is the tractable option -
  no SD card hardware needed - but is real, unbuilt work (custom
  partition table, mount/read/write, a corename→file mapping) that
  should ideally be written through by *both* the push path (`CMDCORC`)
  and the pull path, so either one satisfies future lookups.
- Remaining quality gap even with all the above solved: catalog images
  would arrive full-rectangle with none of `editor.ts`'s round-safe
  crop/fit applied (the device has no such transform, it just centers/
  crops blind) - fine for a catalog curated specifically for this
  round display, ugly for a generic rectangular one.

Net: still blocked on "no catalog exists," same as before - this section
just records that the pull-side design is otherwise sound and what it
would take, so it isn't re-derived from scratch next time this comes up.
