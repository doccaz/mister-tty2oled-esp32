# firmware/ notes (moved from the root CLAUDE.md)

### CYD (ESP32-2432S028) support (branch `cyd-support`, 2026-10-24)

`env:esp32_cyd` (`-DBOARD_CYD=1`): the 2.8" 320x240 ILI9341 "Cheap Yellow Display" on a classic ESP32 (CH340
UART serial, not native USB). Panel settings were found with a probe sketch on the owner's real board, not
assumed: ILI9341-style minimal init (SWRESET, COLMOD 0x55, inversion off), RGB order, MADCTL `0x60` upright
(`0x20`/`0xE0` came out rotated/mirrored), HSPI at 40 MHz on SCLK 14 / MOSI 13 / MISO 12 / CS 15 / DC 2,
backlight 21 (PWM), RST on GPIO4 (shared with the red LED). The RGB LED is active low and lit by the floating
pins at power-up; `display_init()` drives 4/16/17 high. The BOOT button is GPIO0; the XPT2046 touch (33/36/25/32/39) wakes the display too (see "Touch wake"); a
settings screen with touch support is still to do. The light
sensor is an analog photoresistor on GPIO34 (see "Auto brightness" below). HW id `HWCYD28` (`HW_ID` in `version.h`; the C3 boards still answer `HWGC9A01C`).

- **Canvas:** still 240x240; `PANEL_OFF_X` (40) in `setAddrWindow()` centres it, `clearPanel()` blacks the side
  bars once. Existing art is shown as is; wide art would be a later, separate step.
- **Memory is the constraint (measured, 2026-10-24).** The classic ESP32 has ~160KB static DRAM, no free heap
  block of 115,200 bytes (largest 110,580 even after `esp_bt_mem_release()`), and, decisively, **its WiFi/TCP
  stack needs about 80KB of free heap to serve anything**: a minimal WiFi + WebServer sketch passed 40/40 requests
  at ~80KB free and above, 17/40 at ~70KB, 0/40 at ~60KB (heap burned down with filler allocations). A first
  version kept the frame in five 48-row heap bands (the C3 design) and left ~60KB: the setup AP's DHCP was
  intermittent and TCP dead after ~10 requests. So the CYD keeps **no decoded frame**: `display_draw_jpeg()` copies the
  JPEG (<=20KB) into a heap buffer and every draw, transition step, replay (`CMDSPIC`), half-size view
  (`CMDSSCP`, JPEGDEC `JPEG_SCALE_HALF`) and notification revert decodes it straight to the glass. Transitions
  re-decode once per step (~55-70 ms) with a mask (wipes, iris) or a brightness scale (fade) applied to each
  block in the draw callback, driven by elapsed time and always ending on a full draw, so a 600 ms effect gets
  8-13 steps and finishes within ~100 ms of the request. The iris reaches full radius at 85% of the duration
  so the corners are part of the animation. The gray tint is applied in the callback (a check-only decode pass
  decides once per new picture whether it is all gray). The QR screen pushes one row at a time. Free heap with
  WiFi up and everything running: ~144-149KB. `display_prealloc()` runs first in `setup()`; the C3 builds keep
  the 240x240 static array unchanged (`frameRow()`/`pushFrame()` helpers).
