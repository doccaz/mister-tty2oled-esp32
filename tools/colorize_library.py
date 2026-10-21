#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Erico Mendonca
"""
Procedural full-color marquee generator for tty2oled-esp32.

Turns a per-core *source* (a PD-textlogo SVG, or one of the existing
grayscale 4bpp masters) into a round-native 240x240 full-color JPEG, sized
for the GC9A01 and the device's CMDCORC buffer (<=19 KB, baseline JPEG).

One engine, two source loaders:
  - svg      : render a Public-Domain wordmark SVG crisp via Inkscape.
  - gradient : recolor a grayscale master through a luminance->color ramp,
               preserving its 4bpp shading so it reads dimensional.
Both loaders yield the same (luminance, alpha) pair at high resolution, then
share one gradient-map step and one compositor.

Output art is a derivative of personal-use source material and is therefore
written to library/local-extra/ (gitignored). Only THIS script and
tools/themes.json (the shareable spec, with provenance) belong in the repo.

Usage:
  tools/colorize_library.py [core ...]        # default: all cores in themes.json
  tools/colorize_library.py --contact out.png # also write a masked contact sheet
"""
import argparse
import hashlib
import io
import json
import subprocess
import sys
from pathlib import Path

import numpy as np
from PIL import Image, ImageFilter

ROOT = Path(__file__).resolve().parent.parent
THEMES = ROOT / "tools" / "themes.json"
CORES_TS = ROOT / "web" / "src" / "cores.ts"
ALIASES_TS = ROOT / "web" / "src" / "aliases.ts"
CONVERTED = ROOT / "library" / "converted"
SVG_DIR = ROOT / "library" / "color-src" / "svg"   # gitignored, fetched separately
# Generated art lands in its OWN dir (not local-extra, which is the user's
# hand-curated art) so a regen never clobbers a hand-placed file of the same
# name. build_library.sh merges both into the library.
OUT_DIR = ROOT / "library" / "color-gen"

OUT = 240          # device resolution
SS = 4             # supersample factor; render at OUT*SS then downsample
SIZE = OUT * SS
R = SIZE / 2.0     # full radius in supersampled px
MAX_BYTES = 19000  # mirrors web exportJpegFit() + device COLOR_BUF_MAX headroom


# ---------------------------------------------------------------------------
# color helpers
# ---------------------------------------------------------------------------
def hexrgb(s: str) -> np.ndarray:
    s = s.lstrip("#")
    return np.array([int(s[i : i + 2], 16) for i in (0, 2, 4)], dtype=np.float64)


def lerp(a: np.ndarray, b: np.ndarray, t: float) -> np.ndarray:
    return a + (b - a) * t


def parse_core_ids() -> list:
    import re
    return re.findall(r'id:\s*"([^"]+)"', CORES_TS.read_text())


def parse_aliases() -> dict:
    """Parse CORE_ALIASES { Core: ["Alt", ...] } from the TS source."""
    import re
    txt = ALIASES_TS.read_text()
    body = txt[txt.index("{"): txt.rindex("}") + 1]
    out = {}
    for m in re.finditer(r'(\w+)\s*:\s*\[([^\]]*)\]', body):
        out[m.group(1)] = re.findall(r'"([^"]+)"', m.group(2))
    return out


def master_index() -> dict:
    """name(lower) -> master path under converted/, preferring gsc over xbm."""
    idx = json.loads((CONVERTED / "index.json").read_text())
    best = {}
    for e in idx:
        if e.get("kind") not in ("gsc", "xbm"):
            continue
        key = e["name"].lower()
        if key not in best or (e["kind"] == "gsc" and best[key][1] != "gsc"):
            best[key] = (e["png"], e["kind"])
    return {k: v[0] for k, v in best.items()}


def resolve_master(core: str, aliases: dict, masters: dict):
    hit = masters.get(core.lower())
    if hit:
        return hit
    for alt in aliases.get(core, []):
        hit = masters.get(alt.lower())
        if hit:
            return hit
    return None


