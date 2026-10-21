# t2o-wifi: the MiSTer-side daemon for the WiFi display

A companion to the stock `tty2oled.sh` that talks to the GC9A01 display over WiFi
(its WebSocket port 81) instead of a USB cable. It does not replace the stock daemon and can run
next to it (for example with a USB display on the stock one). Like the stock daemon it watches
`/tmp/CORENAME` and sends the picture for the core. It is one static ARM binary
(about 2.7 MB), standard library only.

## What it sends

For each core change, the first match wins:

1. `/media/fat/tty2oled/art/<core>.jpg` - full-colour JPEGs (for example exported from the web app).
2. `pics_pri/<core>.gsc|.xbm`, then `pics/GSC_US`, `XBM_US`, `GSC`, `XBM`, `XBM_TEXT` (per the
   stock `USE_*_PICTURE` switches) - the community's 256x64 packs. The daemon converts them to a
   JPEG itself, because the display only accepts legacy pictures over USB.
3. Nothing found: the bare core name, which the display draws as text.

Like the stock daemon (and sharing its `pics/` folders), a name with no match is shortened one character at a time, `_altN`
pictures are diced (`USE_RANDOM_ALT`), and `/tmp/tty2oled_sleep` pauses it. Pictures over
19 KB are re-encoded smaller (the display's buffer is 20 KB). It reconnects with back-off and
re-sends the current core after a dropped link or a display reboot.

## Build

Needs only Go 1.21+, no MiSTer-side toolchain:

```bash
mister/build.sh        # -> mister/release/t2o-wifi
cd mister && go test ./...
```

## Install on the MiSTer

Everything lives in its own folder, `/media/fat/t2o-wifi/`; the stock `/media/fat/tty2oled/` is
only read (its ini files and `pics/` packs), never changed.

1. Copy `t2o-wifi` (from `release/`), `S60t2o-wifi` and `t2o-wifi.ini` to `/media/fat/t2o-wifi/`,
   and `t2o_wifi.sh` to `/media/fat/Scripts/`.
2. Run `t2o_wifi` from the Scripts menu (or `/media/fat/t2o-wifi/t2o-wifi -menu`): find your
   display, start the daemon, and turn on "start at boot". Or edit `t2o-wifi.ini` by hand and add
   `[[ -e /media/fat/t2o-wifi/S60t2o-wifi ]] && /media/fat/t2o-wifi/S60t2o-wifi start` to
   `/media/fat/linux/user-startup.sh`.
3. Test without the daemon: `/media/fat/t2o-wifi/t2o-wifi -once NES`. The running daemon logs to
   `/tmp/t2o-wifi.log`.

Full-colour art goes in `/media/fat/t2o-wifi/art/<core>.jpg`.

## Updating

Two ways, use either.

**Automatic, with the MiSTer's own `update_all` / `update`.** The project publishes a
[MiSTer Downloader](https://github.com/MiSTer-devel/Downloader_MiSTer) database, so the
daemon updates alongside everything else you already keep current. Add this once to the
bottom of `/media/fat/downloader.ini`:

```ini
[doccaz_mister_tty2oled_gc9a01]
db_url = https://doccaz.github.io/mister-tty2oled-gc9a01/t2o-wifi-db.json
```

From then on `update_all` keeps `t2o-wifi/t2o-wifi`, `S60t2o-wifi` and `Scripts/t2o_wifi.sh`
current (it compares MD5s, so it only downloads when something changed), and offers a reboot
after it replaces the running binary. Your `t2o-wifi.ini` is shipped once but never
overwritten (`overwrite: false`), so your `HOST` and per-display settings survive updates.
The first install still needs the one-time `t2o-wifi -menu` step to pick a display and turn
on start-at-boot - the Downloader only manages files, it can't edit `user-startup.sh` or
start the daemon.

**Manual, from the daemon itself.** `t2o-wifi -update` (or the menu's "Check for updates")
reads that same published database, and if the binary it lists differs from the running one,
downloads it, verifies its size and MD5, then swaps it in - stopping the daemon first and
restarting it after (so nothing is executing the file on `/media/fat`'s exFAT during the
swap). It downloads with `wget` (falling back to `curl`), the same tools the stock tty2oled
scripts use, rather than linking a TLS stack into the always-running binary. It updates the
binary only; the init script and menu wrapper change rarely, so for those run `update_all`
or reinstall.

**Why the name has no "tty2oled" in it:** the stock `S60tty2oled stop` finds its process with
`ps | grep tty2oled`, which would also match a binary, path or argument containing that word and
could end up killing this daemon. So the folder, binary, init script, pid file and log are all
`t2o-wifi`, and the daemon takes no command-line arguments when it starts (the stock folder is
its built-in default; `-stock` overrides it for testing).

By default it does not send contrast, rotation or screensaver, so the display keeps its own
settings-page values; `WIFI_SEND_SETTINGS="yes"` sends them from the stock ini instead.

## The menu

Run `t2o-wifi -menu`, or copy `t2o_wifi.sh` to `/media/fat/Scripts/` and start it from
the MiSTer's Scripts menu (the d-pad, A and B buttons act as the arrow keys, Enter and Esc).
It shows the display, whether it is reachable, whether the daemon runs, and the current core,
and lets you:

- find displays on the network and tick up to three, or type an address;
- start, stop and restart the daemon;
- turn "start at boot" on or off (only the WiFi daemon's own line; the stock tty2oled daemon's
  line is never touched);
- change the options (send contrast/rotation, contrast, rotation, transition, `_alt` pictures);
- check for and install a new version (see "Updating");
- send a test picture, and read the log.

It is part of the same binary (plain ANSI, no `dialog`). On a UTF-8 terminal it draws box
characters; the MiSTer's console gets ASCII lines (`TTY2OLED_ASCII=1` forces that).

## Finding the display (mDNS)

The MiSTer's libc can't resolve `.local` names, so the daemon has its own small mDNS resolver
(`mdns.go`): it asks for the name's A record from port 5353 on each network interface and
uses the answer, re-resolving at every reconnect (so a new DHCP address is followed). It
remembers the last good address in case one answer is lost. The display's responder only
answers queries sent from port 5353, which is why a plain unicast query doesn't work. Tested
against the real display from a Linux PC; a plain IP in `HOST` still works.

## Finding displays

A display advertises itself as an mDNS service `_tty2oled._tcp` (port 81, TXT `ver`, `hw`; the
instance name is the device name, `tty2oled-XXXX`). Firmware from before 0.5.3 is still found
through its `_ws._tcp` service, and a display whose record carries no version is asked over
HTTP (`GET /status`).

```bash
t2o-wifi -list      # print every display on the network (name, IP, firmware)
t2o-wifi -select    # list them, ask which (1 to 3, e.g. "1 2"), and save HOST in t2o-wifi.ini
```

`HOST="auto"` uses the display on the network if there is exactly one; with several it refuses
to guess and tells you to run `-select`.

## Several displays (up to 3)

One daemon drives up to three displays at once: list them in `HOST`, separated by commas
(`HOST="tty2oled-CD20.local,tty2oled-B610.local"`), or tick them in the menu's display picker.
More than three are ignored (the limit keeps the MiSTer's load small). Each display has its own
connection, reconnect and keepalive, so one being off does not hold up the others, and uploads
are staggered 0.4 s apart. The art is looked up and converted once per core change and sent to
every display, so they all show the same picture.

Settings are per display. A key prefixed with the display's name overrides the plain one for
that display only: the name is the host without `tty2oled-` and `.local`, in capitals, so
`tty2oled-CD20.local` gives `CD20_`, and an IP gives `192_168_86_27_`:

```
WIFI_SEND_SETTINGS="yes"
CONTRAST="100"
CD20_CONTRAST="60"      # only CD20 gets 60
B610_ROTATE="yes"       # only B610 is rotated
```

This covers `WIFI_SEND_SETTINGS`, `CONTRAST`, `ROTATE`, `SCREENSAVER*` and `TRANSITION`. The menu's
Options screen has a "Settings for" line (shown when there are several displays) that picks
"all displays" or one of them and writes the prefixed keys. `USE_RANDOM_ALT` and the stock
picture switches apply to all displays, since the art is shared. Log lines carry the display name
(`[CD20] core "NES": ...`).

## Resource use

Idle it is blocked in inotify (zero CPU) with a few MB of RAM; a core change reads one file
and sends under 20 KB. A ping every 20 s detects a dead link. The stock bash daemon spawns
`xxd` and `inotifywait` for the same work.

## Status

Unit-tested against fake displays (handshake, chunked transfer, acks, ping, art lookup, ini
parsing, inotify, two displays at once). Run on a real MiSTer next to the stock daemon, driving
real displays over WiFi: pictures, the menu with the d-pad, start at boot, and discovery. The
three-display limit and the per-display settings were tried with two displays; a third was not.