- **Auto brightness (2026-10-24, CYD only).** `light_sensor.*` reads the GPIO34 photoresistor (ADC1, 11 dB, 16-sample
  average); boards without `PIN_LDR` report no sensor and never see the option. Measured on the owner's board: **higher
  reading = darker**; a lit room and a phone flashlight both read 0 (below the ADC floor, also at 0 dB, so the bright end
  can't be told apart), the room at dusk with the lamp off ~170, the sensor covered ~700-790. `auto_brightness.*` smooths
  the reading (~2.5 s), maps it through anchors 0 -> 100%, 170 -> 50%, 650 -> 0% of the saved range, and fades the backlight
  3 levels per 50 ms with a 6-level dead-band (so it settles at about 19/250 instead of exactly 16/255). Settings:
  `autoBright` (default off), `autoMin` (16), `autoMax` (255) in NVS, on the `/settings` page only when a sensor exists.
  A live `CMDCON` calls `auto_brightness_override()` and wins until reboot; saving the settings page re-enables it.
  `display_set_contrast_auto()` never relights a screensaver-blanked backlight. `/status` now reports `contrast` (all
  boards) and `ldr` (CYD). Verified on the real CYD: covering the sensor faded 250 -> 19 in ~8 s and uncovering faded back
  up in ~12 s, no flicker in the logged levels. The anchors came from three lighting conditions only; the middle of the
  curve (indoors with the lamp on vs off) is the part most worth re-tuning.
- **Transfer LED (2026-10-27, CYD only).** `led_status.*` drives the active-low RGB LED (G=16, B=17; red is GPIO4 = panel RST,
  never touched): solid blue while a picture arrives, one 150 ms green flash on success, a cyan double flash on failure, a 15 s
  receive timeout counts as failure. Hooks: `handleLegacyPicture`/`handleColorPicture` and `protocol_ws_xfer_*` (WS and MQTT; the
  abort hook is guarded by `wsXferActive` because MQTT aborts after finishing). `transferLed` setting (NVS `s_led`, default on) on
  `/settings`. Verified on the real CYD over serial: blue/green on a good picture, blue/cyan-cyan on a truncated one. Not tested
  over WiFi/MQTT. Note a serial read gives up ~1 s after the last byte, so a failure flash is short.
- **WS transfer owner (2026-10-27).** `ws_protocol.cpp` used to abort the in-flight picture when ANY WebSocket client disconnected, so
  the menu's "Send a test picture" (or a web app tab) coming and going could kill the daemon's transfer (a cyan flash, a lost picture).
  It now remembers the client that began the transfer (`g_xferOwner`) and only that client's disconnect aborts it; binary frames from
  other clients are ignored. Tested on the CYD: client A sent half a picture, client B connected and disconnected, A finished and got
  its ack. Not A/B-tested against the old behaviour. `-DWS_DIAG=1` (opt-in) prints transfer events to the serial port.
- **0.7.0 was the first release built on the 3.x core** (Espressif 32 2026.5.50, Arduino-ESP32 3.3.8). `release.sh` needs PlatformIO's own Python (`~/.platformio/penv`) first on PATH for `esptool` (`rich_click`). The partition tables are byte-identical to 0.6.0's (NVS stays at 0x9000), so updating keeps saved settings. Flash-tested on the CYD, Sunton and Super Mini+OLED; `esp32c3_nooled` was built only.
- **Build on the current toolchain (2026-10-27).** `platform = espressif32` now resolves to Tasmota's Arduino-ESP32 3.3.8, which has
  no `WiFiClientSecure.h` and no classic `mbedtls_sha1_*`. Fixed without pinning, so 2.x and 3.x both build: `lib/WiFiClientSecureShim/`
  (inert stand-in; no wss:// is used), `ws_sha_shim.cpp` (own SHA-1 for WebSockets' `esp_sha`), `backlightBegin/Write()` in
  `display.cpp` (ledc API differs) and an `esp_sntp_*` guard in `time_manager.cpp`. All four envs build; only the CYD was run on
  hardware, and its WebSocket handshake is verified (accept key matches SHA-1, CMDHWINF reply, a CMDCORC picture acked, free heap ~152KB with WiFi up). The device name now comes from `esp_read_mac()` because `WiFi.macAddress()` is zeros before WiFi starts on core 3.x. Treat released-image claims above as for the old toolchain.
- **Touch wake (2026-10-24, CYD only).** `touch.*` reads the XPT2046 pressure (Z1/Z2 commands, software SPI on the touch
  pins so it never shares the display's or the SD card's bus), polled every 25 ms and debounced to two hits in a row. Only
  "is the screen pressed" is used, not coordinates. Measured threshold: idle reads 0 (a stray 17), a resting fingertip
  stays under ~30 (a resistive screen needs real pressure), any real press reads 2,700-6,400, so `kPressThreshold` is 250.
  `protocol_button_check()` treats a press like the BOOT button for waking (refreshes `lastWakeMs`, so the idle clock/blank
  stage ends and the picture is redrawn) and flips the AP-mode QR on a press edge, but takes NO part in the 10 s
  factory-reset hold: a palm on the screen must never wipe the device. `/status` reports `touch` (the raw pressure).
  Verified on the real CYD: blank after 5 s idle, a touch woke it in ~1.3 s and the logo came back intact, three cycles.
- **MQTT on the CYD (2026-10-24, verified).** With a throwaway `eclipse-mosquitto:2` broker (anonymous, bound to the PC's LAN
  address) and a tiny allow-listed HTTP server for the pictures, configured through the CYD's own `/mqtt-save` form: the
  CYD connected, a text notification showed a banner and reverted, and image notifications (fetched over plain HTTP into
  `colorBuf`, shown through `display_draw_jpeg_transient()`) showed and then reverted to the previous picture, which on
  the CYD is a re-decode of the stored JPEG (`g_lastJpeg`; the transient path never touches it). Confirmed by eye by the
  owner ("all fine"). Leak check: 40 notifications 3 s apart (mixed text and images, including overlapping ones) left free
  heap between 144.4 and 145.2KB, settled at the pre-burst level, largest block constant at 86,004, MQTT stayed connected,
  no reset. The rig was torn down afterwards and MQTT switched back off on the CYD.
- **Daemon with the CYD (2026-10-24; first a PC run of the repo's `t2o-wifi`, then the owner's MiSTer, see the end of this bullet).** `-list` finds the CYD through
  `_tty2oled._tcp` (name, address, fw 0.5.8). A live run against `tty2oled-4108.local` resolved the name, connected,
  and sent six core changes (7-15 KB each, all accepted) plus an unknown core, which went as a bare name; the CYD never
  reset and kept ~145KB free. With `WIFI_SEND_SETTINGS="yes"` the daemon's start-up `CMDCON,60` held the backlight at 60
  while the light sensor swung 316 -> 690, i.e. a MiSTer-driven brightness overrides auto brightness as intended; saving
  the settings page re-enables auto. Note: `-once` skips the start-up handshake, so it never sends settings (use the
  real loop to test those). One `-once` run got no mDNS answer for the `.local` name (multicast on WiFi is lossy; the
  long-running daemon caches the address), the IP address worked. **On the owner's MiSTer (same day, daemon v0.6.0 updated
  through the menu):** `HOST` holds three displays (`tty2oled-4108.local` = the CYD, B610, CD20; three is the daemon's
  maximum), all three connected and got MENU at start-up, and opening the Apple II core sent `Apple-II.jpg` (9,435 bytes) to
  all three within a second; the owner confirmed the logo looked right on every display.
- **Link fix:** `cyd_stubs.cpp` stubs `mbedtls_sha512_*`, which the framework's libmbedcrypto lacks but WebSockets'
  `esp_sha()` object references (only SHA-1 is ever used).
- **Dev WiFi shortcut (opt-in):** copy `firmware/src/wifi_dev.h.example` to `wifi_dev.h` (git-ignored), fill in your
  WiFi, and build with `PLATFORMIO_BUILD_FLAGS="-DUSE_DEV_WIFI=1" pio run -e <env> -t upload`: a device with no saved
  network then joins it at boot, in memory only (never written to NVS). It is NEVER compiled just because the header
  exists: the first version was automatic, which would have put the owner's real WiFi password into any release built on
  that machine (caught before a release was made); `tools/release.sh` now also greps every built image for the header's
  credentials and refuses to release if any is found (negative-tested with an opt-in build).
- **Verified on the real CYD:** boot splash, rotation, sysinfo, LED off, USB and WebSocket pictures with all five
  transitions (looked fine), replay and half-size, the setup AP (join, DHCP, captive page, entering the home WiFi,
  rejoin from the saved settings), and 30/30 HTTP, 15/15 WebSocket connects, 16/16 pictures over WiFi in one run.
  Backlight PWM (`CMDCON` 255..0 over WiFi) confirmed by eye: small even steps, 0 almost completely dark, 127 comfortable.
  **Not done:** the settings screen with touch. The CYD has been flashed from the live web flasher page in a browser (owner,
  after the v0.6.0 release: "web flasher works for CYD"), and runs on the owner's MiSTer. The Sunton was re-flashed with
  this code and works.
- **Pre-existing flake, found while A/B testing the Sunton:** about 1-3% of `CMDCORC` sends over USB get no `ttyack;`
  within 6 s, on the released v0.5.8 as well (1/96 vs 4/144 on the new code; TMNT with the iris failed in both).
  Not investigated.


## WiFi/AP feature — implemented and verified on real hardware (2026-08-13)

User request: AP mode with an mDNS/avahi hostname (like the sibling
`chromecast-esp32`/"KnobCast" project's `CastWebServer` pattern -
`WiFi.softAP()`, `DNSServer` wildcard-redirect captive portal,
`WebServer` on port 80, `MDNS.begin()`), plus: while in AP mode, pressing
the existing GPIO9 wake button (see "GPIO9 wake button" above) shows a
QR code on the GC9A01 for the AP's WiFi credentials instead of its normal
screensaver-wake behavior.

**Before any design work, RAM feasibility was measured on real hardware**
(not assumed) - this project's own CMDCORC bring-up history (silent
malloc failures, a heap-fragmented false-negative) made "does this even
fit" the load-bearing question, not an implementation detail. A temporary
build (`WiFi.h` + `WebServer.h` + `DNSServer.h` + `ESPmDNS.h`, reporting
`ESP.getFreeHeap()`/`getMaxAllocHeap()` over the `Serial0` debug UART -
same channel as `DBG_ENABLED`, see "Debug channel" in protocol.cpp)
found two things:

