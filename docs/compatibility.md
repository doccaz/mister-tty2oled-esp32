# Compatibility notes

This firmware was checked against the actual `tty2oled.sh` daemon script
(not just its documentation) to confirm real-world compatibility:

- The daemon never reads anything back from the device in its main
  loop — it just writes commands with fixed sleeps between them, so this
  firmware's `ttyack;` replies are harmlessly ignored, not required.
- **Upstream `installer.sh` knows this firmware.** `CMDHWINF` replies
  with the new hardware id `HWGC9A01C`. The installer used to treat an
  unrecognized `HW…` id as flashable and would claim "Flash progress
  completed" without flashing anything; since
  [venice1200/MiSTer_tty2oled#43](https://github.com/venice1200/MiSTer_tty2oled/pull/43)
  (merged 2026-10-13) it prints a short notice pointing here and exits.
  Update firmware with PlatformIO, not the MiSTer installer. **The CYD answers
  `HWCYD28`, which upstream's `installer.sh` does not know yet**, so with a CYD attached
  by USB the installer would behave the way it used to for any unrecognised id (claim
  success without flashing). Flash a CYD with the web flasher or PlatformIO, never with
  the MiSTer installer; an upstream notice for `HWCYD28` would be the same one-line change
  as the `HWGC9A01C` one.
- **Zaparoo's NFC reader probe.** Zaparoo's PN532 auto-detect opens every
  `/dev/ttyACM*` (including this display) after each core change and
  writes a PN532 wake-up preamble (`0x55 0x55 0x00 …`). An older build drew
  those bytes as a garbage corename right after each logo. The firmware now
  ignores any bare line containing control or non-ASCII bytes, so nothing
  needs changing on the MiSTer. If you'd rather stop the probing at the
  source, give the `pn532` entry in `/media/fat/zaparoo/config.toml` an
  explicit reader path instead of relying on `auto_detect`.

  **On the CYD this is required, not optional.** It is wired through a CH340
  on `/dev/ttyUSB*`, and a picture takes about 0.7 s at 115200 baud (a C3's
  USB-CDC finishes in milliseconds). Zaparoo reacts to a core change within
  about half a second, so its probe can land in the middle of a picture
  transfer. The logo then comes out sheared or shifted sideways (a few extra
  bytes pushed into the 8192-byte picture), and the same file may draw fine the
  next time. Seen on a MiSTer with `[readers] auto_detect = true`, with and
  without an NFC reader plugged in; the daemon was sending exactly 8192 correct
  bytes. The inserted bytes were not captured; the cause rests on the timing
  (Zaparoo's log shows a PN532 probe of the display's port inside the transfer
  window) and on the fix below, which made every picture clean.

  Fix: turn auto-detection off and connect the reader explicitly, so Zaparoo
  never opens the display's port. In `/media/fat/zaparoo/config.toml`:

  ```toml
  [readers]
  auto_detect = false

  [[readers.connect]]
  driver = 'pn532uart'
  path = '/dev/serial/by-path/platform-ffb40000.usb-usb-0:1.1:1.0-port0'
  ```

  then `/media/fat/Scripts/zaparoo.sh -service restart`. Verified on a real MiSTer
  with the CYD and a PN532 reader both plugged in: the reader connected and
  scanned a tag, Zaparoo's log showed no further probes of the display's port,
  and 12 rapid core changes all drew clean. With no NFC reader at all,
  `auto_detect = false` alone is enough. Zaparoo's docs have no per-port
  exclusion, only the global and per-driver `auto_detect`, so an explicit entry
  is the way to keep a reader and protect the display.

  **None of this applies over WiFi.** A display driven by the `t2o-wifi` daemon
  (see "MiSTer daemon over WiFi") gets its pictures over a WebSocket, not a
  serial port, and the daemon never opens a `/dev/tty*` device, so there is
  nothing for Zaparoo's probe to interfere with. This follows from how the
  transport works; the corruption was only ever seen on the wired path, and I
  didn't specifically test the WiFi path against an active probe.
- **Two USB-serial adapters: use `by-path`.** The CYD and many NFC readers are
  both CH340 (`1a86`) devices. They get `/dev/ttyUSB0`/`ttyUSB1` in whatever
  order they enumerate, so a replug or reboot can swap them, and then the stock
  daemon (`TTYDEV="/dev/ttyUSB0"`) sends pictures to the reader. A replugged port
  also loses the `stty` settings the daemon applied once at start-up (back to
  9600 baud) until the daemon restarts. `/dev/serial/by-id` can't help (identical
  ids, so only one link appears); `/dev/serial/by-path/platform-…-usb-0:1.2:1.0-port0`
  is distinct per physical USB port. Set `TTYDEV` in `tty2oled-system.ini` to it
  (list yours with `ls -l /dev/serial/by-path`) and restart the daemon
  (`/media/fat/tty2oled/S60tty2oled restart`); keep each device in the same port.
