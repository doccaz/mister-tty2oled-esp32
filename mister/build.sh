#!/bin/bash
# Cross-compile the daemon for the MiSTer (32-bit ARM, static - no libc needed there).
# Needs only a Go toolchain (1.21+). Output: release/t2o-wifi
set -euo pipefail
cd "$(dirname "$0")"
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
mkdir -p release
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o release/t2o-wifi .
ls -l release/t2o-wifi