1. **As originally written, WiFi did not fit at all** - linking `WiFi.h`
   into `env:esp32c3` overflowed the DRAM segment by ~39984 bytes before
   a single byte of heap was even requested, i.e. static allocations
   alone (the two 240x240 framebuffers plus everything else) left no
   room for WiFi's own static/BSS footprint.
2. **The fade effect's `blended[DISP_W*DISP_H]` static buffer
   (`display.cpp`, ~112.5KB) was the single largest reclaimable chunk.**
   It only ever existed to hold a full-frame per-step blend of `g_frame`
   before pushing it - rewritten to blend one row at a time into a
   `uint16_t blendedRow[DISP_W]` scratch buffer and push each row inside
   one `beginBatch()/pushRectBatched()/endBatch()` span (same pattern
   already used by the iris effect), functionally identical output, just
   without the full-frame buffer. This alone dropped static RAM usage
   from ~87% to **58.4%** (191248/327680 bytes, `env:esp32c3`) -
   confirmed correct on real hardware (no seams/banding vs. the old
   full-buffer version).

With that fix in place, the full stack was measured end-to-end on real
hardware, holding the AP up continuously while also exercising the
existing serial protocol under load (a real `CMDCORC` JPEG transfer sent
over USB-CDC while WiFi/AP/WebServer/DNSServer/mDNS were all running):

| Stage | Free heap | Largest alloc block |
|---|---|---|
| Before `WiFi.mode(WIFI_AP)` | 79848–84216 | 57332–61428 |
| After `WiFi.softAP()` | 40480–44848 | 21492–26612 |
| After `WebServer.begin()` + `DNSServer.start()` + `MDNS.begin()` | 31680 | 13300 |
| Steady-state, 10s later | 31940 (stable, no leak) | 13300 |
| After a full `CMDCORC` transfer (6399-byte JPEG) with AP+server+mDNS all active | 31940 (unchanged) | 13300 |