def procedural_theme(core: str) -> dict:
    """Stable hash -> hue, for cores not hand-themed."""
    h = int(hashlib.sha1(core.encode()).hexdigest(), 16)
    hue = (h % 360) / 360.0
    import colorsys

    prim = np.array(colorsys.hsv_to_rgb(hue, 0.70, 0.95)) * 255
    bg = np.array(colorsys.hsv_to_rgb(hue, 0.85, 0.14)) * 255
    return {
        "primary": "#%02x%02x%02x" % tuple(int(x) for x in prim),
        "bg": "#%02x%02x%02x" % tuple(int(x) for x in bg),
        "style": "gradient",
    }


# ---------------------------------------------------------------------------
# source loaders -> (luminance[H,W] 0..1, alpha[H,W] 0..1) at high res
# ---------------------------------------------------------------------------
def load_raster(master_rel: str):
    path = CONVERTED / master_rel
    img = Image.open(path).convert("L")
    # upscale BEFORE mapping so the ramp is smooth, not blocky
    target_w = int(SIZE * 0.90)
    target_h = max(1, round(target_w * img.height / img.width))
    img = img.resize((target_w, target_h), Image.LANCZOS)
    lum = np.asarray(img, dtype=np.float64) / 255.0
    # 4bpp masters have a black background; derive alpha from luminance with a
    # soft threshold so the logo floats on our themed bg instead of a dark box.
    alpha = np.clip((lum - 0.06) / 0.14, 0.0, 1.0)
    return lum, alpha


def load_svg(svg_name: str):
    src = SVG_DIR / svg_name
    if not src.exists():
        return None
    target_w = int(SIZE * 0.90)
    png = SVG_DIR / (src.stem + f".{target_w}.png")
    subprocess.run(
        ["inkscape", str(src), "--export-type=png",
         f"--export-filename={png}", "-w", str(target_w)],
        check=True, capture_output=True,
    )
    im = Image.open(png).convert("RGBA")
    arr = np.asarray(im, dtype=np.float64) / 255.0
    rgb, a = arr[..., :3], arr[..., 3]
    lum = (0.2126 * rgb[..., 0] + 0.7152 * rgb[..., 1] + 0.0722 * rgb[..., 2])
    return lum, a, rgb  # rgb kept for style="native"


# ---------------------------------------------------------------------------
# gradient map: luminance 0..1 -> shadow/mid/highlight ramp in the core hue
# ---------------------------------------------------------------------------
def gradient_map(lum: np.ndarray, primary: np.ndarray) -> np.ndarray:
    shadow = primary * 0.22
    mid = primary
    high = lerp(primary, np.array([255.0, 255.0, 255.0]), 0.65)
    t = lum[..., None]
    lo = lerp(shadow, mid, np.clip(t / 0.5, 0, 1))
    hi = lerp(mid, high, np.clip((t - 0.5) / 0.5, 0, 1))
    return np.where(t < 0.5, lo, hi)


# ---------------------------------------------------------------------------
# compositor
# ---------------------------------------------------------------------------
def radial_background(primary: np.ndarray, bg: np.ndarray) -> Image.Image:
    yy, xx = np.mgrid[0:SIZE, 0:SIZE]
    d = np.sqrt((xx - R) ** 2 + (yy - R) ** 2) / R
    center = lerp(bg, primary * 0.35 + bg * 0.65, 0.5)  # faint tint toward hue
    edge = bg * 0.25
    t = np.clip(d, 0, 1)[..., None]
    field = lerp(center, edge, t ** 1.4)
    circle = (np.sqrt((xx - R) ** 2 + (yy - R) ** 2) <= R)[..., None]
    field = np.where(circle, field, 0.0)  # pure black outside the glass
    return Image.fromarray(field.astype(np.uint8), "RGB")


def crop_to_content(logo_rgb, alpha):
    """Trim the blank margins the 4bpp masters (and some SVGs) carry, so the
    logo fills the safe area instead of floating small."""
    ys, xs = np.where(alpha > 0.04)
    if len(xs) == 0:
        return logo_rgb, alpha
    pad = 2
    y0, y1 = max(0, ys.min() - pad), min(alpha.shape[0], ys.max() + 1 + pad)
    x0, x1 = max(0, xs.min() - pad), min(alpha.shape[1], xs.max() + 1 + pad)
    return logo_rgb[y0:y1, x0:x1], alpha[y0:y1, x0:x1]


