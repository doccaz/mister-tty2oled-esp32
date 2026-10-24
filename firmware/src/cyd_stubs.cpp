// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// Classic-ESP32 link fix. links2004/WebSockets calls esp_sha(SHA1, ...) for its handshake;
// that object also contains the SHA-384/512 paths, which reference mbedtls_sha512_*, and the
// framework's libmbedcrypto is built without SHA-512 (the C3 builds never pull esp_sha in).
// Only SHA-1 is ever used here, so these stubs exist to satisfy the linker and fail loudly
// if the SHA-512 path were ever taken.
#if defined(BOARD_CYD)
#include <cstddef>

extern "C" {
void mbedtls_sha512_init(void *) {}
void mbedtls_sha512_free(void *) {}
int mbedtls_sha512_starts_ret(void *, int) { return -1; }
int mbedtls_sha512_update_ret(void *, const unsigned char *, size_t) { return -1; }
int mbedtls_sha512_finish_ret(void *, unsigned char *) { return -1; }
}
#endif
