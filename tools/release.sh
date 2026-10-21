#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Erico Mendonca
#
# Release checklist: builds the three firmware variants, merges each into a
# single flash image, verifies them, tags the commit and creates the GitHub
# release with the images attached, then dispatches the Pages workflow, which
# puts these exact images on the web flasher.
#
#   tools/release.sh [--notes FILE | --generate-notes] [--dry-run] [--yes]
#
#   --notes FILE       release notes (markdown); default: let GitHub generate them
#   --generate-notes   same as the default, spelled out
#   --dry-run          do everything up to (not including) tagging/publishing
#   --yes              skip the confirmation prompts (for scripted use)
#
# Why the images are built here and not in CI: the firmware tested on real
# hardware is built with the maintainer's local PlatformIO toolchain, which a
# clean CI checkout can't reproduce (see CLAUDE.md, "Web flasher + GitHub
# Pages"). So: build here, flash and test the SAME build on a board, then run
# this. Bump FW_VERSION in firmware/src/version.h first.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

NOTES_FILE=""; DRY_RUN=0; ASSUME_YES=0
while [ $# -gt 0 ]; do
  case "$1" in
    --notes) NOTES_FILE="${2:?--notes needs a file}"; shift 2 ;;
    --generate-notes) NOTES_FILE=""; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    --yes) ASSUME_YES=1; shift ;;
    -h|--help) sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

die() { echo "error: $*" >&2; exit 1; }
# Repo-state problems stop a real release but only warn in a dry run, so the
# build/verify stages can be rehearsed on any commit.
guard() {
  if [ "${DRY_RUN}" = 1 ]; then echo "   warning (dry run): $*" >&2; else die "$*"; fi
}
confirm() {
  [ "${ASSUME_YES}" = 1 ] && return 0
  read -r -p "$1 [y/N] " ans
  [ "${ans}" = "y" ] || [ "${ans}" = "Y" ]
}

VERSION="$(sed -n 's/^#define FW_VERSION "\(.*\)"/\1/p' firmware/src/version.h)"
[ -n "${VERSION}" ] || die "could not read FW_VERSION from firmware/src/version.h"
TAG="v${VERSION}"
OUT="${ROOT}/release/${VERSION}"

echo "== Release ${TAG}"

echo "-- 1. Repository state"
command -v gh >/dev/null || die "gh (GitHub CLI) not found"
command -v go >/dev/null || die "go toolchain not found (needed to cross-compile the MiSTer daemon)"
gh auth status >/dev/null 2>&1 || die "gh isn't logged in (gh auth login)"
[ "$(git rev-parse --abbrev-ref HEAD)" = "main" ] || guard "not on main"
[ -z "$(git status --porcelain --untracked-files=no)" ] || guard "uncommitted changes to tracked files"
git fetch -q origin main --tags
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || guard "main isn't in sync with origin/main (push or pull first)"
git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null && guard "tag ${TAG} already exists - bump FW_VERSION"
gh release view "${TAG}" >/dev/null 2>&1 && guard "release ${TAG} already exists"
echo "   state checks done for ${TAG}"

echo "-- 2. Checklist"
confirm "   Bumped FW_VERSION (now ${VERSION}) and updated the docs for anything user-visible?" || die "aborted"
confirm "   Flashed THIS build to a real board and tested it (display, serial, WiFi/MQTT if touched)?" || die "aborted"

echo "-- 3. Build all variants"
PKGS="${HOME}/.platformio/packages"
ESPTOOL="$(find "${PKGS}" -path '*tool-esptoolpy*' -name esptool.py | head -1)"
BOOT_APP0="$(find "${PKGS}" -path '*framework-arduinoespressif32*' -name boot_app0.bin | head -1)"
[ -f "${ESPTOOL}" ] && [ -f "${BOOT_APP0}" ] || die "PlatformIO esptool/boot_app0 not found (run a pio build once)"
rm -rf "${OUT}"; mkdir -p "${OUT}"
ENVS=(esp32c3 esp32c3_nooled esp32c3_2424s012)
for env in "${ENVS[@]}"; do
  echo "   building ${env}"
  log="$( cd firmware && pio run -e "${env}" 2>&1 )" || { echo "${log}" | tail -20; die "build failed: ${env}"; }
  echo "${log}" | grep -E "^PLATFORM:|^ - framework-arduinoespressif32|^RAM:|^Flash:" | sed 's/^/     /'
  b="firmware/.pio/build/${env}"
  # Full-chip image, for FRESH devices only: the gaps between partitions are
  # filled with 0xFF, so writing it erases the NVS partition (saved WiFi/MQTT).
  python3 "${ESPTOOL}" --chip esp32c3 merge_bin -o "${OUT}/tty2oled-gc9a01-${VERSION}-${env}-full.bin" \
    --flash_mode dio --flash_freq 80m --flash_size 4MB \
    0x0 "${b}/bootloader.bin" 0x8000 "${b}/partitions.bin" \
    0xe000 "${BOOT_APP0}" 0x10000 "${b}/firmware.bin" >/dev/null
done

# The separate parts, written at their own offsets by the web flasher and by
# `esptool write_flash` so the NVS partition (saved settings) is left alone.
# Bootloader, partition table and boot_app0 are the same for every board.
sha() { sha256sum "$1" | cut -d' ' -f1; }
for part in bootloader partitions; do
  for env in "${ENVS[@]}"; do
    [ "$(sha firmware/.pio/build/${env}/${part}.bin)" = "$(sha firmware/.pio/build/${ENVS[0]}/${part}.bin)" ] \
      || die "${part}.bin differs between ${env} and ${ENVS[0]}; the shared-parts layout assumes they match"
  done
  cp "firmware/.pio/build/${ENVS[0]}/${part}.bin" "${OUT}/tty2oled-gc9a01-${VERSION}-${part}.bin"
