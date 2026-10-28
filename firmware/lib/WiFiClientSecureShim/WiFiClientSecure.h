// SPDX-License-Identifier: GPL-3.0-or-later
// Stand-in for the framework's WiFiClientSecure, which the current Arduino-ESP32 3.x
// package no longer ships. links2004/WebSockets includes it unconditionally on ESP32
// and only needs the type to compile; this firmware never opens a wss:// connection
// (WebSocketsServer only, plain TCP), so the TLS setters are inert.
#pragma once
#include <WiFi.h>
#include <WiFiClient.h>

class WiFiClientSecure : public WiFiClient {
  public:
    void setInsecure() {}
    void setCACert(const char *) {}
    void setCACert(const uint8_t *, size_t) {}
    void setCACertBundle(const uint8_t *) {}
    void setCACertBundle(const uint8_t *, size_t) {}
    void setCertificate(const char *) {}
    void setPrivateKey(const char *) {}
    bool verify(const char *, const char *) { return false; }
};
