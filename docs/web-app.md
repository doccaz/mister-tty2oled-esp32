# Marquee web app

## Run the web app

```bash
cd web
npm install
npm run dev                 # open in Chrome or Edge
```

![The web app's marquee editor with a core selected and the device connected over USB](web-ui.png)

Pick a display profile (round GC9A01, or a rectangular preview profile —
see [Display profiles](#display-profiles)), select a core, drop in an
image, pan/zoom to fit, and either let the live preview stream to a
connected device automatically or click "Send to device". "Save to
library" persists the art locally in the browser (IndexedDB) so it's
there next time.

The header's transport selector switches between **USB (WebSerial)** and
**WiFi** — for WiFi, enter the device's hostname (`tty2oled-XXXX.local`)
or IP address, then Connect. Both transports speak the identical command
protocol; the app doesn't care which one is active. The header also has
a **Command console** (documents and lets you exercise every command the
firmware understands) and an **About** panel with links back to this
project and the ones it's built on.

**Import folder** in the header loads art from a folder on your computer
and runs entirely in the browser (nothing is uploaded). It reads the
original tty2oled `.gsc` and `.xbm` files as well as PNG, JPEG, WebP and
GIF, matches names to cores (exact name, then the alternate spellings in
`web/src/aliases.ts`), and opens the "Review core matches" table so you can
check and save them. That's the way to use a pack you already have on the
hosted copy, which ships without the community art packs.
Big folders are fine (a full 1890-file pack imports in a few seconds): the
library shows 300 cores per category at a time with a **Show more** button,
and the filter box above it finds a core by name.

**Back up** and **Restore** in the header save your library (every core's
saved art) to a single JSON file and merge it back in. Browser storage is
per browser and per site, so it's worth backing up before clearing site
data or switching machines (this matters most on the hosted
`github.io` copy). Restoring never overwrites a core you've since edited:
a saved core is only replaced when the backup's copy is newer.

The hosted copy at `github.io` talks to the device over **USB only**: the
WiFi transport uses plain `ws://` (the ESP32 has no RAM to spare for TLS),
which browsers block from an `https://` page. Choosing WiFi there tells you
so instead of failing quietly. For WiFi, open the app over `http`: run it
locally (`npm run dev`, then `http://localhost:5173`) or serve a built copy
with any static server (e.g. `python3 -m http.server` in the app folder).

### Building the local marquee library

The editor's "Browse local packs" button needs a converted image index,
built from the community packs bundled in `reference/Pictures/ZIPs`
(pulled in via the `reference/` submodule):

```bash
git submodule update --init reference
tools/build_library.sh
# -> library/converted/<pack>/<name>.png + index.json
# -> synced into web/public/library/ for the web app to fetch
```

This step is optional and its output isn't tracked in this repo (see
[License](licensing.md#license) — the bundled packs are third-party fan-made assets
of unclear licensing, kept local-only rather than redistributed). Cores
whose id matches a pack filename (or a known alias in
`web/src/aliases.ts`) auto-fill in the library grid once this is run.

## Display profiles

The web app's editor targets a selectable display profile
(`web/src/displays.ts`): the round GC9A01 this firmware drives, the
legacy 256×64 SSD1322 (real original hardware), and a couple of generic
rectangular sizes kept as preview/export targets for a possible future
firmware variant. Only the profiles with matching real firmware get a
live "Send to device" — GC9A01 sends `CMDCORC` JPEGs, the legacy SSD1322
profile sends `CMDCOR` grayscale GSC data; the generic rect profiles are
preview-only since no real hardware exists for them.
