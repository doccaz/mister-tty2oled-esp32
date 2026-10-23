#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Erico Mendonca
#
# Generates the MiSTer Downloader database for the public-domain colour art pack, so
# `update_all` can install and refresh it. It is a SECOND database next to the daemon's
# (tools/build_mister_db.py): opt-in, its own db_id, and it installs into
# /media/fat/t2o-wifi/art-pd/ - never into art/, which holds the user's own pictures
# (the Downloader would overwrite same-named files there). The daemon looks in art/
# first, then art-pd/ (mister/art.go), so user art always wins.
#
#   tools/build_art_db.py --files-dir DIR --tag art-pd-1 [--repo slug] [--db-id id] \
#       --out site/t2o-wifi-art-db.json
#
# DIR holds the release assets exactly as uploaded: art-pd_<core>.jpg for every picture,
# art-pd_ATTRIBUTIONS.md and art-pd_MANIFEST.json. The hashes and sizes come from THESE
# bytes (never a fresh render, whose JPEG bytes could differ with the machine), so what
# the Downloader fetches always matches the database. The manifest is the completeness
# check: every core it lists must be present, or this exits non-zero (a Pages deploy
# replaces the whole site, so a silently short database would be published).
import argparse
import hashlib
import json
import os
import re
import sys
import time

PREFIX = "art-pd_"
DEST_DIR = "t2o-wifi/art-pd/"
SAFE = re.compile(r"^[A-Za-z0-9._-]+$")  # GitHub rewrites other characters in asset names


def md5(path):
    h = hashlib.md5()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(65536), b""):
            h.update(block)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--files-dir", required=True, help="the release assets, as uploaded")
    ap.add_argument("--tag", required=True, help="release tag the assets live under, e.g. art-pd-1")
    ap.add_argument("--repo", default="doccaz/mister-tty2oled-esp32")
    ap.add_argument("--db-id", default="doccaz_mister_tty2oled_art_pd",
                    help="Downloader database id (also the downloader.ini section name)")
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    base = f"https://github.com/{args.repo}/releases/download/{args.tag}/"
    manifest_path = os.path.join(args.files_dir, PREFIX + "MANIFEST.json")
    if not os.path.isfile(manifest_path):
        sys.exit(f"build_art_db: missing {manifest_path}")
    cores = [m["core"] for m in json.load(open(manifest_path))]
    if not cores:
        sys.exit("build_art_db: the manifest lists no pictures")

    assets = [PREFIX + c + ".jpg" for c in cores] + [PREFIX + "ATTRIBUTIONS.md"]
    files = {}
    for asset in assets:
        if not SAFE.match(asset):
            sys.exit(f"build_art_db: '{asset}' has characters GitHub would rewrite in an asset name")
        src = os.path.join(args.files_dir, asset)
        if not os.path.isfile(src) or os.path.getsize(src) == 0:
            sys.exit(f"build_art_db: missing or empty release asset {asset}")
        files[DEST_DIR + asset[len(PREFIX):]] = {
            "hash": md5(src),
            "size": os.path.getsize(src),
            "url": base + asset,
        }

    db = {
        "v": 1,
        "db_id": args.db_id,
        "timestamp": int(time.time()),
        "files": files,
        "folders": {"t2o-wifi/": {}, DEST_DIR: {}},
    }
    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
    with open(args.out, "w") as f:
        json.dump(db, f, indent=2, sort_keys=True)
        f.write("\n")
    print(f"build_art_db: wrote {args.out} (db_id {args.db_id}, {len(cores)} pictures, {args.tag})")


if __name__ == "__main__":
    main()