`readExact()` completed the 6399-byte transfer in 155ms with no
slowdown, no dropped bytes, and no heap movement - the existing
`CMDCORC` pipeline is unaffected by WiFi running concurrently.
**Conclusion: WiFi/AP/mDNS/web-server is feasible on this hardware, but
only after the fade-buffer fix above** - ~31.9KB free heap / ~13.3KB
largest contiguous block is the working budget any new WiFi code has to
live within. `colorBuf` (20000 bytes) and `legacyBuf` (8192 bytes) are
already static, not heap, so they don't compete with this budget - new
WiFi-side buffers (HTTP request bodies, QR code matrix, WiFi credential
storage) should default to static/fixed-size for the same reason
(`colorBuf`'s malloc-failure history above), not `malloc`/`new`.

With that budget confirmed, the feature itself was built and verified
end-to-end on real hardware. Design decisions confirmed with the user
before implementation: **AP network is open** (no password, matching
`chromecast-esp32`'s `WiFi.softAP(ssid)`), **mDNS hostname is
MAC-suffixed** (`tty2oled-XXXX.local`, same convention as the AP SSID -
not a fixed shared name), and **GPIO9 fully splits by mode** (WiFi-QR
toggle in AP mode, existing screensaver-wake in normal operation, no new
hardware).

### New files

- `firmware/src/wifi_config.h` - `WifiConfigStore`/`WifiConfig`, NVS
  namespace `"tty2oled"` (`Preferences`), directly modeled on
  `chromecast-esp32/src/config.h`'s `ConfigStore` but trimmed to just
  `wifiSsid`/`wifiPass`/`configured` - no device-control state, that
  already lives in `protocol.cpp`/`display.cpp`.
- `firmware/src/wifi_manager.h`/`.cpp` - `WifiState` state machine
  (`INIT`/`AP_MODE`/`CONNECTING`/`CONNECTED`), directly modeled on
  `chromecast-esp32/src/main.cpp`'s `AppState` handling. No saved config
  → `AP_MODE`; saved config → `CONNECTING` with a 20s timeout (same
  constant as the reference) falling back to `AP_MODE` on failure;
  success → `CONNECTED`, which starts `MDNS.begin()` +
  `MDNS.addService("http","tcp",80)` and the web portal.
  `wifi_manager_is_ap_mode()` is protocol.cpp/display.cpp's only real
  coupling point to this module.
- `firmware/src/web_portal.h`/`.cpp` - `WebPortal` class (`WebServer(80)`
  + `DNSServer`), directly modeled on `chromecast-esp32/src/web_server.h`'s
  `CastWebServer` but trimmed to WiFi bootstrap only (no device-control
  routes): `GET /` (setup form in AP mode, status+forget page once
  connected), `POST /save` (writes config, `ESP.restart()`), `POST
  /reset` (clears config, `ESP.restart()`), `GET /status` (small
  hand-built JSON, no ArduinoJson dependency needed). HTML is an embedded
  `PROGMEM` C string, same "no LittleFS/SPIFFS" approach as the
  reference's `_handleRoot()`.
- `ricmoo/QRCode` (`lib_deps`) - small pure-C QR encoder, no dynamic
  allocation (caller-supplied fixed buffer via `qrcode_getBufferSize()`),
  used only for the AP-mode WiFi-join QR.

### Display additions (`display.cpp`)

`display_show_ap_mode(ssid, ip)`, `display_show_connecting_wifi(ssid)`,
`display_show_wifi_connected(ssid, ip)`, and `display_toggle_wifi_qr()`
(no args - remembers the AP ssid/ip set by `display_show_ap_mode()`
internally via `g_apSsid`/`g_apIp`/`g_qrShown`, so `protocol.cpp` doesn't
need to know them). The QR renderer (`drawWifiQrScreen()`, anonymous
namespace) builds the `WIFI:T:nopass;S:<ssid>;;` payload, encodes it at a
fixed version 4/ECC LOW, and reuses `g_frame` as scratch (safe to clobber
- AP mode never has a decoded CMDCORC JPEG on screen at the same time) so
no extra static buffer is needed; the whole QR is pushed in **one**
`pushRect()` call, not one `fillRect()`/SPI-transaction per module -
matching the "batch, don't do per-call transactions" lesson from the
iris/wipe stall fix earlier in this doc.

### `protocol.cpp` changes

`protocol_button_check()` gained real press-edge detection (a `static
bool wasPressed`) on top of its existing level-based `lastWakeMs` touch -
the QR toggle needs a single flip per press, not a re-trigger on every
loop() tick the button stays held. `protocol_saver_check()` now returns
early when `wifi_manager_is_ap_mode()` - no idle-blank during WiFi setup,
so the AP-status/QR screens stay up until the button is pressed or WiFi
gets configured, instead of a second idle timer fighting the QR toggle.

### Two real bugs found only by testing on actual hardware (2026-08-13)

Both exactly the kind of thing this project's CLAUDE.md keeps emphasizing
- looked correct in review, only broke on real hardware:

1. **QR square clipped by the round bezel.** The QR's pixel size was
   sized off a naive "200px safe area" constant with no relationship to
   the display actually being round - a square that size, centered, put
   its corners (exactly where the QR's finder patterns live) past the
   physical circular glass, visibly cropping them. Fixed by sizing the
   square so it's fully inscribed in a circle of conservative radius
   100px (`side <= R*sqrt(2)`, so `qrPx = floor(100*1.4142 / qrcode.size)
   * qrcode.size`) and removing a vertical offset that had been added to
   leave room for SSID text below - that offset pushed the square
   further off-center and made the clipping worse on one edge. Confirmed
   fixed on real hardware: a phone camera scanned the QR and prompted to
   join the open network correctly.
2. **`enterConnected()` never redrew the display.** On the first
   end-to-end STA-connect test, the physical screen stayed on
   "Connecting to <ssid>" indefinitely even though the device had
   actually connected in well under the 20s timeout - `WiFi.status()`
   correctly returned `WL_CONNECTED`, mDNS registered
   (`tty2oled-XXXX.local` resolved via `avahi-resolve`/`avahi-browse`),
   and `/status` served correctly over the new STA IP, all confirmed
   independently of the (stale) screen. The bug was simply that
   `enterConnected()` started mDNS/the portal but never called any
   `display_show_*` function, so the last screen drawn (`enterConnecting()`'s
   "Connecting to...") never changed. Fixed by adding
   `display_show_wifi_connected(ssid, ip)` and calling it from
   `enterConnected()`. This is a reminder that "the network side works"
   and "the screen reflects it" are two separate claims - verify both,
   not just the one that's easier to check from a laptop.

### Verified end-to-end on real hardware (2026-08-13)

- Cold boot with no saved WiFi config → device broadcasts
  `tty2oled-XXXX` (open network), captive-portal setup page loads
  automatically for any path (`onNotFound` → root), `/status` returns
  correct AP-mode JSON.
- WiFi credentials submitted through the real setup form (not curl, to
  keep the password out of any debug channel) → device restarts, joins
  the target network, `tty2oled-XXXX.local` resolves via mDNS
  (`avahi-resolve`/`avahi-browse` both confirmed the `_http._tcp`
  service advertisement), `/status` and `/` both serve correctly over
  both the direct IP and the mDNS hostname.
- GPIO9 press in AP mode → QR code renders fully inside the round bezel
  (post-fix), a real phone camera scans it and prompts to join the open
  AP network; second press reverts to the AP-status screen.
- Both `env:esp32c3` and `env:esp32c3_nooled` build clean with the
  feature included, at ~59.2-59.7% static RAM - comfortably inside the
  ~58.4% post-fade-fix baseline plus the feature's own footprint, nowhere
  near the pre-fade-fix 87% danger zone.

### Follow-ups added after initial hardware verification (2026-08-13)

- **mDNS hostname now shown on the "Connected" screen**
  (`display_show_wifi_connected()` gained a third `hostname` param,
  `<deviceName>.local`) alongside SSID/IP, so the hostname to type into a
  browser is visible without needing a laptop/mDNS tooling. Not shown in
  AP mode - mDNS isn't started until `enterConnected()`, and AP mode's
  status screen already shows the device's identity via its SSID (which
  *is* the device name).
- **WiFi network scan added to the setup form** - a "Scan for networks"
  button calls the new `GET /scan` route (`web_portal.cpp`'s
  `handleScan()`, `WiFi.scanNetworks()`, same synchronous pattern as
  `chromecast-esp32`'s `/wifiscan`), populating a `<select>` that fills
  the SSID text field on selection. Required changing `enterApMode()`
  from `WiFi.mode(WIFI_AP)` to `WIFI_AP_STA` - a pure-AP-mode scan simply
  fails, since `WiFi.scanNetworks()` needs the STA interface active even
  though STA isn't actually connecting to anything yet. Verified
  end-to-end on real hardware: scan populated real nearby SSIDs, and a
  full "scan → select → save" round trip successfully reconfigured the
  device's WiFi.

### Scope cuts (deliberate, not yet built)

- No password-protected AP - open network only, per the confirmed
  decision.
- STA-mode web portal is intentionally minimal (status + forget-WiFi) -
  this project's actual marquee control already lives in the WebSerial
  `web/` app; the new portal's only job is WiFi bootstrapping.
- `wifi_manager_status_line()` exists (meant for `oled_status.cpp`'s
  dashboard) but isn't wired in yet - the onboard OLED's 72x40 dashboard
  is already at 4 lines filling all ~40px of usable height, and adding a
  5th would mean redesigning the layout, not just appending a line.

## WiFi link quality and joining (2026-10-14)

**The ESP32's default WiFi power-save ("modem sleep") wrecked this device's
network behaviour.** Measured on a home network (Super Mini + OLED, -55 dBm)
before the fix: `ping` 47% loss and 91-407 ms round trips (avg 233 ms),
`GET /status` 0.4-3.9 s, WebSocket picture acks 1-10 s with some never
arriving - the web app showed that as a black screen between images (its
"blank first" had already cleared the display while the next picture sat
queued behind slow acks). After `WiFi.setSleep(false)` (in `enterConnected()`
and `startStaAttempt()`): 0% loss, 4-78 ms (avg 22 ms), `/status` ~30 ms, and
acks equal the effect duration plus ~0.4 s decode (instant picture: 0.15 s).
The board is USB-powered, so power-save buys nothing. `GET /status` now also
reports `rssi` (dBm) and `txpower` (dBm).

**Joining the saved network** used to be one 20 s attempt, then AP mode with
no way back until a human reset it ("I have to reset it 2-3 times"). Now
`wifi_manager.cpp` makes 3 attempts of 15 s each (`WiFi.disconnect()` between,
fresh association), and in AP mode with a saved config it reboots after 120 s
if nobody has joined the AP and the join QR isn't showing
(`AP_RETRY_SAVED_MS`). Measured: 8/8 reboots joined (2.6 s x6, ~10 s x2), none
fell to AP. Not proven gone from 8 samples.

**Why the first join sometimes fails (measured 2026-10-14, with the new
`/status` fields `join_ms`, `disc_count`, `disc_reason`):** 15 reboots on the
owner's home network: every boot joined, but 5/15 had a failed association
first (reason 204 = WPA 4-way handshake timeout, once 39), slowest join 31 s
with six drops - which the old single 20 s attempt turned into AP mode (the
"reset it 2-3 times" symptom) and the 3 x 15 s attempts now absorb. Not a
signal-level problem as such (RSSI -55..-74 dBm, i.e. a marginal uplink).
Experiments, boots with a failed first association: **11 dBm TX power 11/11**
(auth expired, joins up to 49 s, one fell to AP - lowering power makes it
*worse*, so the uplink is the limit, not self-interference; the default
~20 dBm stays); **20 MHz channel width 2/15, then 6/10 for the same build**
(`-DWIFI_FORCE_HT20=1`; no reliable effect, so off by default); default width
5/15. The same setting giving 13% then 60% says the link's own variation
dominates any knob, and what's left is physical/router side: board and case
placement (antenna), a dedicated 2.4 GHz SSID, WPA2-only, fixed channel
1/6/11. The retry logic (3 attempts, then AP mode that reboots to retry the
saved network every 120 s while idle) is the fix that matters.
`-DWIFI_TX_POWER_DBM=<n>` pins TX power before each join (testing only).

`CMDWTX,<dBm 2-20>` (diagnostic, not in the original protocol) sets WiFi TX
power live, because some ESP32-C3 Super Mini boards behave better below full
power; it wasn't needed here (default 20 dBm is fine once power-save is off)
and isn't persisted.

**Picture size limit over WiFi**: the device accepts a CMDCORC picture only up
to `COLOR_BUF_MAX` = 20,000 bytes and answers `ERR CMDCORC;` otherwise (a
44 KB JPEG, tested). The web app now fits its JPEG (`exportJpegFit()`,
quality stepped down until <= 19,000 bytes; worst-case full-screen random
noise came out at 17.7 KB), encodes before clearing the screen, and fails the
pending send on any `ERR` reply instead of waiting out the ack timeout. The
buffer is deliberately not raised: WiFi leaves ~30 KB of free heap.

## Device settings page, idle clock, NTP, tint (2026-10-14, v0.5.0)

`http://<device>/settings` (STA mode; `web_portal_settings.cpp`) edits the
boot defaults stored in NVS (`device_settings.h/.cpp`, keys `s_*`): brightness,
rotation, idle stages, status-OLED idle blank, NTP server + timezone (dropdown
of common zones + custom POSIX TZ), optional admin password (HTTP basic auth,
user `admin`; gates `/`, `/save`, `/reset`, `/mqtt-*`, `/scan`, settings; `/status`
stays open) and the tint options. MiSTer commands (`CMDCON`, `CMDROT`, `CMDSAVER`)
still override live until the next reboot.

- **Default brightness is 127** (2026-10-23; was 255, too bright on the Sunton board). Only a device
  that never saved a value (or a fresh flash) gets it; one that saved brightness on `/settings` keeps its own.
- **Idle stages** (`protocol_saver_check()`): clock after X min idle (default 15 since 2026-10-27, all boards; only a device that never saved the settings page gets it, one that saved 0 keeps 0; the clock is skipped until the time is known), then blank
  after a further Y min (0 = never); any command or the BOOT button wakes. The
  clock redraws each minute and never paints over an MQTT notification.
- **Time** (`time_manager.*`): NTP via `configTzTime`; `CMDSETTIME` is ignored
  once NTP has synced (it is local time already shifted, NTP is authoritative).
- **Tint** (`display.cpp`, `display_set_tint`/`display_new_picture`): colours the
  MiSTer's 4bpp/1bpp pictures by brightness - off / fixed / gradient (top-bottom or
  left-right) / random hue per new picture (replays keep their colour).
  Also applied (0.5.1) to a CMDCORC JPEG when every pixel is grayscale (R~=G~=B within 20),
  which is what the web app's black-and-white library images are; real colour art is
  left alone. Done once at decode into `g_frame`, so replays don't re-tint.
  Checked by a host g++ test and by eye on the GC9A01.
- `/status` also reports `ver, name, uptime, heap, heap_max, time, ntp, stage,
  mqtt, auth, crumb_prev, reset`.

### The hang: big HTTP responses stall this device (found 2026-10-14)

The first `/settings` was one 10.5 KB chunked page. Under repeated loads (and right
after a reboot) the device stopped answering HTTP, WebSocket and USB serial for
minutes while **ping kept working** (the loop task was stuck; CPU idle). Method:
an RTC_NOINIT breadcrumb (`diag.*`, `crumb(C_*)` at loop stages / handlers,
reported as `crumb_prev` + `reset` in `/status` after a reset) said "inside
the portal / settings flush"; instrumenting the send with `MSG_DONTWAIT` and a
3 s deadline showed the stall at ~5.7 KB, i.e. the lwip **send buffer (5744 B) was
full and the peer's ACKs did not come back for seconds** (curl sees the page cut at a
fixed offset). Ping still works because small ICMP frames get through; bulk TCP
from the ESP on this marginal link (RSSI -55..-70, the same environment that makes
the first WiFi join flaky) stalls, and `WiFiClient::write()` (select loop) then blocks
the whole loop and the WebServer piles up dead connections.

What did not help: one TCP write per chunk instead of three; non-blocking send with
a deadline (turns hangs into truncated pages); `TCP_NODELAY` + 512-byte segments
(worse: pbufs ate the heap, `heap_max` fell to 3.5 KB). **What fixed it: every
response must fit in the send buffer**, so `handleSettings()` now sends a ~3.2 KB
shell and `/settings.js` + `/settings-part?p=1..5` (0.5-3 KB each) are fetched by the
script one after another; the handler returns without waiting for ACKs and lwip
retransmits in the background. Soak: 3 reboot cycles x 40 full page loads (7 requests
each), 120/120 exact sizes, 15-50 ms per request, and 90/90 later. **Rule: keep any
new HTTP response under ~5 KB, never stream a big page.** The same
mechanism probably explains the earlier slow first `/status`/page loads after boot.

Debugger caveat: attaching OpenOCD/gdb over the C3's USB-JTAG and killing it uncleanly
left the chip in a state where every boot panicked with "Breakpoint" in
`semihosting_call_noerrno`; unplugging USB cleared it. It is not a firmware bug.
Also: `pio run -t upload` auto-picks a newly attached USB serial adapter (a CH340 on
`/dev/ttyUSB0` made it fail with "This chip is ESP32 not ESP32-C3"); pass
`--upload-port /dev/ttyACM0`.

## Wire protocol over WiFi (WebSocket transport, 2026-08-13)

The same `CMDxxx` command grammar the serial path speaks is now also
reachable over the network, so the web app can drive the display without
a USB cable from any machine on the LAN. Browsers can't open raw TCP
sockets (only WebSocket/HTTP), so this is a **WebSocket server on port
81** (`firmware/src/ws_protocol.h`/`.cpp`), started only once WiFi is
in the `CONNECTED` (STA) state - AP mode's only job stays WiFi
bootstrapping, matching the setup portal's existing scope cut.

**RAM was the design constraint, not an afterthought.** `arduinoWebSockets`
reassembles each binary frame into one contiguous heap allocation before
handing it to the app, and the measured largest-allocatable-block budget
(13.3KB, see the WiFi/AP feasibility table above) couldn't safely
guarantee a whole 6-15KB JPEG frame at once - the same failure class as
this project's own `malloc(40000)` bug. Rather than measure-and-hope, the
art transfer (`CMDCORC` equivalent) is **chunked at the protocol layer
from the start**: a text header frame (`CMDCORC,<name>,<effect>,
<durationMs>,<totalLength>`) followed by the JPEG split into 2048-byte
binary frames, each appended into the existing static `colorBuf`
(`protocol.cpp`) at a running offset. Every single allocation the
WebSockets library ever needs to make stays small and constant regardless
of total JPEG size - the risk is designed out, not measured around.
`web/src/wifiLink.ts`'s `sendColorArt()` does the client-side chunking to
match.

**Grammar is shared with the serial path, not duplicated.** `protocol.cpp`'s
`dispatch()` was renamed to `protocol_dispatch_line()`, moved out of its
anonymous namespace, and declared in `protocol.h` - both
`protocol_process()` (serial) and `ws_protocol.cpp`'s WS handler call it
for every command except `CMDHWINF` (writes a reply directly to `Serial`
in the shared function, so the WS handler intercepts and replies on the
socket instead) and the picture commands (each transport sources their
bytes differently). A new `protocol_note_activity()` (touches both
`lastActivityMs`/`lastWakeMs`) must be called by any transport dispatching
a command, or `oled_status.cpp`'s RX indicator lies and the screensaver
can blank mid-session under an active WiFi client - the same distinction
this project's GPIO9 button code already had to get right.

**Concurrent-transfer guard.** A WS transfer spans multiple `loop()`
iterations while chunks arrive (unlike the serial path's single blocking
`readExact()` call), so a serial `CMDCORC` landing mid-WS-transfer would
race the shared `colorBuf` without a guard. `protocol_ws_xfer_begin()`
rejects a second transfer outright, and `handleColorPicture()`/
`handleLegacyPicture()` (serial) each gained one guard line checking
`protocol_ws_xfer_in_progress()` before touching `colorBuf`/`legacyBuf` -
the only change made to either function's body.

**No legacy XBM/GSC transfer over WS** - that grammar exists for real-
MiSTer-over-serial compatibility, which doesn't apply to a WS client (the
web app only ever produces JPEG art). `WifiLink.sendLegacyPicture()`
throws rather than attempting it.

