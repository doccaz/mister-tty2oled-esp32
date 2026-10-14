#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Erico Mendonca
#
# Assembles the GitHub Pages site into ./site:
#   /              web flasher (flasher/index.html, ESP Web Tools)
#   /firmware/     one merged flash image per board + a manifest for each
#   /app/          the marquee web app (web/, Vite build)
#
# Expects the three firmware envs to be built already (pio run -e <env>),
# and `npm ci` done in web/. Run from anywhere:  tools/build_site.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SITE="${ROOT}/site"
VERSION="$(sed -n 's/^#define FW_VERSION "\(.*\)"/\1/p' "${ROOT}/firmware/src/version.h")"
[ -n "${VERSION}" ] || { echo "could not read FW_VERSION" >&2; exit 1; }

PKGS="${HOME}/.platformio/packages"
ESPTOOL="$(find "${PKGS}" -path '*tool-esptoolpy*' -name esptool.py | head -1)"
BOOT_APP0="$(find "${PKGS}" -path '*framework-arduinoespressif32*' -name boot_app0.bin | head -1)"
[ -f "${ESPTOOL}" ] && [ -f "${BOOT_APP0}" ] || { echo "PlatformIO esptool/boot_app0 not found" >&2; exit 1; }

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
  b="${ROOT}/firmware/.pio/build/${env}"
  out="tty2oled-gc9a01-${env}.bin"
  python3 "${ESPTOOL}" --chip esp32c3 merge_bin -o "${SITE}/firmware/${out}" \
    --flash_mode dio --flash_freq 80m --flash_size 4MB \
    0x0 "${b}/bootloader.bin" 0x8000 "${b}/partitions.bin" \
    0xe000 "${BOOT_APP0}" 0x10000 "${b}/firmware.bin" >/dev/null
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