- Related Zaparoo issues: [#89 "Conflict with tty2oled"](https://github.com/ZaparooProject/zaparoo-core/issues/89)
  (closed; a maintainer said the cause was identified, no confirmed fix in the
  thread; the tty2oled author's workaround is in the
  [tty2oled wiki](https://github.com/venice1200/MiSTer_tty2oled/wiki/Troubleshooting)
  under "tapto recommendation") and
  [#99 "Launching cores causes UART reader to disconnect"](https://github.com/ZaparooProject/zaparoo-core/issues/99)
  (closed; also a tty2oled conflict). Zaparoo's reader options are documented at
  [zaparoo.org/docs/core/config/readers](https://zaparoo.org/docs/core/config/readers/).
- **Device path matters.** The ESP32-C3 boards use the chip's native
  USB-CDC peripheral, which Linux enumerates as `/dev/ttyACM*` — the
  original hardware used a CP2102/FTDI-style chip, enumerating as
  `/dev/ttyUSB*` (so does the CYD, through its CH340). Point `TTYDEV` at the right one in
  `tty2oled-system.ini`, and make sure `USBMODE="yes"` (otherwise the
  daemon only ever sends plain core names, no picture data).
- `CMDSETTIME`, which `tty2oled.sh` sends once at daemon startup, **is
  implemented** (since 2026-10-10; this note used to say it wasn't, and that it
  flashed as garbled "core name" text). The MiSTer sends its local time as epoch
  seconds already shifted by the timezone offset, and the firmware keeps it as
  the clock for the idle clock screen (`CMDSHTIME`). It isn't saved across
  power cycles (there's no battery-backed RTC), but the script re-sends it
  at every start, and once the device has synced with an NTP server (the default
  once it's on WiFi, see [Device settings](setup.md#device-settings-v050)) the NTP time wins
  and `CMDSETTIME` is ignored. `CMDSAVER` is implemented too, see [Wire
  protocol](protocol.md#wire-protocol) above.

# Why a custom GC9A01 driver

Two existing Arduino display libraries were tried first and both had real
ESP32-C3 bugs, only discovered by flashing actual hardware:

1. **`moononournation/GFX Library for Arduino`** — its fast SPI databus
   references a hardware register (`DR_REG_SPI3_BASE`) that doesn't exist
   on the C3, pulled in unconditionally regardless of which databus class
   is actually used. Compile-time failure, never got as far as flashing.
2. **`TFT_eSPI`** — compiled and flashed cleanly, but hung the chip on
   every boot inside its ESP32-C3-specific SPI init path (task watchdog
   reset, confirmed via bisection with checkpoint logging, with the
   underlying SPI wiring separately proven fine via a bare `SPIClass`
   diagnostic sketch).

Given both, `firmware/src/gc9a01.h`/`.cpp` is a ~150-line direct driver
on top of plain Arduino `SPIClass` — manual CS/DC/RST toggling, the
GC9A01's standard vendor register-init sequence (adapted from
TFT_eSPI's MIT-licensed `GC9A01_Init.h` — reused as functional
hardware-bring-up data, not as library code), and a fast `pushRect()`
bulk blit for JPEG frames and transitions. It subclasses `Adafruit_GFX`
only for its software text/shape routines.