def paste_logo(bg_img, logo_rgb, alpha, primary):
    logo_rgb, alpha = crop_to_content(logo_rgb, alpha)
    # fit logo fully inscribed in a safe circle (corners inside), centered
    h, w = alpha.shape
    safe = R * 0.88
    scale = (safe * 0.99) / (0.5 * np.hypot(w, h))
    nw, nh = max(1, round(w * scale)), max(1, round(h * scale))

    def rs(a, mode):
        return np.asarray(
            Image.fromarray(a).resize((nw, nh), mode), dtype=np.float64
        )

    logo = rs(np.clip(logo_rgb, 0, 255).astype(np.uint8), Image.LANCZOS)
    al = rs((np.clip(alpha, 0, 1) * 255).astype(np.uint8), Image.LANCZOS) / 255.0

    layer = np.zeros((SIZE, SIZE, 3))
    amask = np.zeros((SIZE, SIZE))
    oy, ox = int(R - nh / 2), int(R - nw / 2)
    layer[oy : oy + nh, ox : ox + nw] = logo
    amask[oy : oy + nh, ox : ox + nw] = al

    rgb = np.asarray(bg_img, dtype=np.float64)

    # accent glow: blurred, hue-tinted halo behind the logo
    glow_a = Image.fromarray((amask * 255).astype(np.uint8)).filter(
        ImageFilter.GaussianBlur(SS * 4)
    )
    ga = np.asarray(glow_a, dtype=np.float64) / 255.0 * 0.45
    glow_rgb = primary[None, None, :]
    rgb = rgb * (1 - ga[..., None]) + glow_rgb * ga[..., None]

    # soft drop shadow
    sh = Image.fromarray((amask * 255).astype(np.uint8)).filter(
        ImageFilter.GaussianBlur(SS * 2)
    )
    sa = np.asarray(sh, dtype=np.float64) / 255.0 * 0.5
    sa = np.roll(np.roll(sa, SS * 2, 0), SS * 2, 1)
    rgb = rgb * (1 - sa[..., None])

    # the logo itself
    a3 = amask[..., None]
    rgb = rgb * (1 - a3) + layer * a3

    # re-mask the glass edge to black (glow can bleed past it)
    yy, xx = np.mgrid[0:SIZE, 0:SIZE]
    circle = (np.sqrt((xx - R) ** 2 + (yy - R) ** 2) <= R)[..., None]
    rgb = np.where(circle, rgb, 0.0)
    return Image.fromarray(np.clip(rgb, 0, 255).astype(np.uint8), "RGB")


def export_jpeg(img: Image.Image, path: Path) -> int:
    small = img.resize((OUT, OUT), Image.LANCZOS)
    arr = np.asarray(small, dtype=np.int16)
    # guard: device re-tints an all-grayscale JPEG; ensure we have real color
    chroma = np.abs(arr[..., 0] - arr[..., 1]) + np.abs(arr[..., 1] - arr[..., 2])
    if int((chroma > 20).sum()) < 200:
        print(f"  WARN {path.name}: nearly grayscale, device would re-tint it")
    for q in range(92, 20, -4):
        buf = io.BytesIO()
        small.save(buf, "JPEG", quality=q, optimize=True,
                   progressive=False, subsampling=0)  # 4:4:4, baseline
        if buf.tell() <= MAX_BYTES:
            path.write_bytes(buf.getvalue())
            return q
    path.write_bytes(buf.getvalue())
    return q


