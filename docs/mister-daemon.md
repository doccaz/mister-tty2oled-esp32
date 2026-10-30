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
