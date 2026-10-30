# MiSTer daemon over WiFi

`mister/` holds `t2o-wifi`, a companion to the MiSTer's `tty2oled.sh` (it runs next to it, not instead of it) that sends the
current core's picture to the display over WiFi, so no USB cable is needed. It handles both the
community's legacy art packs (converted on the fly) and full-colour JPEGs. It can
broadcast to up to three displays at once (list them comma-separated in `HOST`), each with
its own settings, so a cabinet, a shelf and a desk can all follow the core you load. See
[`mister/README.md`](../mister/README.md) for building and installing it.

**Keeping it updated.** The daemon is published as a
[MiSTer Downloader](https://github.com/MiSTer-devel/Downloader_MiSTer) database, so
`update_all` can keep it current alongside everything else. Add this once to the bottom of
`/media/fat/downloader.ini`:

```ini
[doccaz_mister_tty2oled_gc9a01]
db_url = https://doccaz.github.io/mister-tty2oled-esp32/t2o-wifi-db.json
```

Your `t2o-wifi.ini` (`HOST` and per-display settings) is shipped once but never overwritten,
so it survives updates. Or update on demand without touching `downloader.ini`: run
`t2o-wifi -update` (or the menu's **Check for updates**), which reads the same database,
verifies the download, and swaps the binary in place. See
[`mister/README.md`](../mister/README.md#updating) for details.

## Free colour art pack

The stock tty2oled library is grayscale. For about 77 cores there is also a ready-made **colour pack**
you can install with the Downloader, opt-in and free to redistribute: every picture is rendered from a
logo that Wikimedia Commons tags as public domain (the generator is in `tools/`, nothing from the
fan-made packs is used). The pack covers the common systems (NES, SNES, MegaDrive, Game Boy line,
Atari, Commodore, MSX, Spectrum and more) plus a few arcade families (Street Fighter, King of Fighters,
Metal Slug, Mortal Kombat). Cores it doesn't cover keep their stock grayscale picture, which the display can
tint (see [settings](setup.md)).

Add this below the daemon's section in `/media/fat/downloader.ini`, then run `update_all` (or
the Downloader) once:

```ini
[doccaz_mister_tty2oled_art_pd]
db_url = https://doccaz.github.io/mister-tty2oled-esp32/t2o-wifi-art-db.json
```

The pictures land in `/media/fat/t2o-wifi/art-pd/` together with an `ATTRIBUTIONS.md` listing the
Commons source of each one. They are installed there and never into `art/`, so the Downloader can't
overwrite pictures of your own. The daemon (0.7.0 or later) picks art in this order, and the first match wins:

1. your own pictures in `t2o-wifi/art/` (`<core>.jpg`, 240×240, up to 19,000 bytes)
2. the stock tty2oled's priority folder
3. the colour pack in `t2o-wifi/art-pd/`
4. the stock grayscale pictures

The daemon looks the picture up on every core change, so there is nothing to configure: load a core
the pack has and it shows up in colour. Trademark rights in the logos stay
with their owners, the same caveat as for the grayscale pictures. The licence re-check behind the pack is in
[`tools/PD_LICENSE_CHECK.md`](../tools/PD_LICENSE_CHECK.md).

