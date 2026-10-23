#!/usr/bin/env python3
# Copyright (C) 2026 doccaz. GPL-3.0-or-later.
"""Re-check every Wikimedia Commons source in tools/themes.json against the live site.

For each distinct source file it reports the current licence, whether the file still
exists, and any licence/deletion template that is NOT a public-domain one (a CC BY-SA
box inside a "Public domain" file page, a deletion request, a copyvio tag...).
Read-only, a couple of API calls (50 titles each, spaced out, descriptive User-Agent).
This is a point-in-time check of Commons' own tagging, not legal advice; trademark
rights in the logos are untouched by it.

    tools/check_commons_licenses.py            # print the report
    tools/check_commons_licenses.py --write    # also write tools/PD_LICENSE_CHECK.md
"""
import argparse
import datetime
import json
import time
import urllib.parse
import urllib.request
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
THEMES = ROOT / "tools" / "themes.json"
UA = "mister-tty2oled-esp32 art tooling (github.com/doccaz/mister-tty2oled-esp32)"
API = "https://commons.wikimedia.org/w/api.php"
# templates that mean "not (only) public domain" or "this file is in trouble"
SUSPECT = ("cc-by", "cc-zero", "cc0", "gfdl", "copyvio", "no permission", "nsd", "npd",
           "deletion", "speedy", "fair use", "non-free", "nonfree", "dw no source", "no source")
PD_WORDS = ("public domain", "pd", "cc0", "cc-zero")


def title_of(url: str) -> str:
    return urllib.parse.unquote(url.rsplit("/", 1)[-1]).replace("_", " ")


def query(titles):
    params = {"action": "query", "format": "json", "titles": "|".join(titles),
              "prop": "imageinfo|templates", "iiprop": "extmetadata", "tllimit": "max"}
    req = urllib.request.Request(API + "?" + urllib.parse.urlencode(params),
                                 headers={"User-Agent": UA})
    return json.load(urllib.request.urlopen(req, timeout=30))["query"]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--write", action="store_true", help="write tools/PD_LICENSE_CHECK.md")
    args = ap.parse_args()

    themes = json.loads(THEMES.read_text())
    users = defaultdict(list)  # title -> [core ids]
    for core, th in themes.items():
        if th.get("source", "").startswith("http"):
            users[title_of(th["source"])].append(core)
    titles = sorted(users)

    info = {}
    for i in range(0, len(titles), 40):
        chunk = titles[i:i + 40]
        q = query(chunk)
        norm = {n["from"]: n["to"] for n in q.get("normalized", [])}
        pages = {p["title"]: p for p in q["pages"].values()}
        for t in chunk:
            info[t] = pages.get(norm.get(t, t))
        time.sleep(15)

    rows, problems = [], []
    for t in titles:
        p = info.get(t)
        if not p or "missing" in p or "imageinfo" not in p:
            rows.append((t, "MISSING", "", users[t]))
            problems.append(f"{t}: file no longer exists on Commons")
            continue
        meta = p["imageinfo"][0]["extmetadata"]
        lic = meta.get("LicenseShortName", {}).get("value", "?")
        tmpls = [x["title"].removeprefix("Template:") for x in p.get("templates", [])]
        odd = [x for x in tmpls if any(s in x.lower() for s in SUSPECT)]
        pd_ok = any(w in lic.lower() for w in PD_WORDS)
        flag = ""
        if not pd_ok:
            flag = f"licence is '{lic}'"
        if odd:
            flag = (flag + "; " if flag else "") + "templates: " + ", ".join(odd)
        rows.append((t, lic, flag, users[t]))
        if flag:
            problems.append(f"{t}: {flag}")

    today = datetime.date.today().isoformat()
    out = [f"# Commons licence re-check ({today})", "",
           "Live check of every Commons source in `themes.json` (`tools/check_commons_licenses.py`).",
           "This reflects Commons' own tagging at that moment, not legal advice; trademark rights",
           "in the logos are unaffected.", "",
           f"{len(rows)} files, {len(problems)} flagged.", "",
           "| File | Licence (Commons) | Flag | Used by |", "|---|---|---|---|"]
    for t, lic, flag, cores in rows:
        out.append(f"| {t} | {lic} | {flag or 'ok'} | {', '.join(sorted(cores))} |")
    text = "\n".join(out) + "\n"
    print(text)
    if problems:
        print("FLAGGED:\n  " + "\n  ".join(problems))
    if args.write:
        (ROOT / "tools" / "PD_LICENSE_CHECK.md").write_text(text)
        print("wrote tools/PD_LICENSE_CHECK.md")


if __name__ == "__main__":
    main()
