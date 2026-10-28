// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// Link fix for links2004/WebSockets (every board on core 3.x, and
// the classic-ESP32 CYD on any core). Its handshake calls esp_sha(SHA1, ...),
// which lives in the framework's sha_parallel_engine object. That object references mbedtls_sha1_*
// and mbedtls_sha512_*; the framework's libmbedcrypto is built without SHA-512, and newer cores
// (Arduino-ESP32 3.x, mbedtls 4) dropped the classic mbedtls_sha1_* entry points as well. Only
// SHA-1 is ever used here. So SHA-1 is a small self-contained implementation below (the library's
// own libsha1 is compiled out on ESP32), and the SHA-512 symbols are stubs that fail loudly if
// that path were ever taken. On core 2.x the C3 builds never pull esp_sha in.
#include <Arduino.h>  // ESP_ARDUINO_VERSION_MAJOR
#if defined(BOARD_CYD) || (defined(ESP_ARDUINO_VERSION_MAJOR) && ESP_ARDUINO_VERSION_MAJOR >= 3)
#include <cstddef>
#include <cstdint>
#include <cstring>

#if defined(ESP_ARDUINO_VERSION_MAJOR) && ESP_ARDUINO_VERSION_MAJOR >= 3
namespace {
// One shared context: the WebSocket handshake runs in loop(), never concurrently, and the
// caller's own mbedtls_sha1_context is never touched (its size is not ours to rely on).
struct Sha1 {
  uint32_t h[5];
  uint64_t bits;
  uint8_t buf[64];
  size_t used;
} g_sha1;

inline uint32_t rol(uint32_t v, int n) { return (v << n) | (v >> (32 - n)); }

void sha1Block(const uint8_t *p) {
  uint32_t w[80];
  for (int i = 0; i < 16; i++)
    w[i] = (uint32_t)p[i * 4] << 24 | (uint32_t)p[i * 4 + 1] << 16 | (uint32_t)p[i * 4 + 2] << 8 | p[i * 4 + 3];
  for (int i = 16; i < 80; i++) w[i] = rol(w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16], 1);
  uint32_t a = g_sha1.h[0], b = g_sha1.h[1], c = g_sha1.h[2], d = g_sha1.h[3], e = g_sha1.h[4];
  for (int i = 0; i < 80; i++) {
    uint32_t f, k;
    if (i < 20) { f = (b & c) | (~b & d); k = 0x5A827999; }
    else if (i < 40) { f = b ^ c ^ d; k = 0x6ED9EBA1; }
    else if (i < 60) { f = (b & c) | (b & d) | (c & d); k = 0x8F1BBCDC; }
    else { f = b ^ c ^ d; k = 0xCA62C1D6; }
    uint32_t t = rol(a, 5) + f + e + k + w[i];
    e = d; d = c; c = rol(b, 30); b = a; a = t;
  }
  g_sha1.h[0] += a; g_sha1.h[1] += b; g_sha1.h[2] += c; g_sha1.h[3] += d; g_sha1.h[4] += e;
}

void sha1Add(const uint8_t *in, size_t n) {
  g_sha1.bits += (uint64_t)n * 8;
  while (n) {
    size_t c = 64 - g_sha1.used;
    if (c > n) c = n;
    memcpy(g_sha1.buf + g_sha1.used, in, c);
    g_sha1.used += c; in += c; n -= c;
    if (g_sha1.used == 64) { sha1Block(g_sha1.buf); g_sha1.used = 0; }
  }
}
}  // namespace
#endif

extern "C" {
#if defined(ESP_ARDUINO_VERSION_MAJOR) && ESP_ARDUINO_VERSION_MAJOR >= 3
void mbedtls_sha1_init(void *) {}
void mbedtls_sha1_free(void *) {}
int mbedtls_sha1_starts(void *) {
  g_sha1 = {{0x67452301, 0xEFCDAB89, 0x98BADCFE, 0x10325476, 0xC3D2E1F0}, 0, {0}, 0};
  return 0;
}
int mbedtls_sha1_update(void *, const unsigned char *in, size_t n) { sha1Add(in, n); return 0; }
int mbedtls_sha1_finish(void *, unsigned char *out) {
  uint64_t bits = g_sha1.bits;
  static const uint8_t pad = 0x80, zero = 0;
  sha1Add(&pad, 1);
  while (g_sha1.used != 56) sha1Add(&zero, 1);
  uint8_t len[8];
  for (int i = 0; i < 8; i++) len[i] = (uint8_t)(bits >> (56 - 8 * i));
  sha1Add(len, 8);
  for (int i = 0; i < 20; i++) out[i] = (uint8_t)(g_sha1.h[i / 4] >> (24 - 8 * (i % 4)));
  return 0;
}
int mbedtls_sha512_starts(void *, int) { return -1; }
int mbedtls_sha512_update(void *, const unsigned char *, size_t) { return -1; }
int mbedtls_sha512_finish(void *, unsigned char *) { return -1; }
#endif

#if defined(BOARD_CYD)
void mbedtls_sha512_init(void *) {}
void mbedtls_sha512_free(void *) {}
int mbedtls_sha512_starts_ret(void *, int) { return -1; }
int mbedtls_sha512_update_ret(void *, const unsigned char *, size_t) { return -1; }
int mbedtls_sha512_finish_ret(void *, unsigned char *) { return -1; }
#endif
}
#endif
