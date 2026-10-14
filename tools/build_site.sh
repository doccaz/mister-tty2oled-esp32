#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Erico Mendonca
#
# Assembles the GitHub Pages site into ./site:
#   /              web flasher (flasher/index.html, ESP Web Tools)
#   /firmware/     one merged flash image per board + a manifest for each
#   /app/          the marquee web app (web/, Vite build)
#
# Needs `npm ci` done in web/. Firmware images come from one of two places:
#   FIRMWARE_DIR=<dir>   merged images named *-<env>.bin, e.g. the assets of a
#                        GitHub release (what CI does, so the flasher ships the
#                        exact binaries that were tested), version taken from
#                        FIRMWARE_VERSION (default: FW_VERSION in version.h)
#   (unset)              merge them from the local PlatformIO builds
#                        (pio run -e esp32c3 / esp32c3_nooled / esp32c3_2424s012)
# Run from anywhere:  tools/build_site.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SITE="${ROOT}/site"
VERSION="${FIRMWARE_VERSION:-$(sed -n 's/^#define FW_VERSION "\(.*\)"/\1/p' "${ROOT}/firmware/src/version.h")}"
[ -n "${VERSION}" ] || { echo "could not read FW_VERSION" >&2; exit 1; }

if [ -z "${FIRMWARE_DIR:-}" ]; then
  PKGS="${HOME}/.platformio/packages"
  ESPTOOL="$(find "${PKGS}" -path '*tool-esptoolpy*' -name esptool.py | head -1)"
  BOOT_APP0="$(find "${PKGS}" -path '*framework-arduinoespressif32*' -name boot_app0.bin | head -1)"
  [ -f "${ESPTOOL}" ] && [ -f "${BOOT_APP0}" ] || { echo "PlatformIO esptool/boot_app0 not found" >&2; exit 1; }
fi

rm -rf "${SITE}"
mkdir -p "${SITE}/firmware"

# env | label shown in the flasher
BOARDS=(
  "esp32c3|ESP32-C3 Super Mini with onboard OLED + GC9A01"
  "esp32c3_nooled|ESP32-C3 Super Mini without OLED + GC9A01"
  "esp32c3_2424s012|Sunton ESP32-2424S012C all-in-one"
)

for entry in "${BOARDS[@]}"; do
  env="${entry%%|*}"; label="${entry#*|}"
  out="tty2oled-gc9a01-${env}.bin"
  if [ -n "${FIRMWARE_DIR:-}" ]; then
    # exact suffix match so "esp32c3" doesn't also catch "esp32c3_nooled"
    src="$(ls "${FIRMWARE_DIR}"/*-"${env}".bin 2>/dev/null | head -1)"
    [ -f "${src}" ] || { echo "no *-${env}.bin in ${FIRMWARE_DIR}" >&2; exit 1; }
    cp "${src}" "${SITE}/firmware/${out}"
  else
    b="${ROOT}/firmware/.pio/build/${env}"
    python3 "${ESPTOOL}" --chip esp32c3 merge_bin -o "${SITE}/firmware/${out}" \
      --flash_mode dio --flash_freq 80m --flash_size 4MB \
      0x0 "${b}/bootloader.bin" 0x8000 "${b}/partitions.bin" \
      0xe000 "${BOOT_APP0}" 0x10000 "${b}/firmware.bin" >/dev/null
  fi
  cat > "${SITE}/firmware/manifest-${env}.json" <<EOF
{
  "name": "tty2oled-gc9a01 - ${label}",
  "version": "${VERSION}",
  "new_install_prompt_erase": false,
  "builds": [
    { "chipFamily": "ESP32-C3", "parts": [ { "path": "${out}", "offset": 0 } ] }
  ]
}
EOF
done

cp "${ROOT}/flasher/index.html" "${SITE}/index.html"
sed -i "s/__FW_VERSION__/${VERSION}/g" "${SITE}/index.html"

( cd "${ROOT}/web" && npx --no-install vite build --base=./ --outDir "${SITE}/app" --emptyOutDir )

# web/public/library is generated locally from third-party fan-made art
# (gitignored, never redistributed - see CLAUDE.md). A local build picks it
# up via Vite's public/ copy; CI never has it. Strip it either way.
rm -rf "${SITE}/app/library"

touch "${SITE}/.nojekyll"
echo "site/ ready (firmware ${VERSION})"
