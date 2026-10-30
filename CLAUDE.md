# CLAUDE.md — tty2oled-esp32

A protocol-compatible re-implementation of [venice1200/MiSTer_tty2oled](https://github.com/venice1200/MiSTer_tty2oled)
targeting an **ESP32-C3-mini + GC9A01 240×240 round SPI display**, plus a
standalone **web app** for browsing/editing the per-core marquee library and
previewing it live on the physical display over WebSerial.

> **Repo renamed 2026-10-21** from `mister-tty2oled-gc9a01` to
> `doccaz/mister-tty2oled-esp32` (panel-neutral, ahead of possible rectangular-panel
> support — see the `repo-rename-multipanel` memory). The GitHub repo and Pages URL
> are now `-esp32`; the **local working directory was also renamed to
> `mister-tty2oled-esp32`** (done with Claude Code closed, moving `~/.claude/projects/`'s
> path-keyed slug dir alongside it so memory/history weren't orphaned). Deliberately kept
> at their old values because they're decoupled identities, not the repo name: the Downloader
> `db_id` `doccaz_mister_tty2oled_gc9a01`, the backup format `tty2oled-gc9a01-library`,
> the HW id `HWGC9A01C`, and the firmware release-asset prefix `tty2oled-gc9a01-*`
> (panel-accurate).

Design decisions and full protocol research are in
`/home/erico/.claude/plans/prancy-bouncing-manatee.md` (the approved plan this
was built from). Summary below.

## Scope

- **Per-core** marquee library (~100 images: NES, SNES, Arcade, ao486, …),
  matching the original tty2oled's actual design — not per-game.
- **Protocol superset**: same line-based command grammar and `ttyrdy;`/
  `ttyack;` handshake as the original, so an unmodified `tty2oled.sh` on a
  real MiSTer still works (text corename, contrast, rotation, and even
  legacy 2048/8192-byte grayscale `.xbm`/`.gsc` pictures, scaled/letterboxed
  onto the round display). New commands (`CMDCORC`) add full-color,
  round-native JPEG art for the web app.
- **WebSerial** is the only web-app transport in v1 — no WiFi/AP firmware.

## License

GPLv3-or-later (`GPL-3.0-or-later`) as of 2026-10-11, compatible with the upstream
`MiSTer_tty2oled` project; it was MIT before that (older copies stay MIT).
Sole copyright holder, so no one else's consent was needed. Nothing is
copied from upstream's code - the firmware is still a from-scratch
reimplementation of the wire protocol - but if upstream code were ever
copied in, GPLv3 is what makes that legal. See the README's "License"
section for what the grant does not cover (the `reference/` submodule,
TFT_eSPI-derived init data, the third-party art packs, dependencies).

## Local marquee library (imported from the reference project)

`reference/Pictures/ZIPs/` bundles the tty2oled community's marquee packs
(per-core system logos plus several per-game arcade packs, e.g. Cave/Neo
Geo/CPS titles) as `.gsc` (4bpp grayscale) / `.xbm` (1bpp) 256×64 text
files — a C-array-style header followed by hex byte literals (some variants
prefix 6 extra header bytes; see `tools/convert_library.py`'s comment/size
handling). `tools/build_library.sh` extracts the archives and converts
every file to PNG:

```bash
tools/build_library.sh
# -> library/converted/<pack>/<name>.png + index.json
# -> synced into web/public/library/ for the web app
```

The web app's editor has a "Browse local packs" button
(`web/src/importLibrary.ts`) that fetches `/library/index.json`, shows a
searchable thumbnail grid, and loads a selection straight into the Canvas
editor for the currently selected core — from there it's cropped/exported
exactly like any other imported image. These are third-party fan-made
assets from the reference repo, kept local to this project for personal
preview/development use only; they are not redistributed anywhere.

Cores whose id matches a pack filename exactly (case-insensitive - `NES`,
`SNES`, `C64`, …), or matches one of that core's known alternate spellings
in `web/src/aliases.ts` (`CORE_ALIASES` - e.g. pack file `GAMEBOY2P` →
core `Gameboy`, `GBA2P` → `GBA`, `Spectrum` → `ZXSpectrum`), auto-fill in
the library grid and editor at boot (`findAutoMatch` in
`importLibrary.ts`). The alias table is a best-effort guess, not verified
against a live MiSTer - the header's "Review core matches" button opens a
pre-filled, per-row-checkable table of every guessed match (with
already-saved cores unchecked by default so they aren't silently
overwritten) and a "Save checked matches" bulk action
(`openMatchReviewModal` in `main.ts`). Not every core has a guess; those
stay blank until assigned manually.

## Display profile & fit presets

`web/src/displays.ts` defines selectable target displays (`DISPLAY_PROFILES`):
round GC9A01 240×240 — **the default** (2026-08-13; previously the legacy
SSD1322 profile defaulted, to match most existing tty2oled setups, but
the round display is this project's actual, real-hardware-verified
firmware target) — plus a few rectangular options (240×240, 320×240, and
the legacy SSD1322 256×64, matching the reference project's actual
hardware). The header's "display" dropdown is a **global** preview
setting — it reshapes the editor canvas, the library thumbnails, and the
JPEG export resolution, but isn't stored per-core (a core's saved art is a
single blob; switching profiles doesn't keep per-profile versions). Only
the round profile is currently wired to real firmware/hardware — the
rectangular profiles are preview/export targets for a possible future
rect-display firmware variant.

`web/src/transforms.ts` defines fit presets (`cover`, `contain`,
`center-scale`) with a default per display **shape**
(`DEFAULT_TRANSFORM_BY_SHAPE`): round defaults to `contain` (2026-08-13 -
previously `center-scale` at 91%; changed so the whole image is visible,
uncropped, on first load), rect defaults to plain `cover`. Presets are a
one-shot starting point, applied on image load and on profile switch
(`MarqueeEditor.setProfile`/`applyPreset`) - the user can still pan/zoom
manually afterward via the editor's drag/scroll, and re-pick a preset any
time from the "fit" control to reset.

`DISPLAY_PROFILES` (`web/src/displays.ts`) lists round (GC9A01) first in
the dropdown, then the legacy SSD1322 rect profile, then the generic rect
profiles — `DEFAULT_DISPLAY_PROFILE` now matches (round/GC9A01), see
above.

## Wire protocol (firmware ⇄ MiSTer or web app)

115200 baud, line-based ASCII commands terminated by `\n`. Firmware sends
`ttyrdy;` once after boot, then `ttyack;` (no trailing newline) after each
command unless `CMDSTTYACK,0` disables it.

Compatible commands (same behavior as upstream): `CMDCLS`/`cls`, `CMDCLSWU`,
`CMDSORG`/`sorg`, `CMDCON,<0-255>`, `CMDROT,<0|1>`, `CMDTXT,x,y,size,text`,
`CMDGEO,type,x,y,w,h,fill`, `CMDSNAM`, `CMDDOFF`, `CMDDON`, `CMDDUPD`,
`CMDSECD,<ms>`, `CMDSHCD`, `CMDSTTYACK,<0|1>`, `CMDRESET`, and a bare line
with no `CMD` prefix (legacy plain-corename fallback).

Additional compatible commands, added 2026-08-13 and verified on real
hardware (see `protocol.cpp`'s `dispatch()`):

- `CMDBYE`, `CMDTEST`, `CMDSHSYSHW` — cosmetic/diagnostic screens
  (farewell text, concentric-ring test pattern, FW version/chip
  model/free heap). Built entirely from existing `display_draw_text`/
  `draw_circle`-style primitives, not ported bitmap assets — RAM is
  already tight (see "RAM headroom") and the reference's icon/test
  bitmaps are separate-provenance assets we don't have license to reuse.
- `CMDSETTIME,<epoch>` / `CMDSHTIME` — added 2026-10-10. `tty2oled.sh`
  sends `CMDSETTIME` unconditionally at startup (`sendtime()`), with the
  MiSTer's local time as epoch seconds *already shifted by the timezone
  offset*; before this was handled it fell through to the bare-corename
  fallback and drew `CMDSETTIME,...` as the core name. The epoch is
  stored with `settimeofday()` and read back with `gmtime_r()` (no TZ
  handling on our side, since it's already local). `CMDSHTIME` (the
  reference's name) shows `HH:MM` + date via `display_show_clock()`, or
  "Time not set!" if none arrived. Not persisted - no RTC battery, but the
  script re-sends it every start. `toInt()` is 32-bit `long`, so epochs
  past 2038 would overflow. The reference only uses its RTC for the
  Show Time/Date screensaver modes, which our `CMDSAVER` doesn't have, so
  nothing shows the time unless `CMDSHTIME` is sent. Verified on real
  hardware 2026-10-10 (ESP32-2424S012C): a simulated `tty2oled.sh`
  startup/core-change cycle (QWERTZ, CMDCON, CMDROT, CMDSETTIME,
  CMDSAVER, CMDCOR + NES.xbm, CMDSHTIME, CMDSPIC, bare corename) was
  acked on every line and the screens looked correct, including
  "Time not set!" before the time arrives.
- `CMDHWINF` — replies over `Serial` with `HW<id>;<version>;` (no
  trailing newline; `web/src/serial.ts` tokenizes on `;`, not `\n`, same
  as `ttyack;`/`ttyrdy;`). Our hardware id is `HWGC9A01C` — a new value
  outside the reference's enum, confirmed safe because only its
  `installer.sh` (a separate setup tool) keys off this string;
  `tty2oled.sh` itself never reads it. Upstream `installer.sh` now
  recognizes `HWGC9A01C` (venice1200/MiSTer_tty2oled PR #43, merged
  2026-10-13, commit `a6f4fff`): it prints a short notice pointing at
  this project, restarts the daemon if it had stopped it, and exits,
  instead of falling into the update flow, which showed "Updating
  tty2oled" on the device and then reported "Flash progress completed"
  without flashing anything. Only an `installer.sh` case branch changed;
  no other upstream hardware/flash/version logic.
- **Bare lines with control/non-ASCII bytes are ignored** (2026-10-13,
  `protocol_dispatch_line()`'s bare-corename branch). Found on a real
  MiSTer: after every core change a garbage "corename" appeared a second
  or so after the (correctly received) logo. Root-caused with a temporary
  ring buffer of raw parsed lines (since removed): each picture arrived
  intact (`n=8192`), then a ~75-byte line starting `U\0\0\0...` was
  parsed. That is Zaparoo's PN532 auto-detect (`[readers] auto_detect =
  true` + a `pn532` entry in `/media/fat/zaparoo/config.toml`) probing
  `/dev/ttyACM0` with a PN532 wake-up preamble after each core change -
  Zaparoo's own log showed `PN532 auto-detect ... probed uart:/dev/ttyACM0`
  seconds after each `CORENAME` write. A real corename is plain printable
  text, so the firmware drops any bare line containing bytes outside
  0x20-0x7E. Verified clean on real hardware (MENU/TGFX16/MENU through the
  unmodified daemon). Two wrong theories came first and were reverted:
  unread `ttyack;` replies, and `readFixed()`'s `readBytes()` busy-spin
  (rewrote it with a yield loop; no effect). Paced vs burst sends looked
  like a cause only because the probe timing shifted.
- `CMDCLST,<transition>,<color 0-15>` — fill the screen with a solid
  4bpp-grayscale color and reveal it with a transition. Implemented by
  filling `legacyBuf` with the solid color and reusing the existing
  legacy-GSC draw path wholesale (exactly like the reference fills
  `logoBin`), so `CMDSPIC`/`CMDSSCP` redisplay of a `CMDCLST` screen falls
  out for free rather than needing a separate solid-fill code path.
- `CMDSPIC[,<effect>]` — redisplay the last-drawn picture (any kind) with
  a new transition. No redecode: JPEG-sourced pictures replay straight
  from `display.cpp`'s still-resident `g_frame`; legacy XBM/GSC pictures
  replay from `legacyBuf`, which already persists between commands.
  Tracked via `protocol.cpp`'s `PictureKind lastPictureKind` +
  `redisplayCurrent()`, set by every picture-drawing path (`CMDCOR`/
  `CMDAPD`/`CMDCORC`/`CMDCLST`). Matches the reference's own behavior of
  *not* clearing the screen first — `transitionReveal()`/`oled_drawlogo()`
  both only ever draw new pixels over whatever's already there, so
  replaying a transition over *unchanged* content is visually a no-op in
  both implementations, not a bug.
- `CMDSSCP` — redisplay the last-drawn picture at reduced ("1/4 area")
  size, no effect parameter (matching the reference). JPEG downsamples
  directly from `g_frame` (nearest-neighbor ×2, no new buffer — a 120×120
  scratch buffer would cost another ~28KB of static RAM we don't have to
  spare); legacy XBM/GSC reuses `display.cpp`'s `drawLegacyScaled()`
  helper (shared with the full-size `display_draw_legacy_xbm/gsc`) with a
  smaller destination rect instead of duplicating the pixel-decode loop.
- `CMDSAVER,<mode>,<interval-sec>,<logotime-sec>` / `CMDSWSAVER,<0|1>` —
  **parses the reference's exact grammar and range clamps** (mode 0-255,
  interval 5-600s, logotime 20-600s) so a real `tty2oled.sh` never
  errors, but only implements simple blank-after-idle (`mode>0` ⇒
  enabled, blank both displays after `interval` seconds idle, wake and
  restore both on the next command or GPIO9 press — see "GPIO9 wake
  button" below for the idle-timer details) — **not** the reference's
  animated multi-screen/starfield/toaster screensaver system. `logotime`
  is parsed for grammar compatibility but intentionally unused. This is a
  deliberate scope cut, not an oversight — see `protocol.cpp`'s
  `CMDSAVER`/`CMDSWSAVER` handlers for the exact divergence. Blanks the
  onboard status OLED too (`oled_status_off()`/`_on()`), added
  2026-08-13 once a real burn-in concern was raised for its always-on
  static dashboard — an OLED is emissive, so unlike the GC9A01's
  backlight-only "off", blanking it by content actually stops pixel
  current, not just dims it.

While implementing these, also fixed a latent bug in the existing
`CMDCOR`/`CMDAPD` legacy path: `effect < 0` was silently mapped to a
hardcoded `1` instead of randomizing like the reference's `-1` convention
does. All effect-taking commands (`CMDCOR`/`CMDAPD`/`CMDSPIC`/`CMDCLST`)
now share one `resolveEffect()` helper in `protocol.cpp` that clamps to
our curated `1..5` set and randomizes on `-1`, so `-1` means the same
thing everywhere.

### Backlight PWM has no effect on at least one real GC9A01 module

> **Root cause found 2026-10-27:** the module is the seven-pin `M128-240240-RGB-7-V1.0`, which has no BL pin at all (RST, CS, DC, SDA, SCL, GND, VCC; seen in the owner's photos of B610). Nothing is miswired; there is no backlight signal to drive. README documents it.

Discovered 2026-08-13 while testing `CMDSAVER`: `CMDDOFF`/`CMDCON,0`
(both drive the same `ledcWrite()` PWM call on `PIN_LCD_BL`) were
confirmed on real hardware to have **zero visible effect**, even after a
full hardware reset — almost certainly this module's `BL` pin is wired
straight to VCC rather than through a GPIO-controlled transistor, not a
software bug. Since backlight control can't be relied on, `display_off()`
(`display.cpp`) now **blanks by content** (`fillScreenFast(0x0000)`) in
addition to the (harmless, possibly-ineffective) PWM call, and callers in
`protocol.cpp` (`CMDDON`, the screensaver wake path) explicitly call
`redisplayCurrent()` after `display_on()` to restore content -
`display_on()` itself has no generic "what was on screen" concept beyond
`g_frame`'s JPEG-only content, so it can't restore anything by itself.
Confirmed working end-to-end on real hardware for both `CMDDOFF`/`CMDDON`
and the full `CMDSAVER` blank/wake cycle.

Legacy picture transfer: `CMDCOR,<name>,<effect>` or `CMDAPD,...` followed
by a **blocking fixed-size** read of exactly 2048 (1bpp XBM) or 8192 (4bpp
grayscale "GSC") raw bytes, classified purely by byte count — identical to
upstream, including its truncation trap, which is why it's only used for
the legacy path and not for new art.

New color command: `CMDCORC,<name>,<effect>,<durationMs>,<length>\n` +
exactly `<length>` raw JPEG bytes, **length-prefixed** (not
size-classified), read in a loop with an overall timeout — avoids the
byte-count trap for variable-length payloads. Decoded on-device with
JPEGDEC into a full-frame RGB565 buffer (`firmware/src/display.cpp`'s
`g_frame`), then revealed onto the display using `effect` over
`durationMs` (`transitionReveal()`). Effect ids are shared with the web
app's `web/src/effects.ts` so the numbers mean the same thing on both
ends: `0` none/instant, `1`/`2` wipe left↔right, `3` wipe top→bottom, `4`
iris (expanding circle from center), `5` fade (cross-dissolve). The web
app's editor has a "▶ Preview effect" button (with a slow/normal/fast
speed picker) that plays the same reveal pattern locally on the canvas —
useful without a device connected, and representative but not
pixel-identical to the firmware's version (`MarqueeEditor.playEffectPreview`
in `web/src/editor.ts`).

## Firmware pins

Edit `firmware/src/pins.h` to match your board — it's the single place
for all display pins again (`PIN_LCD_SCLK/MOSI/CS/DC/RST/BL`), since the
custom `gc9a01.h`/`.cpp` driver (see below) reads them directly, unlike
the library-based approaches tried earlier that needed pins elsewhere.

Current wiring, **verified on real hardware 2026-08-13** (both the
GC9A01 startup screen and the onboard OLED status dashboard confirmed
showing correctly after physically moving the MOSI/DC jumpers and
reflashing): `SCLK=4, MOSI=0, CS=7, DC=1, RST=10, BL=3`. MOSI/DC were
moved off their original GPIO6/GPIO5 (verified working 2026-08-12) to
free those pins for `PIN_OLED_SDA=5, PIN_OLED_SCL=6` — this board's
onboard 0.42" SSD1306 status OLED (see "Onboard status OLED" below) uses
those two as fixed I2C pins, not user-selectable, since it's built into
the board. Moving these pins in `pins.h` only changes what the firmware
expects; the physical jumper wires must be moved to match, or the GC9A01
goes blank while still compiling and flashing cleanly. Avoid the C3's
strapping pins (GPIO2, 8, 9),
the FN4/FH4 module's in-package flash pins (GPIO11-17), and GPIO20/21
(left free for the `Serial0` debug UART, see `protocol.cpp`'s
`DBG_ENABLED`) if you change these further. The native USB-Serial/JTAG
peripheral is fixed to GPIO18/19 in hardware and used automatically by
`Serial` — never wired manually, and what both a real MiSTer and the
WebSerial web app talk to.

### Sunton ESP32-2424S012C all-in-one board (2026-10-02)

Third build variant, `env:esp32c3_2424s012` (`-DBOARD_2424S012=1`,
`HAS_ONBOARD_OLED=0`): a 1.28" round GC9A01 with an ESP32-C3-MINI-1U built
onto the back, no touch variant. Display pins are fixed by the board and
`pins.h` selects them under `BOARD_2424S012`: `SCLK=6, MOSI=7, CS=10,
DC=2, BL=3, RST=-1`. The pin map came from memory of the Sunton pinout,
not a schematic, and was then **verified on real hardware** (display
works). There is no reset GPIO (panel reset is tied to the board reset),
so `gc9a01.cpp`'s `begin()` skips the RST pin when `PIN_LCD_RST < 0` and
sends the `SWRESET` command (0x01) before the init sequence instead; the
other envs still use the hardware reset pin unchanged. GPIO9 is the BOOT
button here too, so the wake/QR/factory-reset behavior is unchanged. Backlight
PWM on GPIO3 **works on this board** (verified on real hardware: stepping
`CMDCON` 128/64/16/0 visibly dims the screen, `CMDCON,255` restores it) -
unlike the "Backlight PWM has no effect" finding below, which was a
different module.

See `case/CLAUDE.md` for the 3D-printed case and stand notes (loaded when working under `case/`).

See `mister/CLAUDE.md` for the t2o-wifi daemon, art lookup order, pack distribution and menu notes (loaded when working under `mister/`).

See `firmware/CLAUDE.md` for CYD support, WiFi/AP, settings page, WebSocket, MQTT and WiFi link-quality notes (loaded when working under `firmware/`).

### Onboard status OLED

`firmware/src/oled_status.h`/`.cpp` drives this board's built-in 0.42"
SSD1306 (72×40 visible, addressed as a 128×64 controller with a
`kOffX=28, kOffY=24` window into that RAM — values copied from a working
WLED usermod for the same panel, not re-derived geometrically; a naive
`(128-72)/2, (64-40)/2` centering clips on real hardware) over I2C on
`PIN_OLED_SDA`/`PIN_OLED_SCL` (GPIO5/6). It's a **separate display from
the GC9A01** — shows firmware/connection status (core name, firmware
version, RX activity, uptime), not marquee art. `oled_status_loop()` is rate-limited to ~100ms
redraws and is called from `main.cpp`'s `loop()`. Because an I2C
`sendBuffer()` (~20ms) is long enough to starve `Serial`'s RX queue drain
if it runs mid-transfer (the exact class of bug that broke `CMDCORC`
originally — see below), `protocol.cpp` wraps every blocking picture-byte
read (`readFixed()`/`readExact()`, used by `handleLegacyPicture()` and
`handleColorPicture()`) in `oled_status_suspend()`/`oled_status_resume()`,
which skips redraws entirely for the duration. `protocol_get_corename()`
and `protocol_last_activity_ms()` are the read-only accessors
`oled_status.cpp` polls from `loop()` — cheap to call, never call them
from inside the suspend-guarded transfer path. If `u8g2.begin()` fails
(no OLED present on a given board), the module just stays permanently
inactive rather than erroring.

`oled_status_off()`/`_on()` (added 2026-08-13, driven by the same
`CMDSAVER` idle timer as the GC9A01's own screensaver - see "GPIO9 wake
button") blank the dashboard by content (`clearBuffer()`/`sendBuffer()`),
**not** u8g2's `setPowerSave()` - that API is known from firsthand
experience porting the WLED usermod below to hang this exact SSD1306
panel, requiring a full re-init to recover, not just a mode switch. An
always-on static dashboard is exactly the kind of content that burns in
on a real (emissive) OLED over time, so blanking it isn't optional
long-term the way it might be for an LCD.

Ported from a WLED usermod the same author wrote for this exact
ESP32-C3-mini + 0.42" OLED combo
(https://github.com/wled/WLED/pull/5475) — reused for the I2C
init/offset/font conventions, not the WLED-specific dashboard content
(effect name/brightness graph) or its config/button/LED-heartbeat layer,
none of which apply here.

### OLED idle blank and live command line (2026-10-14)

An always-on dashboard burns in, so `oled_status.cpp` blanks the glass (by
content, never `setPowerSave()`) after `kIdleBlankMs` = 60 s with no command
from any transport and no GPIO9 press, counted from boot so the IP/version stay
readable after power-up; any activity wakes it. This is independent of
`CMDSAVER` (which can still blank it sooner via `oled_status_off()`). Line 3
now shows the last command received (`> CMDCOR`, plus `*` for 2 s when fresh),
fed by `protocol_note_command()` (called from `protocol_dispatch_line()`, the
WebSocket CMDCORC/CMDHWINF handlers and MQTT notifications), and uptime moved
onto the version line. `protocol_last_wake_ms()` exposes the shared
wake timestamp. Compile-verified for all three envs; the idle-blank and the
live command line are both **confirmed working on the real OLED** (owner,
2026-10-21).

### Firmware version and repo URL

`firmware/src/version.h` defines `FW_VERSION` and `REPO_URL` as the
single place to bump on release — shared by `protocol.cpp` (`CMDHWINF`'s
reply, `CMDSHSYSHW`'s screen), `display.cpp` (GC9A01 boot splash), and
`oled_status.cpp` (status dashboard, appended to the same line as the
existing RX indicator to fit the tiny OLED's ~31px of usable vertical
space). `REPO_URL` deliberately omits the `github.com/` prefix — the full
URL is ~240px wide at the boot splash's 6px/char font, wider than the
240px display itself; the shorter `owner/repo` form fits within the
round display's ~91%-inset safe area at all three splash-screen line
positions (verified on real hardware 2026-08-13, after an initial version
that centered "tty2oled" with a fixed pixel offset - now `getTextBounds()`
on every line instead, like `display_show_corename()` already did).

### GPIO9 wake button

This board's onboard "BOOT" pushbutton (GPIO9) is repurposed at runtime
as a screensaver wake button (`protocol.cpp`'s `protocol_button_check()`,
polled from `main.cpp`'s `loop()`). GPIO9 is a C3 strapping pin (pulled
low at reset = download mode), which is why it's on `pins.h`'s
"avoid wiring anything else to this" list — but that only matters during
boot, not while running, so reading it as a plain input afterward is
safe and doesn't interfere with normal flashing/boot. No debounce logic:
touching the idle timer repeatedly while the button is held is
idempotent, and releasing it just lets normal idle timing resume on its
own.

Screensaver idle timing runs off a separate `lastWakeMs` timestamp, not
the pre-existing `lastActivityMs` — `lastActivityMs` is also what
`oled_status.cpp`'s dashboard uses for its serial-RX indicator, and a
button press isn't a received command, so touching the shared timestamp
would make that indicator lie. `protocol_process()` keeps both in sync on
every dispatched line; only the button touches `lastWakeMs` alone.
Verified end-to-end on real hardware 2026-08-13: screensaver blanks after
idle, GPIO9 press wakes it and restores the last picture. Since
`lastWakeMs` also drives the onboard OLED's blank/wake (see "Onboard
status OLED" and `CMDSAVER` above), the same button press wakes both
displays together - also confirmed on real hardware.

### Why a custom GC9A01 driver, not a display library

Two libraries were tried and both had real ESP32-C3 bugs, discovered only
by actually flashing hardware (not by code review — both looked fine and
built without errors):

1. **`moononournation/GFX Library for Arduino`** (`Arduino_GC9A01`): its
   fast-path SPI databus (`Arduino_ESP32SPI`) references
   `DR_REG_SPI3_BASE`, a hardware register that only exists on classic
   ESP32/S2/S3 (the C3 has fewer SPI peripherals) - and the library's own
   master header unconditionally pulls that file in for every consumer
   regardless of which databus class you actually instantiate, so
   restricting our own `#include`s to just the pieces we needed didn't
   help. Newer releases separately require `esp32-hal-periman.h`, a header
   no resolvable Arduino-ESP32 core version provided either. **Compile-time
   failure** - never got as far as flashing.
2. **`TFT_eSPI`**: compiled and flashed cleanly, but `tft.init()` hung the
   chip on every boot (`rst:0x8 TG1WDT_SYS_RST` - task watchdog, in a
   crash loop). Bisected by adding `Serial.println()` checkpoints through
   `setup()`/`display_init()` to find which call never returned, then
   independently proved the wiring/SPI hardware itself was fine with a
   bare-`SPIClass` diagnostic sketch (worked instantly, no hang) before
   concluding the bug was in TFT_eSPI's ESP32-C3-specific
   `driver/spi_master.h`-based init path, not our code or wiring. **Runtime
   hang** - looked correct at compile time, only showed up on real hardware.

Given both, wrote a **~150-line direct driver** (`gc9a01.h`/`.cpp`) on top
of plain Arduino `SPIClass` (the exact thing proven working in the
diagnostic sketch) - manual CS/DC/RST toggling, the GC9A01's standard
vendor register-init sequence (adapted from TFT_eSPI's
`TFT_Drivers/GC9A01_Init.h`, MIT licensed - functional hardware-bringup
data, reused for interoperability with this exact chip, not display-library
code), and a fast `pushRect()` bulk blit for JPEG frames/transitions.
Subclasses `Adafruit_GFX` *only* for its software text/shape routines (no
display-specific driver code in Adafruit_GFX itself, so nothing
C3-specific left to break) - `drawPixel()` is the only thing it strictly
needs from us.

### RAM headroom

Build reports **~87.1% RAM usage** (285248 / 327680 bytes, `env:esp32c3`
with the onboard OLED) vs **~86.6%** (283808 / 327680 bytes,
`env:esp32c3_nooled`, see "Build variants" above) — only ~1.4KB
difference (2026-08-13, measured directly by building both envs).
`Wire`/the ESP-IDF I2C driver is **not** what the OLED adds: it's already
a hard transitive dependency of Adafruit GFX Library (`Adafruit_GFX.h`
includes `Adafruit_BusIO`'s `Adafruit_I2CDevice.h`, which requires
`Wire.h` to even compile `display.cpp`/`gc9a01.cpp`) — confirmed by
trying `lib_ignore = Wire` in `env:esp32c3_nooled`, which broke the build
with `fatal error: Wire.h: No such file or directory` from
`Adafruit_I2CDevice.h`. So Wire's cost was already baked into the
firmware before this session's OLED work; only U8g2 itself (excluded
from `env:esp32c3_nooled`'s `lib_deps`) plus a couple of small globals
account for the real difference. Both static 240×240×2-byte (112.5KB)
full-frame buffers in `display.cpp` (`g_frame` and the fade effect's
`blended` scratch buffer) remain the dominant cost either way. Leaves
**~42-44KB** free for stack/heap at runtime in both variants — fits the
32KB `ARDUINO_LOOP_STACK_SIZE` plus Serial's 4KB/2KB RX/TX queues plus
JPEGDEC's own small working buffers, but this has **not yet been
re-tested on hardware** with a full `CMDCORC` transfer under either
variant — do that before trusting it, since this exact class of problem
(silent allocation failure under tight/fragmented heap) is what caused
the original CMDCORC bring-up bugs.

## Build & flash

The same ESP32-C3-mini board is sold both with and without the built-in
0.42" status OLED, so `firmware/platformio.ini` has two build variants
controlled by the `HAS_ONBOARD_OLED` build flag — GC9A01 pins and
behavior are identical in both, only the status OLED (and its U8g2
library dependency) is compiled in or out (see "Onboard status OLED"
above):

```bash
cd firmware
pio run -e esp32c3 --target upload           # default: includes the onboard OLED
pio run -e esp32c3_nooled --target upload    # board variant without the OLED
pio device monitor          # 115200 baud, watch for "ttyrdy;"
```

Both variants verified on real hardware 2026-08-13: `esp32c3` shows the
status dashboard on the onboard OLED, `esp32c3_nooled` leaves it dark
(and off entirely — `Wire.begin()`/`u8g2.begin()` are never called) while
the GC9A01 behaves identically either way.

## Web flasher + GitHub Pages (2026-10-13)

`flasher/index.html` is a static [ESP Web Tools](https://esphome.github.io/esp-web-tools/)
page (pinned `esp-web-tools@10.0.1` from unpkg) with a board picker that
swaps the `manifest` attribute between four manifests, one per PlatformIO
env (`esp32c3`, `esp32c3_nooled`, `esp32c3_2424s012`, and the classic-ESP32 `esp32_cyd`, see "Release plumbing for the CYD"). `tools/build_site.sh`
assembles `site/` (gitignored): the flash parts (bootloader, partition
table, boot_app0, one app image per board), the generated manifests (each
writes the four parts at 0x0/0x8000/0xe000/0x10000), the flasher at `/`, and the Vite-built web app at `/app/`
(`--base=./`, so `importLibrary.ts` builds its fetch URLs from
`import.meta.env.BASE_URL` instead of a hardcoded `/library/`).
`.github/workflows/pages.yml` is `workflow_dispatch` only ("Run workflow",
optional tag, default latest release; `tools/release.sh` dispatches it after
publishing) and deploys with the Pages actions. It is NOT triggered by the
`release` event: that run executes on the tag ref and the `github-pages`
environment only allows deploys from main, so GitHub rejected it (v0.4.2:
"Tag is not allowed to deploy to github-pages") - a dispatched run executes on
main and works. Setup is Settings -> Pages -> Source: GitHub Actions.

**The workflow publishes a release's attached `.bin` files; it does not
build firmware.** Releases are cut with `tools/release.sh` (checklist prompts,
builds the four envs, merges, verifies the version string and sizes,
tags, `gh release create`; `--dry-run` rehearses without publishing - a
fresh local rebuild reproduces the released images byte-for-byte). By hand
it's: build the three envs locally,
`esptool.py merge_bin` each (`0x0` bootloader, `0x8000` partitions, `0xe000`
boot_app0, `0x10000` app; dio, 80m, 4MB - the layout `pio run -t upload`
uses), `gh release create` with the files named `*-<env>.bin`. Why not
build in CI (tried first, failed review): the firmware tested on real
hardware is a **hybrid toolchain** that a clean machine can't reproduce.
`platform = espressif32` resolves to whichever matching platform is already
installed; locally that's Tasmota's `2024.06.00` (Arduino-ESP32 2.0.18, no
`WiFiClientSecure`), and the `lib_extra_dirs` line in `platformio.ini`
points at a *separate* official `framework-arduinoespressif32` 2.0.17
package that supplies `WiFiClientSecure.h` (and the WiFi/WebServer libs).
A clean CI checkout instead resolves to the registry's `espressif32@7.1.3` +
official Arduino 2.0.17: builds fine, but is a 1.0 MB image instead of 0.73
MB, a different core, and was never run on hardware. Pinning to the Tasmota
platform (tried, reverted) makes clean machines fail with `WiFiClientSecure.h
not found`. If the firmware is ever moved to a reproducible toolchain,
re-verify everything in this file on hardware first, then CI can build it.

### Release plumbing for the CYD (2026-10-24)

The CYD is a different chip, so it does not share the C3 parts: the classic ESP32's bootloader sits at **0x1000** (not
0x0), the manifest chip family is `ESP32`, and its bootloader and partition table differ from the C3's (`boot_app0.bin`
is byte-identical across every framework copy, so the shared copy is reused). Its assets use their own prefix,
`tty2oled-ili9341-<ver>-{bootloader,partitions,boot_app0}.bin`, `-esp32_cyd-app.bin` and `-esp32_cyd-full.bin`; the C3
ones keep `tty2oled-gc9a01-*`.

- `tools/release.sh`, block "3a": builds `esp32_cyd`, merges its full image with `--chip esp32 --flash_freq 40m` at
  0x1000/0x8000/0xe000/0x10000, copies the four parts, and verifies it like the others (size; parts reproduce the full image
  byte for byte). The version check differs on purpose: a standalone `0.5.9` string is NOT in the CYD image (the Xtensa
  linker merges it into the tail of "GC9A01  v0.5.9", the RISC-V one used for the C3 does not), so it greps for the
  CMDHWINF reply `;<ver>;` ("HWCYD28;0.5.9;") instead. The checklist prompt "flashed THIS build to a real board" now means
  the CYD too.
- **Credential guard:** if `firmware/src/wifi_dev.h` exists, no `.bin` in the release may contain its SSID or password.
- `tools/build_site.sh`: the C3 `pick` patterns are now `tty2oled-gc9a01-*` (a bare `*-bootloader.bin` would also match the
  CYD's); the CYD writes `bootloader-esp32.bin`, `partitions-esp32.bin`, `app-esp32_cyd.bin` and `manifest-esp32_cyd.json`
  (offsets 4096/32768/57344/65536). The CYD is **optional**: a release made before it existed (e.g. re-dispatching Pages for
  v0.5.8) has no `tty2oled-ili9341-*` assets, and the site is then built without the CYD manifest and without its `<option
  data-cyd="1">` in the page, so the old-tag path still works (tested).
- `flasher/index.html`: a CYD entry; the "hold BOOT" step and port name are C3-only (`.c3-only` / `.cyd-only` toggled by
  the picker; the CYD's CH340 resets it into download mode itself), plus a `[hidden]{display:none!important}` rule.
- **Verified:** `release.sh --dry-run` with a temporary 0.5.9 (all four builds, daemon, database, credential check); the
  site built from that release dir has every file byte-identical to its release asset, the C3 files are still the C3 ones,
  and the old-release path drops the CYD; the CYD's separate parts, taken from the dry-run release, were written to the real
  board with esptool at exactly the manifest's offsets: it booted, kept its saved WiFi/settings (NVS untouched) and answered
  `HWCYD28;0.5.9;`. The ESP Web Tools page itself could not be tried
  headlessly (no Web Serial); the owner then flashed a CYD from the live page after the v0.6.0 release and it worked, so
  the whole released path (manifest, chip family `ESP32`, offsets, the CYD-specific instructions) is now verified.
- Gotcha: the dry run writes `release/<FW_VERSION>/`, which would overwrite the local copy of an already-published release,
  so rehearse with a temporarily bumped `FW_VERSION`.

First real flash from the page (2026-10-13, Super Mini): "Failed to
initialize" until BOOT was held while plugging in. `esptool.py` from the CLI
entered the bootloader fine on the same port (so USB/hardware were OK and
ModemManager had no modems) - the browser's esp-web-tools 10.0.1 just can't
auto-reset the C3's native USB while the firmware's HWCDC is running. The
page now puts "hold BOOT while plugging in" as step 1. If newer ESP Web
Tools handles the USB-JTAG reset, try bumping the pinned version and drop
the requirement.

**A merged flash image erases saved settings (found 2026-10-14, the hard
way).** The first versions served one `esptool merge_bin` image per board
written at 0x0. merge_bin fills the gaps between partitions with 0xFF, and the
NVS partition (0x9000, 0x5000) sits in one - verified: 20,480 bytes of 0xFF
there - so flashing it wiped the saved WiFi/MQTT config and the device came up
in AP setup mode. It showed up as "booted in AP mode" right after flashing the
released image, and it made the README/flasher claim "flashing keeps your
saved WiFi settings" false. Now the flasher manifests list four parts and the
release carries them (`*-bootloader.bin`, `*-partitions.bin`,
`*-boot_app0.bin`, `*-<env>-app.bin`; bootloader/partitions/boot_app0 are
identical across boards, `release.sh` checks that and that the parts
reproduce each full image). Full images remain as release assets for fresh
devices only (`*-<env>-full.bin`, 0.4.3+; v0.4.2 has them without `-full`).
`pio run -t upload` was always fine: it writes the parts separately.

**ESP Web Tools erases the whole flash unless told to ask (found 2026-10-14, v0.5.1
report: "flashed 0.5.1 but didn't keep the settings", device came up in AP mode).**
The manifest flag is `new_install_prompt_erase`, and `false` does NOT mean "don't
erase": in `install-dialog` the Install button calls `_startInstall(true)` (erase) when
the flag is falsy and only shows the "Erase device" checkbox (default off) when it is
`true`. Manifests had `false` since the flasher was added, so every browser flash wiped
NVS (the earlier "merged image erases NVS" finding was real too, but this masked it;
the separate-parts layout is still right). Now `true` in `tools/build_site.sh`; the
Pages workflow was re-dispatched, no new firmware release needed. Verified by the owner (browser flash of 0.5.1 kept WiFi and settings). Verify the dialog
shows the unticked checkbox after any ESP Web Tools bump.

Gotchas: the script deletes `site/app/library` on purpose - `web/public/library`
holds the gitignored third-party art pack, which a *local* Vite build copies
in and must never be published. Web Serial can't be exercised headlessly, so
the page's flashing itself is verified only by serving `site/` and checking
every URL resolves; the published images are byte-identical to the release
assets (checked with `cmp`). The marquee app's IndexedDB lives at the shared
`doccaz.github.io` origin and WiFi transport doesn't work from the https page
(browsers block `ws://` from https); USB works.

See `web/CLAUDE.md` for the web app notes (loaded when working under `web/`).

Deferred full-color marquee research: `docs/color-marquee-notes.md` (read it before resuming that work).

## Known gaps / follow-ups

- Legacy grayscale crop numbers (`LEGACY_W/H` in `display.cpp`) were spot
  checked against real converted `.gsc` samples (edge-vs-center pixel
  brightness on 15 random images): most have blank/background margins in
  the outer ~8px as assumed, but a handful (e.g. images with a full-bleed
  background pattern) don't — worth an eyeball pass over the actual
  library once art is being assigned for real, not just a geometric
  guarantee.
- Transition effects are a small curated set (basic wipe), not the
  original's 23.
- **No `tty2oled.sh` fork — decided against (2026-10-21).** We co-exist
  with the stock daemon (that's what `mister/` t2o-wifi is for) and handle
  all the color art ourselves; we will not patch the stock script to send
  `CMDCORC`. The web app's WebSerial previewer doesn't need a MiSTer at all.
- Pin defaults in `pins.h` (`SCLK=4, MOSI=0, CS=7, DC=1, RST=10, BL=3`,
  plus `PIN_OLED_SDA=5, PIN_OLED_SCL=6`) are **verified on real hardware**
  (2026-08-13): GC9A01 shows its startup screen and the onboard OLED
  shows the "tty2oled" status dashboard, both after physically moving the
  MOSI/DC jumpers from their original GPIO6/GPIO5 and reflashing.
- Onboard status OLED (`oled_status.h`/`.cpp`) is confirmed showing the
  live dashboard on real hardware (2026-08-13), **including with a
  `CMDCORC` transfer in flight** (2026-08-13: a 5365-byte JPEG sent with
  the OLED active completed and rendered cleanly) — the
  `oled_status_suspend()`/`resume()` guard around `readFixed()`/
  `readExact()` is confirmed working under real load, not just reasoned
  about, and the reduced RAM headroom (see "RAM headroom" above) held up.
- **`CMDCORC` JPEG rendering is verified end-to-end on real hardware**
  (2026-08-12, re-confirmed 2026-08-13 with the OLED active and the new
  protocol commands in place): JPEGs transfer and decode correctly. All
  six transition effects (wipe ×3, iris, fade, instant) are now
  individually verified correct on real hardware (2026-08-13) - three of
  them (wipe left→right, wipe right→left, iris) had real bugs before that
  (see "transitionReveal() pushRect stride bug" above) that this specific
  wording previously glossed over as just "wipe/iris/fade" working.
  Legacy `CMDCOR` rendering over the wire is **now verified on real
  hardware too** (2026-10-03, ESP32-2424S012C, over native USB-CDC): a
  2048-byte 1bpp XBM (`NES.xbm`) and two 8192-byte 4bpp GSC files
  (`C64.gsc`, `battroad.gsc`) from the local packs, sent with effects 1,
  3, 4, 5 and -1 (random), each acked with `ttyack;` and visually
  confirmed correct on the display (centered/letterboxed, no skew). Note
  some pack files declare `_bits[8198]`/`[2054]` (6 extra header bytes) -
  those must be stripped to exactly 8192/2048 before sending, since the
  firmware classifies by byte count (`tools/convert_library.py` does the
  same). `CMDAPD` shares the same handler path but wasn't sent separately.
- **New protocol commands verified end-to-end on real hardware
  (2026-08-13)**: `CMDBYE`, `CMDTEST`, `CMDSHSYSHW`, `CMDHWINF` (reply
  format confirmed over the wire), `CMDCLST`, `CMDSPIC` (both JPEG- and
  GSC-sourced redisplay), `CMDSSCP`, and the full `CMDSAVER`/`CMDSWSAVER`
  blank-after-idle/wake cycle (see "Wire protocol" above). This testing
  is also what surfaced and fixed the backlight-PWM hardware issue (see
  "Backlight PWM has no effect on at least one real GC9A01 module"
  above) — `CMDDOFF`/`CMDDON` are now confirmed working too, which they
  weren't before (backlight PWM alone was silently a no-op on this
  hardware).
- The test image used for hardware validation was scaled/cropped
  incorrectly (truncated at the edges) because it was a raw ad hoc
  240×240 JPEG made directly for the test script, not run through the
  web app's actual editor (which applies the round display's
  `center-scale` 91% inset). The firmware itself does no scaling — it
  just centers whatever pixel dimensions it decodes. Next real-world test
  should go through the actual web app export path, not a hand-made test
  file.

### Root causes of the CMDCORC bring-up bugs (found 2026-08-12)

Three independent bugs combined to make `CMDCORC` completely non-functional
on real hardware, manifesting as raw JPEG bytes scrolling across the
display as if misread as commands. All three are now fixed and confirmed
on hardware:

1. **Dispatch order**: `dispatch()` checked `cmd.startsWith("CMDCOR")`
   before `cmd.startsWith("CMDCORC")`, so every `CMDCORC` line was
   swallowed by the legacy fixed-8192-byte handler (byte-count mismatch
   against a variable-length JPEG → "Picture xfer error"). Fixed by
   checking `CMDCORC` first (`protocol.cpp`'s `dispatch()`).
2. **`colorBuf` malloc failure**: `colorBuf` was `malloc(40000)`'d per
   call. Heap fragmentation on the C3 made that malloc fail
   *intermittently* despite tens of KB of nominally free heap — no single
   40KB contiguous block was available. The failure was silent:
   `handleColorPicture()` returned before ever reading the incoming JPEG
   bytes off the wire, leaving them unconsumed for the main loop to
   misread as garbage commands — this was the literal cause of the "binary
   data scrolling on screen" symptom. Fixed by making `colorBuf` a static
   20000-byte array (`protocol.cpp`), eliminating the malloc dependency.
3. **RX queue silently undersized**: `Serial.setRxBufferSize(16384)`
   **silently fails and returns 0** if the requested size doesn't fit in
   available heap at that point in boot (~28KB free, most of the C3's RAM
   already consumed by `display.cpp`'s static 240×240 frame buffers) —
   `HWCDC::begin()` then falls back to its own tiny 256-byte default queue
   whenever the queue is still NULL, with the ISR silently dropping bytes
   once it's full. This was undetected for a long debugging stretch
   because the return value wasn't checked. Found via an independent
   hardware UART0 (GPIO20/21) debug channel wired through an external
   FTDI adapter, bypassing the very USB-CDC channel under suspicion.
   Fixed by checking the return value and falling back to 4096 (which
   allocates reliably).
4. **Sender-side burst overflow** (the last piece, found after the above
   three): even with a correctly-sized ~4KB RX queue, the web app's
   `sendColorArt()` wrote the *entire* multi-KB JPEG in one
   `writer.write()` call, which overflows a 4KB queue long before the
   firmware's `loop()` gets a chance to drain it — the queue has no
   backpressure back to the browser. Fixed by `writePaced()` in
   `web/src/serial.ts`, which chunks writes to 512 bytes with a 10ms gap.
   Confirmed on hardware: a full 10352-byte transfer that previously timed
   out at 5s (having received as little as ~1.4–4.6KB) now completes in
   ~215ms once paced.

`readExact()` in `protocol.cpp` also switched from a single
`Serial.readBytes()` call to an explicit `yield()`-based read loop, since
`Stream::readBytes()`'s busy-spin starves the FreeRTOS task that feeds
bytes from USB hardware into HWCDC's queue on this single-core chip.

### transitionReveal() pushRect stride bug + per-row transaction stall (found 2026-08-13)

Reported by the user: wipe left→right showed visible artifacts while
drawing; wipe right→left and iris didn't work at all; fade and instant
were fine; the web app's own local canvas preview of the same effects
was unaffected. That last detail pointed straight at `display.cpp`'s
`transitionReveal()`/`gc9a01.cpp`'s `pushRect()` - firmware-only, nothing
wrong with the effect logic itself. Two independent bugs, both in
`GC9A01Display::pushRect()`:

1. **Stride bug (the artifacts)**: `pushRect(x, y, w, h, data)` read
   `w*h` pixels from `data` assuming it was a densely-packed `w`-wide
   buffer. `display.cpp`'s `g_frame` is always 240 pixels wide -
   `transitionReveal()`'s left→right wipe calls
   `pushRect(0, 0, w, DISP_H, g_frame)` with `w < 240` during the
   animation, so every row after the first read starting `240-w` pixels
   too early, into the *previous* row's trailing pixels - visible as
   exactly the kind of artifact reported. The top→bottom wipe and
   full-frame draws were never affected: they always pass `w == 240`,
   which happens to equal `g_frame`'s real stride, so the bug was silent
   there. Fixed by adding a `srcStride` parameter (default `w`, so
   existing full-width/single-row callers are unaffected) and having
   every partial-width caller pass `g_frame`'s real width (`DISP_W`)
   explicitly.
2. **Per-row transaction stall (the "doesn't work" effects)**: the
   right→left wipe and the iris effect pushed one row at a time in a
   loop (`for y: gfx.pushRect(x0, y, w, 1, ...)`), and `pushRect()` opens
   and closes a *full* SPI transaction (`SPI.beginTransaction()`/CS
   toggle/`SPI.endTransaction()`) on every call - up to 240 separate
   transactions per animation step, on hardware this project has already
   found to be sensitive to exactly this class of timing issue (see the
   `CMDCORC` bring-up bugs above). Fixed by adding
   `beginBatch()`/`pushRectBatched()`/`endBatch()`: one transaction/CS-low
   span wraps the whole per-row loop, and `pushRectBatched()` only issues
   the (cheap) address-window command per row instead of a full
   transaction. The right→left wipe no longer needs a per-row loop at
   all once the stride fix lets it collapse into one
   `pushRect(x0, 0, w, DISP_H, &g_frame[x0], DISP_W)` call; only iris still
   loops per row, since its visible x-range genuinely differs per row
   (circular mask), but now inside one batch.

All six effects (wipe ×3, iris, fade, instant) re-verified individually
on real hardware after the fix.
