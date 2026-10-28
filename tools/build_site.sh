#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Erico Mendonca
#
# Assembles the GitHub Pages site into ./site:
#   /              web flasher (flasher/index.html, ESP Web Tools)
#   /firmware/     the flash parts (bootloader, partition table, boot_app0, one
#                  app image per board) + a manifest per board that writes them
#                  at their own offsets
#   /app/          the marquee web app (web/, Vite build)
#
# Needs `npm ci` done in web/. Firmware parts come from one of two places:
#   FIRMWARE_DIR=<dir>   the release assets (what CI does, so the flasher ships
#                        the exact binaries that were tested): *-bootloader.bin,
#                        *-partitions.bin, *-boot_app0.bin and *-<env>-app.bin;
#                        version from FIRMWARE_VERSION (default: FW_VERSION in
#                        version.h)
#   (unset)              the local PlatformIO builds
#                        (pio run -e esp32c3 / esp32c3_nooled / esp32c3_2424s012, and
#                        esp32_cyd for the classic-ESP32 CYD board)
#
# The CYD is a different chip (bootloader at 0x1000, chip family ESP32, its own bootloader and
# partition table), so its parts are separate (tty2oled-ili9341-*) and OPTIONAL: a release made
# before the CYD existed has none, and the site is then built without the CYD entry.
#
# Why separate parts and not one merged image: a merged image fills the gaps
# between partitions with 0xFF, which overwrites the NVS partition (0x9000) and
# so erases the saved WiFi/MQTT settings. Writing the four parts separately
# never touches it.
# Run from anywhere:  tools/build_site.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SITE="${ROOT}/site"
VERSION="${FIRMWARE_VERSION:-$(sed -n 's/^#define FW_VERSION "\(.*\)"/\1/p' "${ROOT}/firmware/src/version.h")}"
[ -n "${VERSION}" ] || { echo "could not read FW_VERSION" >&2; exit 1; }

rm -rf "${SITE}"
mkdir -p "${SITE}/firmware"

# env | label shown in the flasher
BOARDS=(
  "esp32c3|ESP32-C3 Super Mini with onboard OLED + GC9A01"
  "esp32c3_nooled|ESP32-C3 Super Mini without OLED + GC9A01"
  "esp32c3_2424s012|Sunton ESP32-2424S012C all-in-one"
)

# find <glob> in FIRMWARE_DIR, or fail loudly
pick() {
  local f; f="$(ls "${FIRMWARE_DIR}"/$1 2>/dev/null | head -1)"
  [ -f "${f}" ] || { echo "no $1 in ${FIRMWARE_DIR}" >&2; exit 1; }
  echo "${f}"
}

# The three shared parts (identical for every board).
if [ -n "${FIRMWARE_DIR:-}" ]; then
  cp "$(pick 'tty2oled-gc9a01-*-bootloader.bin')" "${SITE}/firmware/bootloader.bin"
  cp "$(pick 'tty2oled-gc9a01-*-partitions.bin')" "${SITE}/firmware/partitions.bin"
  cp "$(pick 'tty2oled-gc9a01-*-boot_app0.bin')" "${SITE}/firmware/boot_app0.bin"
else
  B0="${ROOT}/firmware/.pio/build/esp32c3"
  BOOT_APP0="$(find "${HOME}/.platformio/packages" -path '*framework-arduinoespressif32*' -name boot_app0.bin | head -1)"
  [ -f "${B0}/bootloader.bin" ] && [ -f "${BOOT_APP0}" ] || { echo "build esp32c3 first (pio run -e esp32c3)" >&2; exit 1; }
  cp "${B0}/bootloader.bin" "${SITE}/firmware/bootloader.bin"
  cp "${B0}/partitions.bin" "${SITE}/firmware/partitions.bin"
  cp "${BOOT_APP0}" "${SITE}/firmware/boot_app0.bin"
fi

for entry in "${BOARDS[@]}"; do
  env="${entry%%|*}"; label="${entry#*|}"
  app="app-${env}.bin"
  if [ -n "${FIRMWARE_DIR:-}" ]; then
    cp "$(pick "*-${env}-app.bin")" "${SITE}/firmware/${app}"
  else
    cp "${ROOT}/firmware/.pio/build/${env}/firmware.bin" "${SITE}/firmware/${app}"
  fi
  cat > "${SITE}/firmware/manifest-${env}.json" <<EOF
{
  "name": "tty2oled-gc9a01 - ${label}",
  "version": "${VERSION}",
  "new_install_prompt_erase": true,
  "builds": [
    {
      "chipFamily": "ESP32-C3",
      "parts": [
        { "path": "bootloader.bin", "offset": 0 },
        { "path": "partitions.bin", "offset": 32768 },
        { "path": "boot_app0.bin", "offset": 57344 },
        { "path": "${app}", "offset": 65536 }
      ]
    }
  ]
}
EOF
done

# ---- classic-ESP32 (CYD) ----------------------------------------------------------------
HAVE_CYD=0
if [ -n "${FIRMWARE_DIR:-}" ]; then
  ls "${FIRMWARE_DIR}"/tty2oled-ili9341-*-esp32_cyd-app.bin >/dev/null 2>&1 && HAVE_CYD=1
else
  [ -f "${ROOT}/firmware/.pio/build/esp32_cyd/firmware.bin" ] && HAVE_CYD=1
