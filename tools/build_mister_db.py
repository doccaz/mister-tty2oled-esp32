#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Erico Mendonca
#
# Generates the MiSTer Downloader custom database for the WiFi daemon (t2o-wifi),
# so `update` / `update_all` keep the daemon current on a user's MiSTer.
#
# The database is one plain JSON file published at a stable URL on the project's
# GitHub Pages site (tools/build_site.sh puts it there). Each file entry's `url`
# points at the matching GitHub release asset, and the MD5/size let the downloader
# (and the daemon's own `-update`) tell when a file has changed.
#
#   tools/build_mister_db.py --files-dir DIR --version X.Y.Z [--repo slug] \
#       [--db-id id] [--pages-url URL] --out site/t2o-wifi-db.json
#
# DIR must contain the four installable files: t2o-wifi (the ARM binary),
# S60t2o-wifi, t2o-wifi.ini and t2o_wifi.sh.
#
# Why plain .json and not .json.zip: the downloader accepts both, and a four-file
# database is tiny, so there's nothing to compress and no inner-filename to get
# wrong.
import argparse
import hashlib
import json
import os
import sys
import time

# Each installable file: its destination path under /media/fat (also the Downloader
# `files` key), the release-asset basename it is uploaded as, and its flags.
#   overwrite=False  ships the file once but never clobbers the user's later edits
#                    (the config ini: HOST, per-display settings).
#   reboot=True      update_all offers a reboot after replacing it (the running
#                    daemon keeps the old binary in memory until it restarts).
FILES = [
    # dest (under /media/fat)   asset basename   overwrite  reboot
    ("t2o-wifi/t2o-wifi",       "t2o-wifi",      True,      True),
    ("t2o-wifi/S60t2o-wifi",    "S60t2o-wifi",   True,      False),
    ("t2o-wifi/t2o-wifi.ini",   "t2o-wifi.ini",  False,     False),
    ("Scripts/t2o_wifi.sh",     "t2o_wifi.sh",   True,      False),
]


def md5(path):
    h = hashlib.md5()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(65536), b""):
            h.update(block)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--files-dir", required=True,
                    help="directory holding t2o-wifi, S60t2o-wifi, t2o-wifi.ini, t2o_wifi.sh")
    ap.add_argument("--version", required=True, help="release version, e.g. 0.5.4 (no leading v)")
    ap.add_argument("--repo", default="doccaz/mister-tty2oled-gc9a01",
                    help="GitHub owner/repo the release assets live under")
    ap.add_argument("--db-id", default="doccaz_mister_tty2oled_gc9a01",
                    help="Downloader database id (also the downloader.ini section name)")
    ap.add_argument("--out", required=True, help="output JSON path")
    args = ap.parse_args()

    version = args.version.lstrip("v")
    tag = "v" + version
    base = f"https://github.com/{args.repo}/releases/download/{tag}/"

    files = {}
    folders = {}
    for dest, asset, overwrite, reboot in FILES:
        src = os.path.join(args.files_dir, asset)
        if not os.path.isfile(src):
            sys.exit(f"build_mister_db: missing {src} (needed for {dest})")
        entry = {
            "hash": md5(src),
            "size": os.path.getsize(src),
            "url": base + asset,
        }
        # Defaults in the Downloader are overwrite=true, reboot=false; only emit the
        # non-default flags, to keep the database minimal and obvious.
        if not overwrite:
            entry["overwrite"] = False
        if reboot:
            entry["reboot"] = True
        files[dest] = entry
        folders[os.path.dirname(dest) + "/"] = {}

    db = {
        "v": 1,
        "db_id": args.db_id,
        "timestamp": int(time.time()),
        "files": files,
        "folders": folders,
    }

    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
    with open(args.out, "w") as f:
        json.dump(db, f, indent=2, sort_keys=True)
        f.write("\n")

    print(f"build_mister_db: wrote {args.out} (db_id {args.db_id}, {len(files)} files, {tag})")
    for dest, entry in sorted(files.items()):
        flags = []
        if entry.get("overwrite") is False:
            flags.append("keep-user-edits")
        if entry.get("reboot"):
            flags.append("reboot")
        print(f"  {dest:24} {entry['size']:>8} B  {entry['hash']}  {' '.join(flags)}")


if __name__ == "__main__":
    main()