### Web app (`web/src/`)

`deviceLink.ts` defines the `DeviceLink` interface both `SerialLink`
(unchanged behavior, now `implements DeviceLink`) and the new `WifiLink`
satisfy - `main.ts`'s send/preview code only depends on this shape, not
on which transport is active. `main.ts` gained a header transport
selector (`USB (WebSerial)` / `WiFi`) plus a host field (persisted via
`localStorage`); switching transport disconnects the old one and swaps
`link` to a fresh instance, re-attaching the one "permanent" listener
(`statechange` → `renderConnectionState`) - short-lived listener
pairs like the command console's already register/deregister within
their own scope against whatever `link` was active when they opened, so
they didn't need any special handling.

### A PlatformIO/LDF gotcha worth remembering

`links2004/WebSockets` unconditionally `#include`s `<WiFiClientSecure.h>`
for the ESP32 target, even though this project never uses `wss://`.
That header is a genuine framework-bundled library (same
`framework-arduinoespressif32/libraries/` folder as `WiFi`/`Wire`/
`DNSServer`, which all resolve automatically with no `lib_deps` entry) -
but PlatformIO's LDF failed to find it **no matter where it was
`#include`d from** (this project's own source, a bare `lib_deps` entry,
`lib_ldf_mode = deep`/`deep+`) while every other bundled library kept
resolving fine. Root cause not fully explained, but the working fix was
`lib_extra_dirs = ${platformio.packages_dir}/framework-arduinoespressif32/libraries`
in `platformio.ini` - pointing LDF straight at the folder sidesteps
whatever the auto-discovery gap is for this one library.