fi
if [ "${HAVE_CYD}" = 1 ]; then
  if [ -n "${FIRMWARE_DIR:-}" ]; then
    cp "$(pick 'tty2oled-ili9341-*-bootloader.bin')" "${SITE}/firmware/bootloader-esp32.bin"
    cp "$(pick 'tty2oled-ili9341-*-partitions.bin')" "${SITE}/firmware/partitions-esp32.bin"
    cp "$(pick 'tty2oled-ili9341-*-esp32_cyd-app.bin')" "${SITE}/firmware/app-esp32_cyd.bin"
  else
    CB="${ROOT}/firmware/.pio/build/esp32_cyd"
    cp "${CB}/bootloader.bin" "${SITE}/firmware/bootloader-esp32.bin"
    cp "${CB}/partitions.bin" "${SITE}/firmware/partitions-esp32.bin"
    cp "${CB}/firmware.bin" "${SITE}/firmware/app-esp32_cyd.bin"
  fi
  # boot_app0.bin is the same for every chip, so the shared copy is reused.
  cat > "${SITE}/firmware/manifest-esp32_cyd.json" <<EOF
{
  "name": "tty2oled-ili9341 - CYD ESP32-2432S028 (2.8 inch, 320x240)",
  "version": "${VERSION}",
  "new_install_prompt_erase": true,
  "builds": [
    {
      "chipFamily": "ESP32",
      "parts": [
        { "path": "bootloader-esp32.bin", "offset": 4096 },
        { "path": "partitions-esp32.bin", "offset": 32768 },
        { "path": "boot_app0.bin", "offset": 57344 },
        { "path": "app-esp32_cyd.bin", "offset": 65536 }
      ]
    }
  ]
}
EOF
else
  echo "note: no CYD (classic ESP32) images for ${VERSION}; the site is built without the CYD entry" >&2
fi

cp "${ROOT}/flasher/index.html" "${SITE}/index.html"
[ "${HAVE_CYD}" = 1 ] || sed -i '/data-cyd="1"/d' "${SITE}/index.html"
sed -i "s/__FW_VERSION__/${VERSION}/g" "${SITE}/index.html"
cp "${ROOT}/web/public/favicon.svg" "${SITE}/favicon.svg"
mkdir -p "${SITE}/img"
cp "${ROOT}/docs/hw-gc9a01-connected.jpg" "${ROOT}/docs/hw-supermini-oled.jpg" "${ROOT}/docs/case-photo.jpg" "${ROOT}/docs/cover.jpg" "${SITE}/img/"

# ---- MiSTer daemon Downloader database --------------------------------------------------
# Published at a stable URL (site root) so `update_all` and the daemon's own -update keep
# the WiFi daemon current; each entry points at the matching release asset.
DBSTAGE="$(mktemp -d)"
trap 'rm -rf "${DBSTAGE}"' EXIT
DBFILES=(t2o-wifi S60t2o-wifi t2o-wifi.ini t2o_wifi.sh)
gen_db=1
if [ -n "${FIRMWARE_DIR:-}" ]; then
  # CI: the daemon assets must be downloaded alongside the firmware, or the published
  # database would 404 for every subscriber. Fail loudly rather than quietly skip it.
  for f in "${DBFILES[@]}"; do
    [ -f "${FIRMWARE_DIR}/${f}" ] || { echo "missing daemon release asset '${f}' in ${FIRMWARE_DIR}" >&2; exit 1; }
    cp "${FIRMWARE_DIR}/${f}" "${DBSTAGE}/${f}"
  done
elif [ -f "${ROOT}/mister/release/t2o-wifi" ]; then
  # Local build: the cross-compiled binary plus the checked-in support files.
  cp "${ROOT}/mister/release/t2o-wifi" "${DBSTAGE}/t2o-wifi"
  cp "${ROOT}/mister/S60t2o-wifi" "${ROOT}/mister/t2o-wifi.ini" "${ROOT}/mister/t2o_wifi.sh" "${DBSTAGE}/"
else
  gen_db=0
  echo "note: mister/release/t2o-wifi not built; skipping the daemon database (run mister/build.sh)" >&2
fi
if [ "${gen_db}" = 1 ]; then
  python3 "${ROOT}/tools/build_mister_db.py" --files-dir "${DBSTAGE}" --version "${VERSION}" \
    --out "${SITE}/t2o-wifi-db.json"
fi

# ---- Public-domain art pack Downloader database (optional second database) ---------------
# ART_DIR holds the pack release's assets (art-pd_*) and ART_TAG its tag. Like the daemon
# database it is regenerated on EVERY deploy (a Pages deploy replaces the whole site), from
# the uploaded asset bytes, and the generator exits non-zero if any picture the manifest
# lists is missing - so a deploy never publishes a short database. With no pack release yet,
# ART_DIR is unset and the database is simply not published.
if [ -n "${ART_DIR:-}" ]; then
  [ -n "${ART_TAG:-}" ] || { echo "ART_DIR set without ART_TAG" >&2; exit 1; }
  python3 "${ROOT}/tools/build_art_db.py" --files-dir "${ART_DIR}" --tag "${ART_TAG}" \
    --out "${SITE}/t2o-wifi-art-db.json"
fi

( cd "${ROOT}/web" && npx --no-install vite build --base=./ --outDir "${SITE}/app" --emptyOutDir )

# web/public/library is generated locally from third-party fan-made art
# (gitignored, never redistributed - see CLAUDE.md). A local build picks it
# up via Vite's public/ copy; CI never has it. Strip it either way.
rm -rf "${SITE}/app/library"

touch "${SITE}/.nojekyll"
echo "site/ ready (firmware ${VERSION})"