done
cp "${BOOT_APP0}" "${OUT}/tty2oled-gc9a01-${VERSION}-boot_app0.bin"
for env in "${ENVS[@]}"; do
  cp "firmware/.pio/build/${env}/firmware.bin" "${OUT}/tty2oled-gc9a01-${VERSION}-${env}-app.bin"
done

echo "-- 3b. Build the MiSTer WiFi daemon"
# Pin VERSION to the tag so the binary reports it (build.sh otherwise uses git describe),
# and so its MD5 matches what -update/update_all will serve. Asset names are plain
# (t2o-wifi, S60t2o-wifi, ...) - they don't collide with the firmware's *.bin, and the
# Downloader database (tools/build_mister_db.py) maps them to their install paths.
VERSION="${TAG}" "${ROOT}/mister/build.sh" >/dev/null
cp "${ROOT}/mister/release/t2o-wifi" "${OUT}/t2o-wifi"
cp "${ROOT}/mister/S60t2o-wifi" "${ROOT}/mister/t2o-wifi.ini" "${ROOT}/mister/t2o_wifi.sh" "${OUT}/"
if command -v file >/dev/null; then
  file "${OUT}/t2o-wifi" | grep -q ARM || guard "the daemon binary isn't an ARM build"
fi
echo "   built $(stat -c %s "${OUT}/t2o-wifi")-byte daemon (version ${TAG})"

echo "-- 4. Verify images"
for env in "${ENVS[@]}"; do
  f="${OUT}/tty2oled-gc9a01-${VERSION}-${env}-full.bin"
  size="$(stat -c %s "${f}")"
  [ "${size}" -gt 500000 ] && [ "${size}" -lt 1500000 ] || die "${f}: implausible size ${size}"
  [ "$(strings -a "${f}" | grep -c -x "${VERSION}")" -ge 1 ] || die "${f}: version string ${VERSION} not found inside"
  echo "   ok: $(basename "${f}") (${size} bytes)"
done
# The parts must be exactly what the full image was built from.
for env in "${ENVS[@]}"; do
  tmp="$(mktemp)"
  python3 "${ESPTOOL}" --chip esp32c3 merge_bin -o "${tmp}" --flash_mode dio --flash_freq 80m --flash_size 4MB \
    0x0 "${OUT}/tty2oled-gc9a01-${VERSION}-bootloader.bin" 0x8000 "${OUT}/tty2oled-gc9a01-${VERSION}-partitions.bin" \
    0xe000 "${OUT}/tty2oled-gc9a01-${VERSION}-boot_app0.bin" 0x10000 "${OUT}/tty2oled-gc9a01-${VERSION}-${env}-app.bin" >/dev/null
  cmp -s "${tmp}" "${OUT}/tty2oled-gc9a01-${VERSION}-${env}-full.bin" || { rm -f "${tmp}"; die "parts don't reproduce the ${env} full image"; }
  rm -f "${tmp}"
done
echo "   ok: parts reproduce each full image"
# Rehearse the MiSTer daemon database here (so --dry-run exercises it too). The live copy
# is generated and published by tools/build_site.sh in the Pages workflow, from these same
# uploaded assets; this one just proves the generator works and the files are complete.
python3 "${ROOT}/tools/build_mister_db.py" --files-dir "${OUT}" --version "${VERSION}" --out "${OUT}/t2o-wifi-db.json"
python3 -c "import json; d=json.load(open('${OUT}/t2o-wifi-db.json')); assert d['v']==1 and len(d['files'])==4, d" \
  || die "the generated MiSTer database looks wrong"
echo "   ok: MiSTer daemon database generates and parses"
( cd "${OUT}" && sha256sum ./*.bin t2o-wifi S60t2o-wifi t2o-wifi.ini t2o_wifi.sh | sed 's# \./# #' > SHA256SUMS )
echo "   toolchain lines above should be: Espressif 32 (2024.6.0), framework 2.0.18 - if not, this isn't the tested build"

if [ "${DRY_RUN}" = 1 ]; then
  echo "== dry run: images are in ${OUT}; nothing tagged or published"
  exit 0
fi

echo "-- 5. Publish"
confirm "   Tag ${TAG} on $(git rev-parse --short HEAD) and publish the release?" || die "aborted (images kept in ${OUT})"
git tag -a "${TAG}" -m "${TAG}"
git push origin "${TAG}"
DAEMON_ASSETS=("${OUT}/t2o-wifi" "${OUT}/S60t2o-wifi" "${OUT}/t2o-wifi.ini" "${OUT}/t2o_wifi.sh")
if [ -n "${NOTES_FILE}" ]; then
  gh release create "${TAG}" --title "${TAG}" --notes-file "${NOTES_FILE}" "${OUT}"/*.bin "${OUT}/SHA256SUMS" "${DAEMON_ASSETS[@]}"
else
  gh release create "${TAG}" --title "${TAG}" --generate-notes "${OUT}"/*.bin "${OUT}/SHA256SUMS" "${DAEMON_ASSETS[@]}"
fi
# The Pages workflow must run on main (the github-pages environment rejects
# runs on a tag ref), so it's dispatched here rather than triggered by the release.
gh workflow run pages.yml -f tag="${TAG}"
echo "== Released ${TAG} and started the Pages workflow, which puts these images on the web flasher:"
echo "   gh run list --workflow pages.yml -L 1"