### Verified end-to-end on real hardware (2026-08-13)

- Both `env:esp32c3`/`env:esp32c3_nooled` build clean at ~59.6-60.1%
  static RAM with the WS stack included - comfortably inside the
  ~58.4% post-fade-fix baseline, nowhere near the pre-fix 87% danger
  zone.
- A real WS client (both a Python test script and the actual `web/` app
  in a browser) connected, sent plain commands (`CMDCLS`, `CMDTXT`),
  `CMDHWINF` (replied correctly over the socket, not Serial), and
  multiple full chunked `CMDCORC` transfers back-to-back (6-8KB JPEGs) -
  all rendered correctly, `ttyack;` round-tripped every time, heap stayed
  in a healthy 20-28KB range across the whole session with no leak or
  crash.
- `MDNS.addService("ws", "tcp", 81)` (alongside the existing `"http"`
  service) confirmed advertised and resolvable via `avahi-browse`/
  `dns-sd` - **note this only makes the service discoverable to tools
  with real mDNS/Bonjour support, not to the browser-based web app
  itself**: browsers have no JavaScript API for mDNS discovery, so
  `wifiLink.ts` still requires the user to type in a host/IP either way.
- A mid-session WiFi disconnect/reconnect was observed and recovered
  from automatically (confirmed via the same real-hardware debug
  capture) - `WifiLink`'s `ws.onclose` now also resolves any pending
  `sendCommand()`/`sendColorArt()` ack wait immediately instead of
  leaving the UI looking stuck for the full 6s ack timeout.

