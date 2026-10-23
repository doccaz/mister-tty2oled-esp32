#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Erico Mendonca
#
# Publishes the public-domain colour art pack as its own GitHub release (art-pd-N), which
# the Pages workflow turns into a second MiSTer Downloader database (t2o-wifi-art-db.json).
#
#   tools/release_art.sh [--dry-run] [--yes]
#
#   --dry-run   build the set, stage the assets and generate the database, then stop
#   --yes       skip the confirmation prompt
#
# What it does: re-checks the Commons licences (refuses to go on if anything is flagged),
# builds library/pd-set/ (tools/build_pd_set.py), stages the files under the flat asset
# names the release needs, generates the database from those exact bytes to prove it is
# complete, then tags art-pd-N, creates the release and dispatches the Pages workflow.
#
# The art release is created with --latest=false: pages.yml and `gh release view` treat
# the LATEST release as the firmware release, and a pack release taking that place would
# break the next Pages deploy and the web flasher.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"
DRY_RUN=0; ASSUME_YES=0
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --yes) ASSUME_YES=1; shift ;;
    -h|--help) sed -n '2,19p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
die() { echo "error: $*" >&2; exit 1; }
confirm() { [ "${ASSUME_YES}" = 1 ] && return 0; read -r -p "$1 [y/N] " a; [ "${a}" = y ] || [ "${a}" = Y ]; }

echo "-- 1. Licence re-check"
python3 tools/check_commons_licenses.py --write > /dev/null || die "the licence check failed to run"
echo "   written to tools/PD_LICENSE_CHECK.md ($(grep -c '| ok |' tools/PD_LICENSE_CHECK.md) ok)"

echo "-- 2. Build the public-domain set"
python3 tools/build_pd_set.py | sed 's/^/   /'
SET="${ROOT}/library/pd-set"
[ -f "${SET}/MANIFEST.json" ] || die "no manifest, the build failed"

echo "-- 3. Stage the release assets"
STAGE="$(mktemp -d)"; trap 'rm -rf "${STAGE}"' EXIT
for f in "${SET}"/*.jpg; do cp "$f" "${STAGE}/art-pd_$(basename "$f")"; done
cp "${SET}/ATTRIBUTIONS.md" "${STAGE}/art-pd_ATTRIBUTIONS.md"
cp "${SET}/MANIFEST.json" "${STAGE}/art-pd_MANIFEST.json"
N=$(( $(gh release list --limit 200 --json tagName -q '.[].tagName' 2>/dev/null | sed -n 's/^art-pd-\([0-9][0-9]*\)$/\1/p' | sort -n | tail -1 || true) + 1 ))
TAG="art-pd-${N}"
python3 tools/build_art_db.py --files-dir "${STAGE}" --tag "${TAG}" --out "${STAGE}/t2o-wifi-art-db.json"
echo "   ${TAG}: $(ls "${STAGE}"/art-pd_*.jpg | wc -l) pictures"

if [ "${DRY_RUN}" = 1 ]; then
  echo "== dry run: nothing tagged or published (staged in ${STAGE}, removed on exit)"
  exit 0
fi

echo "-- 4. Publish"
git diff --quiet && git diff --cached --quiet || die "uncommitted changes (the tag marks a commit)"
confirm "   Tag ${TAG} on $(git rev-parse --short HEAD) and publish the pack release?" || die "aborted"
git tag -a "${TAG}" -m "${TAG}: public-domain colour art pack"
git push origin "${TAG}"
gh release create "${TAG}" --latest=false --title "${TAG} (public-domain colour art)" \
  --notes "Public-domain colour logos for the t2o-wifi daemon (installed by the Downloader database, not by hand). Sources and licences are in art-pd_ATTRIBUTIONS.md; trademarks remain with their owners." \
  "${STAGE}"/art-pd_*
# the latest release must still be the firmware one
LATEST="$(gh release view --json tagName -q .tagName)"
case "${LATEST}" in art-pd-*) die "the art release became 'latest' (${LATEST}); fix it before dispatching Pages" ;; esac
gh workflow run pages.yml
echo "== Published ${TAG}; latest release is still ${LATEST}. Pages is rebuilding:"
echo "   gh run list --workflow pages.yml -L 1"