# ---------------------------------------------------------------------------
def render_core(core: str, theme: dict) -> Image.Image:
    primary = hexrgb(theme["primary"])
    bg = hexrgb(theme.get("bg", "#0a0a12"))
    style = theme.get("style", "gradient")

    loaded = None
    if style in ("svg", "native") and "svg" in theme:
        loaded = load_svg(theme["svg"])
        if loaded is None:
            print(f"  {core}: SVG missing, falling back to gradient-map")
            style = "gradient"

    if style in ("svg", "native") and loaded is not None:
        lum, alpha, rgb_native = loaded
        if style == "native":
            logo_rgb = rgb_native * 255
        else:
            # An SVG wordmark is flat ink, so a luminance gradient-map would
            # collapse it to the shadow stop. Recolor with a top->bottom sheen
            # in the brand hue, then modulate by the logo's OWN internal
            # luminance so multi-color knockouts (e.g. the white "Nintendo"
            # inside NES's red pill) stay distinct instead of filling solid.
            h, w = alpha.shape
            sheen = np.linspace(1.0, 0.45, h)[:, None] * np.ones((1, w))
            logo_rgb = gradient_map(sheen, primary)
            # Modulate by the logo's OWN internal luminance, but measured over
            # INK pixels only (transparent pixels carry stray RGB that would
            # skew the range). Skip entirely for a monochrome silhouette, whose
            # ink range is ~0 -- otherwise every pixel collapses to the floor.
            ink = alpha > 0.5
            if ink.any():
                lo, hi = float(lum[ink].min()), float(lum[ink].max())
                if hi - lo > 0.15:  # genuinely multi-tone ink (e.g. NES knockout)
                    norm = np.clip((lum - lo) / (hi - lo), 0, 1)
                    mod = 0.55 + 0.45 * norm
                    logo_rgb = logo_rgb * mod[..., None]
    else:
        lum, alpha = load_raster(theme["master"])
        logo_rgb = gradient_map(lum, primary)

    bg_img = radial_background(primary, bg)
    return paste_logo(bg_img, logo_rgb, alpha, primary)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("cores", nargs="*")
    ap.add_argument("--all", action="store_true",
                    help="render every core in cores.ts that has a master")
    ap.add_argument("--contact", help="write a masked contact sheet PNG here")
    args = ap.parse_args()

    themes = json.loads(THEMES.read_text())
    OUT_DIR.mkdir(parents=True, exist_ok=True)

    # build the (core -> theme) work list
    worklist = {}  # core -> theme dict
    if args.all:
        aliases, masters = parse_aliases(), master_index()
        skipped = []
        for core in parse_core_ids():
            if core in themes:
                worklist[core] = themes[core]        # keep hand-curation (incl. SVG)
                continue
            m = resolve_master(core, aliases, masters)
            if not m:
                skipped.append(core)
                continue
            theme = procedural_theme(core)
            theme["master"] = m
            worklist[core] = theme
        print(f"--all: {len(worklist)} cores resolvable, {len(skipped)} with no master")
        if skipped:
            print("  no master: " + ", ".join(skipped))
    else:
        for core in (args.cores or list(themes.keys())):
            worklist[core] = themes.get(core) or procedural_theme(core)
    cores = list(worklist.keys())

    rendered = []
    for core in cores:
        theme = worklist[core]
        try:
            img = render_core(core, theme)
        except Exception as e:
            print(f"  FAIL {core}: {e}")
            continue
        out = OUT_DIR / f"{core}.jpg"
        q = export_jpeg(img, out)
        print(f"  {core:10s} -> {out.relative_to(ROOT)}  q={q}  {out.stat().st_size}B")
        rendered.append((core, img))

    if args.contact and rendered:
        cols = min(3, len(rendered))
        rows = (len(rendered) + cols - 1) // cols
        pad, cell = 12, OUT
        sheet = Image.new("RGB", (cols * cell + (cols + 1) * pad,
                                  rows * cell + (rows + 1) * pad), (30, 30, 34))
        for i, (core, img) in enumerate(rendered):
            r, c = divmod(i, cols)
            sheet.paste(img.resize((cell, cell), Image.LANCZOS),
                        (pad + c * (cell + pad), pad + r * (cell + pad)))
        Path(args.contact).write_bytes(b"")  # ensure writable
        sheet.save(args.contact)
        print(f"  contact sheet -> {args.contact}")


if __name__ == "__main__":
    main()
