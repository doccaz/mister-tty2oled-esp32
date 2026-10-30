# Hardware

Four boards are supported. All of them speak the same protocol and show the same
round-native 240×240 art (centred on the CYD's wider screen):

| Board | MCU | Display | Also on board | Build env | `CMDHWINF` id |
|---|---|---|---|---|---|
| [ESP32-C3 Super Mini](#discrete-build-esp32-c3-super-mini--gc9a01-module), with OLED | ESP32-C3 (RISC-V, native USB-CDC) | separate GC9A01 1.28" round, 240×240 | 0.42" SSD1306 status OLED, BOOT button | `esp32c3` | `HWGC9A01C` |
| ESP32-C3 Super Mini, no OLED | ESP32-C3 | separate GC9A01 module | BOOT button | `esp32c3_nooled` | `HWGC9A01C` |
| [Sunton ESP32-2424S012C](#all-in-one-board-sunton-esp32-2424s012c) | ESP32-C3 | 1.28" round GC9A01, soldered on | BOOT button, USB-C | `esp32c3_2424s012` | `HWGC9A01C` |
| [CYD ESP32-2432S028](#cheap-yellow-display-esp32-2432s028-cyd) | ESP32 (classic, dual core, CH340 USB-serial) | 2.8" 320×240 ILI9341 | resistive touch, light sensor, RGB LED, BOOT button, microSD slot (unused by the firmware) | `esp32_cyd` | `HWCYD28` |

The Super Mini is sold both with and without the built-in 0.42" OLED — the
firmware has a build-time flag for each variant (see
[Build & flash](flashing.md#build--flash-the-firmware)). The pin tables below are board-specific.

## Reference build: Super Mini + GC9A01 module

| Component | Detail |
|---|---|
| MCU | ESP32-C3-mini (RISC-V, WiFi, native USB-CDC) |
| Display | GC9A01 240×240 round SPI TFT |
| Status display (optional) | Onboard 0.42" SSD1306 OLED, 72×40 visible |
| Wake button | Onboard "BOOT" pushbutton (GPIO9), repurposed at runtime |

## Pins (Super Mini + GC9A01 module)

Edit `firmware/src/pins.h` to match your wiring — it's the single place
all display pins are defined. Verified working on real hardware:

| Signal | GPIO |
|---|---|
| SCLK | 4 |
| MOSI | 0 |
| CS | 7 |
| DC | 1 |
| RST | 10 |
| BL (backlight, PWM) | 3 (see the note below: the `M128-240240-RGB-7` module has no BL pin) |
| OLED SDA | 5 (fixed on boards with the onboard OLED) |
| OLED SCL | 6 (fixed on boards with the onboard OLED) |
| Wake button | 9 |

**No brightness control on this module.** The seven-pin `M128-240240-RGB-7-V1.0` GC9A01 module used in the
discrete build exposes only RST, CS, DC, SDA, SCL, GND and VCC: it has no BL/BLK pin, so its backlight is
permanently on and GPIO3 goes nowhere. The firmware still sets the PWM duty (`CMDCON` and the brightness setting
are accepted and reported), but nothing on the screen changes. The screensaver is unaffected, because it blanks
the picture itself as well. Modules that do break out a BL pin, and the Sunton and CYD boards, dim normally. If
you want dimming on this module, the backlight supply on the board would have to be cut and switched through a
small MOSFET from GPIO3 (not tried).

Avoid the C3's strapping pins (GPIO2, 8, 9 — GPIO9 is deliberately used
anyway for the wake button, safe post-boot) and whatever pins your
specific board's onboard peripherals use if you change these. The native
USB-Serial/JTAG peripheral is fixed to GPIO18/19 in hardware and is used
automatically by `Serial` — this is what both a real MiSTer and the web
app's WebSerial connection talk to; it's never wired manually. GPIO20/21
are left free for an optional hardware debug UART (see `protocol.cpp`'s
`DBG_ENABLED`).

## Discrete build: ESP32-C3 Super Mini + GC9A01 module

The reference setup behind the pin table above: an ESP32-C3 **Super Mini**
(the variant with the built-in 0.42" status OLED) wired by jumpers to a
separate GC9A01 round display module. Build it with the default
`pio run -e esp32c3 --target upload` (or `esp32c3_nooled` if your board has
no OLED).

![ESP32-C3 Super Mini wired to a GC9A01 round display, showing its WiFi "Connected" screen](hw-gc9a01-connected.jpg)

The round display on its "Connected" screen after joining WiFi (the
network name and IP are blurred out in this photo; the `.local` hostname
is the device's mDNS name).

![ESP32-C3 Super Mini's onboard OLED status dashboard, with the GC9A01 module's back and jumper wiring](hw-supermini-oled.jpg)

Close-up of the Super Mini: the onboard 0.42" OLED shows the status
dashboard (firmware version, core name, RX activity), and the back of the
GC9A01 module (marked `M128-240240-RGB-7-V1.0`, `IC:GC9A01`) shows the
jumper wiring from the pin table.

The same discrete build mounted in a 3D-printed case (the
[BambuHelper case](https://makerworld.com/en/models/3140798-bambuhelper-bambu-lab-printer-status-display-case#profileId-3546296)
from MakerWorld), cycling through a few core logos:

https://github.com/user-attachments/assets/8abdfe0f-2628-4fc1-9971-e4dd4e71f6bb

([download the mp4](case-demo.mp4), 0.3 MB)

## All-in-one board: Sunton ESP32-2424S012C

Also supported, and the simplest option: the Sunton **ESP32-2424S012C** is
a 1.28" round GC9A01 240×240 IPS display with an ESP32-C3-MINI-1U built
onto the back of it, USB-C, a reset and a BOOT button, and a battery
connector. Nothing to wire. This variant has no capacitive touch (the
firmware doesn't use touch anyway) and no status OLED.

![ESP32-2424S012C board](esp32-2424s012c.png)

The display pins are fixed by the board, and there's no reset GPIO (the
panel reset is tied to the board's reset, so the driver uses the GC9A01
software reset instead). Verified working on real hardware:

| Signal | GPIO |
|---|---|
| SCLK | 6 |
| MOSI | 7 |
| CS | 10 |
| DC | 2 |
| RST | none (software reset) |
| BL (backlight, PWM) | 3 |
| Wake button | 9 (the BOOT button) |

Build it with `pio run -e esp32c3_2424s012 --target upload`. The pins are
selected by the `BOARD_2424S012` flag in `firmware/src/pins.h`.

Verified on real hardware with this board:

- Display init and the `CMDTEST` ring pattern render correctly.
- `CMDHWINF` replies `HWGC9A01C;<version>;`.
- Backlight PWM works: `CMDCON` at 128/64/16/0 visibly dims the screen
  and `CMDCON,255` restores it.
- A full-color `CMDCORC` JPEG transfer (4917 bytes, iris effect) over USB
  decodes and displays correctly, and the firmware acks it.

Not yet tested on this board: sending art through the web app's WebSerial
path (the `CMDCORC` check above used a script speaking the same protocol),
and the WiFi/MQTT features.

## Cheap Yellow Display: ESP32-2432S028 ("CYD")

Also supported: the 2.8" **ESP32-2432S028** "Cheap Yellow Display" (a classic ESP32 with a 320×240
ILI9341 screen, resistive touch, a light sensor and an RGB LED). Nothing to wire. It shows the same
round-native art as the other displays, centred on the wider screen. Flash it from the
[web flasher](https://doccaz.github.io/mister-tty2oled-esp32/) (pick the CYD entry; its USB-serial chip
resets it into download mode by itself, so there is no BOOT-button step), or build it with
`pio run -e esp32_cyd --target upload --upload-port /dev/ttyUSB0`. Its hardware id is `HWCYD28`.

[![The CYD in a 3D-printed case showing the MiSTer logo, driven by a real MiSTer](cyd-photo.jpg)](cyd-photo.jpg)

What is different from the other boards:

- **Transfer LED** (on by default; checkbox on `/settings`): the RGB LED is solid blue while a picture is being
  received, flashes green once when it arrives and cyan twice if the transfer failed. Serial, WiFi and MQTT
  transfers all use it. Red is never used: GPIO4 is shared with the panel reset.
- **Touch wakes the display** (a firm tap; it's a resistive screen) and, in setup mode, shows the join QR.
  The BOOT button works too. Touch cannot trigger the 10-second factory reset.
- **Auto brightness:** on the device's `/settings` page, tick "Follow the room light" to let the light sensor set the
  backlight between a darkest and a brightest limit. A brightness sent by the MiSTer overrides it until the next reboot.
- It keeps no decoded picture in RAM (the classic ESP32's WiFi stack needs ~80 KB of free heap to work at all);
  it stores the JPEG and decodes it for every transition step, so transitions are a little coarser (8 to 13
  steps) than on the C3 boards.

Pins (from the board's silkscreen and verified with a probe on real hardware): display SCLK 14, MOSI 13,
MISO 12, CS 15, DC 2, backlight 21 (PWM), reset 4; touch CS 33, CLK 25, MOSI 32, MISO 39; light sensor 34;
BOOT 0. They are selected by the `BOARD_CYD` flag in `firmware/src/pins.h`.

Verified on real hardware with this board: pictures over USB and WiFi with every transition, the setup network and
saved-WiFi rejoin, backlight and auto brightness, touch wake, MQTT text and image notifications, and the
daemon driving it from a PC and from a real MiSTer (v0.6.0, together with two other displays; the Apple II logo
looked right on all three).