## MQTT notifications (2026-08-13)

A third, independent input channel - text and image-by-URL notifications
pushed from Home Assistant or anything else on the local network that
speaks MQTT (`firmware/src/mqtt_client.h`/`.cpp`, `mqtt_config.h`), with
no web app or MiSTer involved. Explicitly scoped narrower than the
earlier "unknown core → search a catalog → cache" idea (see the "Follow-
up research" note under full-color marquee support below) - notifications
are one-shot and don't need a persistent cache or a real art catalog,
neither of which exist yet.

**Topics**: `<prefix>/text` (payload is the notification text verbatim)
and `<prefix>/image` (payload is a URL to fetch and show), where
`<prefix>` defaults to `tty2oled/<deviceName>` (overridable via the STA-
mode web portal's new MQTT section, `POST /mqtt-save`). Both show for a
configurable duration (default 8000ms, `MqttConfig.durationMs`) then
revert to whatever was on screen before - `protocol_redisplay_current()`,
newly exposed in `protocol.h` for this (previously `redisplayCurrent()`
was file-local in `protocol.cpp`, used only internally by the
screensaver wake path).

**Image payloads are URLs, not raw bytes** - deliberately, to avoid
reintroducing the large-contiguous-allocation risk this project already
solved for the WS command protocol: MQTT has no built-in payload
chunking the way our own `CMDCORC` grammar does, so a raw JPEG in one
MQTT publish would need `PubSubClient`'s receive buffer sized to the
whole image at once. A URL means the device does a normal HTTP GET,
streamed into the same static `colorBuf` `CMDCORC` already uses via the
same guard (`protocol_ws_xfer_begin()`/`_append()`/`_complete()`) - no
new buffer, no new allocation risk.

**No `HTTPClient` - a hand-rolled HTTP/1.1 GET instead.** `HTTPClient.h`
unconditionally references `WiFiClientSecure`'s TLS methods in its own
`.cpp` (not gated by the runtime URL scheme), and this framework's
mbedtls config has PSK cipher suites disabled - `ssl_client.cpp`'s real
implementation compiles out behind an `#if` guard that isn't satisfied,
leaving a stub with none of the symbols `WiFiClientSecure.cpp` calls,
which fails at *link* time (not the "header not found" LDF gap
`links2004/WebSockets` hit - a different, worse failure mode: it
compiles, and only fails when the linker tries to resolve
`start_ssl_client`/etc). Since this project only ever needs plain
`http://` for local-network image URLs, `mqtt_client.cpp`'s `httpGet()`
is a small hand-rolled GET over plain `WiFiClient` instead - in the same
spirit as this project's other hand-rolled wire parsing
(`protocol.cpp`'s `splitField()`). No `https://` support - a deliberate
v1 scope cut, not a limitation worth chasing given the mbedtls
reconfiguration it would require.

**The picture-buffer guard widened from two writers to three.** The WS
feature added `wsXferActive`/`protocol_ws_xfer_in_progress()` to stop a
serial `CMDCORC` racing a chunked WS transfer on `colorBuf`. MQTT's image
fetch is a third writer spanning multiple `loop()` iterations (an
`WiFiClient` stream, polled), so it claims/releases the same flag before
touching `colorBuf` too - same function names (`protocol_ws_xfer_*`,
kept despite now covering three callers, not worth renaming across every
call site for no functional change), just called from a third place.

### Two real bugs found only by testing on real hardware (2026-08-13)

1. **Image notifications never reverted - because they overwrote the
   very state they should have reverted back to.** The first
   implementation used the existing `protocol_ws_xfer_finish()` (same as
   `CMDCORC`), which decodes into the shared `g_frame` and sets
   `lastPictureKind = JPEG` - meaning after a notification image, "the
   last shown picture" *was* the notification, so
   `protocol_redisplay_current()` just redrew the notification again,
   forever. Root-caused by testing (confirmed: it never reverted, stayed
   on the fetched image). Fixed with a new `display_draw_jpeg_transient()`
   (`display.cpp`) that decodes and pushes straight to the physical
   display without touching `g_frame` at all, and a matching
   `protocol_ws_xfer_finish_transient()` that skips the
   `lastPictureKind`/`actCorename` update - so the underlying marquee
   art's `g_frame` content is never disturbed, and reverting later just
   re-reveals it unchanged. Confirmed fixed on real hardware: a real
   `CMDCORC` picture pushed first, then a notification, correctly
   reverted to the original picture afterward - both for text and image
   notifications.
2. **The first version of `display_draw_jpeg_transient()` hung the
   device outright** (confirmed via the `Serial0` debug channel: a
   `[mqtt] calling finish_transient` line printed, `finish_transient
   returned` never did, no crash-reboot banner either - a true hang, not
   a crash). Root cause: it wrapped `jpeg.decode()` in
   `beginBatch()`/`endBatch()`, holding an SPI transaction open across
   JPEGDEC's own internal per-block decode compute time, not just the
   `pushRectBatched()` calls themselves - unlike every other batched-push
   case in this file (fade, iris), which only ever wrap a loop *we*
   fully control. Fixed by pushing one `pushRect()` per JPEG block
   instead (its own transaction each), which - because JPEGDEC calls
   back once per MCU block (~15-30 times for a 240x240 image), not once
   per row - still avoids the older "per-row transaction stall" class of
   problem (see that section above) without needing to hold a transaction
   open across third-party decode timing. Confirmed fixed: the function
   now returns and the image displays correctly.

Also found and fixed in the same session: the MQTT client ID was
accidentally `"tty2oled-" + wifi_manager_device_name()`, doubling to
`tty2oled-tty2oled-B610` in the broker's connection log -
`wifi_manager_device_name()` already returns `"tty2oled-XXXX"`.

### Verified end-to-end on real hardware (2026-08-13)

Both build variants clean at ~60% static RAM (`knolleary/PubSubClient` is
small; no `lib_extra_dirs`-style LDF workaround needed since this feature
avoids `WiFiClientSecure` entirely, unlike WebSockets). Tested against a
real broker (a throwaway `eclipse-mosquitto:2` Docker container, not a
production Home Assistant instance, but a real MQTT implementation, not a
mock) - `mosquitto_pub`-equivalent publishes to `<prefix>/text` and
`<prefix>/image` both round-tripped correctly: banner/image appeared,
woke a blanked display, counted as activity, and reverted to the actual
prior marquee content (verified by pushing a real `CMDCORC` picture over
WS first, then triggering notifications, confirming revert restored that
exact picture) after the configured duration - repeated successfully
multiple times after all fixes landed.
