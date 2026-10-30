# web/ notes (moved from the root CLAUDE.md)

## Web app

```bash
cd web
npm install
npm run dev                 # open in Chrome or Edge (WebSerial required)
```

Select a core, drop in an image, pan/zoom to fit the circular safe area,
"Send to device" (or just edit — live preview streams automatically while
connected), "Save to library" persists it locally (IndexedDB) for next time.

### WiFi transport from the https copy (2026-10-14)

`WifiLink` opens `ws://<host>:81`. From the github.io (https) page that
throws a synchronous `SecurityError` (mixed content) from `new WebSocket()`,
which used to leave the state stuck on "connecting" with the Connect button
disabled and a cryptic message. `connect()` now catches it, resets to
"disconnected" and rejects with `MixedContentError`; `main.ts` shows a notice
(use USB, or open the app over http: `npm run dev` / any static server on
localhost). `ws.onerror` also resets the state. Real fix would be wss on the
device, which doesn't fit in RAM (see the WiFi feasibility numbers above).

### Import folder (2026-10-13)

Header "Import folder" (`web/src/packImport.ts`): `<input type=file
webkitdirectory>` -> `.gsc`/`.xbm` decoded in the browser (a port of
`tools/convert_library.py`: strip `/* */` comments, collect `0x..` literals,
drop 6 leading metadata bytes when the count is 8198/2054, 8192 = 4bpp GSC,
2048 = 1bpp XBM, 256x64) plus PNG/JPEG/WebP/GIF, into `ImportEntry` rows with
an in-memory object `url` (`entryUrl()` prefers it). They're appended to
`importIndex`, the core catalog is rebuilt (`buildAllCores()`), and the
Review-matches modal opens with a summary. Nothing is uploaded or hosted, so
it doesn't conflict with the art packs not being redistributed; it's how a
user brings their own pack to the hosted app. Verified in a real browser on
the hosted-style build (no library): MENU.gsc (the `_bits[8198]` variant),
two `.xbm` and TGFX16.gsc decoded to pixels identical to the Python
converter's PNGs (SHA-256 of the grayscale bytes), a 3-byte `.xbm` was
skipped with a reason, a README ignored. Only the first 5000 files are read;
`.zip`/`.rar` archives are not opened (extract first). **Large folders (found by importing the real 1890-file `GSC` folder):** the
first version crashed the page - everything rendered at once: ~1900 library
cards plus a ~1900-row review modal. Reproduced in a (background) Chrome tab
and narrowed with a heartbeat beacon (`navigator.sendBeacon` keeps working
when the page's main thread is blocked): decoding is fine (~6 s, linear),
the freeze comes from the rendering, and was *not* in my JS (heap 16 MB, no
long-animation-frame ever finished). What fixed it: `renderLibrary()` draws
at most 300 cards per category with a "Show more" button plus a filter box
(`LIBRARY_PAGE`), and the review modal adds rows 400 at a time with its
checked state in a `Map` (so Save covers un-rendered rows). Also changed on
the way, each independently sensible: `.review-list` is no longer a column
flexbox (a synthetic 1894-row flex list froze the page; the same rows in a
block container didn't), `db.ts` shares one IndexedDB connection (it used to
open and never close one per save) and the review Save writes everything in
one transaction (`putCoreArtRecords`), and `withSavedCores()` adds any saved
core missing from the catalog so cores saved from an imported folder still
show after a reload on the hosted copy (no pack index there). Names from the
folder are HTML-escaped before going into `innerHTML`. Verified: full import
+ Save of 1889 cores, page responsive throughout (heartbeat max gap 1 s),
all 1889 records persisted and listed after a reload. NOT verified: the exact
browser-internal cause (I never isolated a minimal case; paging fixed it) and
any of this in a visible, focused tab (the test tab was backgrounded, where
Chrome throttles `canvas.toBlob` to ~1 s per call - the test harness
overrides it).

Also fixed here:
`buildAllCores()` produced duplicate core ids when one name existed as both
`.gsc` and `.xbm`, and the pack browser's card click never called
`updateActionButtons()`, leaving Save/Preview/Send disabled for a core with
no saved art.

### Library backup/restore (2026-10-13)

`web/src/backup.ts` + the header's "Back up"/"Restore" buttons. The only
persisted state is IndexedDB's `coreArt` store (`db.ts`), so the backup is
one JSON file `{format:"tty2oled-gc9a01-library", version:1, items:[{coreId,
updatedAt, type, data(base64)}]}`. Restore merges: a core is only
overwritten when the backup's `updatedAt` is strictly newer, so restoring an
old file can't clobber later work; `putCoreArtRecords()` keeps the original
timestamps. **Saved blobs are not all JPEG** - the editor exports JPEG, but
`autoSeedLibrary()` stores the fetched pack PNGs as-is, so restore sniffs
magic bytes for JPEG *or* PNG and keeps each blob's type (a first version
that only accepted JPEG rejected all 443 auto-seeded records; found by a
real-browser round-trip test: delete everything, restore, compare - 444
records byte-identical, second restore a no-op). Matters most on the hosted
github.io copy, where the library lives only in that browser's storage.

### Command console

The header's "Command console" button opens a modal (`openCommandConsole()`
in `main.ts`) that documents and exercises every wire-protocol command the
firmware understands - both the original tty2oled grammar and the new
commands added for this round display. `web/src/commands.ts` is the
catalog (kept in sync with `firmware/src/protocol.cpp`'s `dispatch()` and
this file's "Wire protocol" section) - one `CommandDef` per command, with
its syntax, a plain-English summary, and a typed param list the modal
turns into input fields. Picture-transfer commands (`CMDCOR`/`CMDAPD`/
`CMDCORC`) are listed as `sendable: false` with a note pointing at the
main gallery/editor instead, since they need raw binary payloads a
generic command-line console has no business constructing. `serial.ts`'s
`SerialLink` gained a `"message"` event (any device output that isn't
`ttyack;`/`ttyrdy;`, e.g. `CMDHWINF`'s reply) so the console's log panel
can show raw device responses, not just sent commands.

### About modal + GitHub ribbon

The header's "About" button opens a short write-up of what the project
is, its architecture, and links (with full URLs shown) to this repo, the
original `venice1200/MiSTer_tty2oled`, the WLED OLED usermod the status
display was ported from, and the sibling `tty2tft` project. A CSS-only
diagonal "Fork me on GitHub" ribbon sits fixed in the page's bottom-right
corner (`.github-ribbon` in `style.css`) - built from scratch rather than
reusing the common SVG-icon ribbon snippet, to avoid pulling in art of
uncertain provenance.

### "Blank first" checkbox (2026-08-13)

The editor panel's "Send to device" row has a "blank first" checkbox
(`#blank-first-checkbox`, checked by default) - when checked, an explicit
"Send to device" click sends `CMDCLS` before the picture. Exists because
`transitionReveal()` on the firmware only ever writes new pixels over
whatever's already on the physical screen (never clears first,
deliberately matching the original tty2oled's own behavior - see
`CMDSPIC` above) - so re-sending pixel-identical content with only the
effect changed shows no visible transition at all, which is confusing
when using this button specifically to evaluate different effects.
Deliberately scoped to explicit sends only, not `scheduleLivePreview()`'s
debounced auto-send during pan/zoom - clearing on every drag update would
flash distractingly. Root-caused on real hardware 2026-08-13: three
back-to-back `CMDCORC` sends of the *same* JPEG with different effects
showed a transition only on the first (screen already matched); the same
sends with three *different* JPEGs showed all three transitions
correctly, confirming the framebuffer-reveal design was working exactly
as intended, not a bug.
