# Flash from your browser

No toolchain needed: the **[web flasher](https://doccaz.github.io/mister-tty2oled-esp32/)**
writes the latest release to the board over USB (Chrome, Edge or Opera on
a desktop; it uses WebSerial and [ESP Web Tools](https://esphome.github.io/esp-web-tools/)).
For the ESP32-C3 boards, first hold BOOT while plugging the board in (the browser can't reset the
C3's native USB into download mode on its own; without this the page says
"Failed to initialize"), then pick your board, press "Connect and install"
and choose the serial port ("USB JTAG/serial debug unit"). The CYD needs no
BOOT step (its CH340 USB-serial chip resets it by itself); choose the CH340 /
"USB Serial" port. Flashing
keeps your saved WiFi and MQTT settings (the page writes the firmware's four
parts separately and never touches the settings area). The same site hosts the
[marquee web app](https://doccaz.github.io/mister-tty2oled-esp32/app/)
(the local art pack isn't included, it isn't redistributed).

With `esptool`, take the parts from the latest
[release](https://github.com/doccaz/mister-tty2oled-esp32/releases) and write
them at their own offsets, which also keeps your settings:

```bash
esptool.py --chip esp32c3 --port /dev/ttyACM0 write_flash \
  0x0     tty2oled-gc9a01-<ver>-bootloader.bin \
  0x8000  tty2oled-gc9a01-<ver>-partitions.bin \
  0xe000  tty2oled-gc9a01-<ver>-boot_app0.bin \
  0x10000 tty2oled-gc9a01-<ver>-<board>-app.bin   # board: esp32c3, esp32c3_nooled or esp32c3_2424s012
```

The CYD is a different chip (classic ESP32), so it has its own parts, with the bootloader at
`0x1000` instead of `0x0`:

```bash
esptool.py --chip esp32 --port /dev/ttyUSB0 write_flash \
  0x1000  tty2oled-ili9341-<ver>-bootloader.bin \
  0x8000  tty2oled-ili9341-<ver>-partitions.bin \
  0xe000  tty2oled-ili9341-<ver>-boot_app0.bin \
  0x10000 tty2oled-ili9341-<ver>-esp32_cyd-app.bin
```

The release also has one full-chip image per board (`…-<board>-full.bin` from
0.4.3 on, `…-<board>.bin` before). Those are for a brand-new device only:
they fill the space between partitions, so **writing one erases the saved
WiFi and MQTT settings** and the device returns to access-point setup.

# Build & flash the firmware

```bash
cd firmware
pio run -e esp32c3 --target upload          # default: includes the onboard OLED
pio run -e esp32c3_nooled --target upload   # board variant without the OLED
pio run -e esp32c3_2424s012 --target upload # Sunton ESP32-2424S012C all-in-one board
pio run -e esp32_cyd --target upload --upload-port /dev/ttyUSB0   # CYD ESP32-2432S028 (classic ESP32)
pio device monitor          # 115200 baud, watch for "ttyrdy;"
```

Requires [PlatformIO](https://platformio.org/) (CLI or the VS Code
extension). All dependencies (`JPEGDEC`, `Adafruit GFX Library`,
`ricmoo/QRCode`, `links2004/WebSockets`, `knolleary/PubSubClient`, plus
`U8g2` in the OLED variant) are pulled automatically via `platformio.ini`.

Maintainers: `tools/release.sh` builds the four variants, verifies them, tags
the commit, publishes the GitHub release and updates the web flasher with the
same images (see `CLAUDE.md`).
