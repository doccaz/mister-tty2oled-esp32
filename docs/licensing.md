# License

Copyright (C) 2026 Erico Mendonca.

The code in this repository (`firmware/`, `web/`, `tools/`) is licensed
under the [GNU General Public License, version 3](../LICENSE) or, at your
option, any later version (`GPL-3.0-or-later`); each source file carries
an SPDX header saying so. It was MIT-licensed until the switch to GPLv3;
copies obtained under the earlier MIT terms remain MIT.

This is compatible with the GPLv3 license of the upstream
[`MiSTer_tty2oled`](https://github.com/venice1200/MiSTer_tty2oled) project
this firmware is protocol-compatible with. The firmware is still an
independent, from-scratch reimplementation of the same wire protocol, not a
copy of that codebase.

A few things are explicitly **not** covered by that grant:

- **`reference/`** is a git submodule pointing at the upstream project
  above, which is GPLv3-licensed under its own copyright. It's included
  purely as a protocol/format reference, and its contents remain under
  their original license.
- The GC9A01 register-init sequence in `firmware/src/gc9a01.cpp` is
  adapted from TFT_eSPI's MIT-licensed `GC9A01_Init.h` (see [Why a custom
  GC9A01 driver](compatibility.md#why-a-custom-gc9a01-driver)). MIT is compatible with
  GPLv3, and that code stays attributed to its original authors.
- The community marquee art packs referenced by `tools/build_library.sh`
  (fetched from `reference/Pictures/ZIPs` via the submodule above) are
  third-party fan-made assets of unclear/unverified licensing. They are
  **not** included in this repository — `library/converted/` and
  `web/public/library/` are build output, gitignored, and only ever
  generated locally on your own machine if you choose to run that
  script.
- Third-party libraries pulled in by PlatformIO and npm keep their own
  licenses.
