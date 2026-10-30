# Future implementations

These are directions, not commitments. Where something is partly built, it says so.

Already done and no longer on this list: running the display without a USB cable. The
[MiSTer daemon over WiFi](mister-daemon.md#mister-daemon-over-wifi) watches the same core-name file as
`tty2oled.sh`, sends the startup commands and every core change to up to three displays over
the WebSocket, converts legacy pictures to JPEG on the way, finds displays by name with its own
mDNS resolver, reconnects and replays the current core, and runs next to the stock daemon.

## Updating the firmware over WiFi

Today a new firmware needs a cable (the web flasher or PlatformIO). The hardware allows
more: the default 4 MB partition table already has two app slots, and the images fit with room
to spare, so an update could go to the idle slot and a failed upload would leave the running
firmware untouched. Not built; the design notes:

- The display has no TLS, so it can't download from GitHub itself; something on the LAN has
  to push the image (the MiSTer daemon's menu, or an upload page on the device).
- Pushing firmware is a powerful action, so it would need a gate: the admin password, or a
  short window after pressing the BOOT button (which is hard to reach inside some cases).
- Bulk HTTP from this device is known to stall on a marginal WiFi link, so receiving a
  ~700 KB image needs measuring on real hardware first. A crash-looping but valid image would
  still need a cable, and the first firmware with the receiver has to be flashed by cable.

## The public-domain art pack

The tooling to publish a downloadable pack of colour marquees is built and rehearsed but
nothing is published: a second, opt-in Downloader database installs it into
`/media/fat/t2o-wifi/art-pd/` (never into your own `art/`), and the daemon already looks
there after your own pictures and before the stock grayscale ones. It would contain only
pictures whose source logo is tagged public domain on Wikimedia Commons (about 77 of the
roughly 430 generated), with a bundled attributions file. What's missing is the decision to
publish it.

## CYD follow-ups

- **A settings screen on the touch display.** Touch currently only wakes the display; the settings
  live on the device's web page.
- **Art made for the wide screen.** The CYD shows the round-native 240×240 art centred on its
  320×240 panel. The art pipeline only renders the round version, so using the full width would
  mean a rectangular variant.
- **Upstream installer notice.** The CYD answers `HWCYD28`, which the upstream
  `MiSTer_tty2oled` `installer.sh` doesn't know yet (see [Compatibility notes](compatibility.md#compatibility-notes));
  a small upstream change like the one for `HWGC9A01C` would stop it misreporting a flash.

## Art for cores that have none

Cores with no picture just show their name. A device could fetch art on demand, but that needs a
public catalog to fetch from (none exists that's free to redistribute) and an on-device cache so a
core change doesn't pay a network round trip every time. Blocked on the catalog, not on the code.

## Known issue to chase

About 1 to 3% of picture sends over USB get no acknowledgement within a few seconds. It also
happens on v0.5.8, so it isn't new, and the cause isn't known.
