#!/usr/bin/env python3
# Copyright (C) 2026 doccaz. GPL-3.0-or-later.
"""Build the public-domain-only colour marquee set (library/pd-set/, gitignored).

Only themes whose logo comes from a Wikimedia Commons file tagged public domain are
rendered; anything derived from the third-party tty2oled grayscale packs (the
`stock:`/pack masters) and the one CC BY-SA logo (ARCHIE) are left out. The point is to
have a set that could be published later without carrying pack art.

To make that checkable rather than assumed, each theme is rendered with its `master`
and `regions` removed and with the pack loaders disabled, so a render that would have
fallen back to pack art fails loudly. The result is then compared with the same core's
file in library/color-gen/ (identical pixels = no pack data involved).

Run tools/check_commons_licenses.py first; it refuses to build while a source is flagged
unless --allow-flagged is given. Nothing here is published; see tools/PD_LICENSE_CHECK.md.

    tools/build_pd_set.py
"""
import argparse
import hashlib
import json
import sys
from pathlib import Path

import numpy as np
from PIL import Image

sys.path.insert(0, str(Path(__file__).resolve().parent))
import colorize_library as cl  # noqa: E402

ROOT = cl.ROOT
OUT = ROOT / "library" / "pd-set"


def is_pd(theme: dict) -> bool:
    lic = theme.get("license", "").lower()
    return theme.get("source", "").startswith("http") and (
        lic.startswith("pd") or "public domain" in lic)


def _blocked(*_a, **_k):
    raise RuntimeError("pack art loader used in a public-domain build")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--allow-flagged", action="store_true",
                    help="build even though PD_LICENSE_CHECK.md has flagged files")
    args = ap.parse_args()

    check = ROOT / "tools" / "PD_LICENSE_CHECK.md"
    if not check.exists():
        sys.exit("run tools/check_commons_licenses.py --write first")
    flagged = {l.split("|")[1].strip().removeprefix("File:").replace(" ", "_")
               for l in check.read_text().splitlines()
               if l.startswith("| File:") and "| ok |" not in l}

    themes = json.loads(cl.THEMES.read_text())
    skip = cl.load_skip_list()
    picked, left_out = {}, []
    for core, th in themes.items():
        if core in skip:
            continue
        src = th.get("source", "")
        name = src.rsplit("/", 1)[-1] if src.startswith("http") else ""
        if not is_pd(th):
            left_out.append((core, "not a public-domain Commons source"))
        elif name in flagged and not args.allow_flagged:
            left_out.append((core, f"{name} flagged in PD_LICENSE_CHECK.md"))
        else:
            picked[core] = th

    # the pack loaders must not be reachable from a PD render
    cl.load_raster = _blocked
    cl.stock_master = _blocked

    OUT.mkdir(parents=True, exist_ok=True)
    for old in OUT.glob("*.jpg"):
        old.unlink()
    manifest, failed, differs = [], [], []
    for core, th in sorted(picked.items()):
        th = {k: v for k, v in th.items() if k not in ("master", "regions", "crop")}
        try:
            img = cl.render_core(core, th)
        except Exception as e:
            failed.append((core, str(e)))
            continue
        out = OUT / f"{core}.jpg"
        cl.export_jpeg(img, out)
        ref = cl.OUT_DIR / f"{core}.jpg"
        same = ref.exists() and np.array_equal(
            np.asarray(Image.open(out).convert("RGB")), np.asarray(Image.open(ref).convert("RGB")))
        if not same:
            differs.append(core)
        src = themes[core]
        manifest.append({"core": core, "source": src["source"], "license": src["license"],
                         "bytes": out.stat().st_size,
                         "sha256": hashlib.sha256(out.read_bytes()).hexdigest()})

    (OUT / "MANIFEST.json").write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n")
    lines = ["# Attributions for the public-domain set", "",
             "Logos from Wikimedia Commons files tagged public domain (re-checked, see",
             "`tools/PD_LICENSE_CHECK.md`). Trademark rights in the logos remain with their owners.", "",
             "| Core | Source | Licence |", "|---|---|---|"]
    for m in manifest:
        lines.append(f"| {m['core']} | {m['source']} | {m['license']} |")
    (OUT / "ATTRIBUTIONS.md").write_text("\n".join(lines) + "\n")

    print(f"{len(manifest)} rendered into {OUT.relative_to(ROOT)}/, "
          f"{len(left_out)} themes left out, {len(failed)} failed")
    for core, why in failed:
        print(f"  FAILED {core}: {why}")
    if differs:
        print(f"  differs from color-gen (check why): {', '.join(differs)}")
    else:
        print("  every file is pixel-identical to its color-gen render (no pack data involved)")
    print("  left out: " + ", ".join(f"{c} ({w.split(' ')[0]})" for c, w in left_out))


if __name__ == "__main__":
    main()
