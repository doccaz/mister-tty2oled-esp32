// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "wifi_manager.h"
#include "wifi_config.h"
#include "mqtt_config.h"
#include "web_portal.h"
#include "ws_protocol.h"
#include "mqtt_client.h"
#include "display.h"
#include <WiFi.h>
#include <esp_wifi.h>
#include <ESPmDNS.h>

namespace {

enum class WifiState { INIT, AP_MODE, CONNECTING, CONNECTED };

WifiConfigStore configStore;
// Separate from mqtt_client.cpp's own MqttConfigStore instance - this one
// only backs the portal's settings form. Both read the same NVS
// namespace; handleMqttSave() restarts the device to apply changes
// (see its comment), so the two never need to stay in sync live.
MqttConfigStore mqttConfigStore;
WebPortal portal;
WifiState state = WifiState::INIT;
String deviceName; // "tty2oled-XXXX"
unsigned long stateEnteredMs = 0;
bool portalStarted = false;
// Real-hardware finding (2026-10-14): on this board the first join attempt
// after power-up fails often enough that the device landed in AP mode and
// needed a reset or two to reach the saved network. One 20 s try then AP is
// too brittle, so a few shorter tries (each starts a fresh association), and
// from AP mode a saved network is retried by rebooting when nobody is using
// the AP.
constexpr unsigned long WIFI_ATTEMPT_TIMEOUT_MS = 15000;
constexpr uint8_t WIFI_CONNECT_ATTEMPTS = 3;
constexpr unsigned long AP_RETRY_SAVED_MS = 120000;
uint8_t connectAttempt = 0;

// Fixed WiFi TX power in dBm, applied before every association (0 = leave the
// driver's default of ~20 dBm). Set with -DWIFI_TX_POWER_DBM=<n> to experiment
// without editing the source; see CLAUDE.md "WiFi link quality and joining".
#ifndef WIFI_TX_POWER_DBM
#define WIFI_TX_POWER_DBM 0
#endif
// -DWIFI_FORCE_HT20=1 limits the station to 20 MHz channel width (a router on a
// 40 MHz 2.4 GHz channel is a known cause of auth/handshake timeouts). OFF by
// default: on this board/network it did NOT reliably help - 15 reboots each,
// boots with a failed first association: default 5/15, 20 MHz 2/15, then the
// same 20 MHz build 6/10. The same setting giving 13% and 60% means the link's
// own variation (RSSI -55..-74 dBm) swamps it.
#ifndef WIFI_FORCE_HT20
#define WIFI_FORCE_HT20 0
#endif
void applyRadioTuning() {
  if (WIFI_FORCE_HT20) esp_wifi_set_bandwidth(WIFI_IF_STA, WIFI_BW_HT20);
  if (WIFI_TX_POWER_DBM > 0) WiFi.setTxPower((wifi_power_t)(int)(WIFI_TX_POWER_DBM * 4));
}

// Join diagnostics, reported on GET /status: how many times the station was
// dropped/refused since boot, the driver's last reason code (e.g. 201 = AP not
// found, 15 = 4-way handshake timeout, 2 = auth expired, 205 = connection
// fail, 8 = left by us), and when (ms since boot) the first IP was obtained.
// Added to find out why the first attempt after a reset sometimes fails.
volatile uint16_t discCount = 0;
volatile uint8_t lastDiscReason = 0;
volatile uint32_t gotIpMs = 0;

void onWifiEvent(arduino_event_id_t event, arduino_event_info_t info) {
  if (event == ARDUINO_EVENT_WIFI_STA_DISCONNECTED) {
    discCount = discCount + 1;
    lastDiscReason = info.wifi_sta_disconnected.reason;
  } else if (event == ARDUINO_EVENT_WIFI_STA_GOT_IP && gotIpMs == 0) {
    gotIpMs = millis();
  }
}

String buildDeviceName() {
  uint8_t mac[6];
  WiFi.macAddress(mac);
  char buf[24];
  snprintf(buf, sizeof(buf), "tty2oled-%02X%02X", mac[4], mac[5]);
  return String(buf);
}

void enterApMode() {
  // AP_STA, not plain AP: WiFi.scanNetworks() (the setup form's "Scan for
  // networks" button, see web_portal.cpp's handleScan()) needs the STA
  // interface active to scan at all - a pure AP-mode scan just fails.
  WiFi.mode(WIFI_AP_STA);
  applyRadioTuning();
  WiFi.softAP(deviceName.c_str()); // open network, confirmed with user
  // Power-save is the radio's default and wrecks latency/loss (see
  // enterConnected()); the AP_STA interface used for scanning and for the
  // setup page's "Test connection" shouldn't run with it either.
  WiFi.setSleep(false);
  delay(100);
  IPAddress apIp = WiFi.softAPIP();

  portal.begin(&configStore, true);
  portal.startCaptiveDns(apIp);
  portalStarted = true;

  display_show_ap_mode(deviceName, apIp.toString());

  state = WifiState::AP_MODE;
  stateEnteredMs = millis();
}

void startStaAttempt() {
  WiFi.setSleep(false); // see enterConnected(): power-save wrecks latency and loss on this radio
  WiFi.begin(configStore.cfg.wifiSsid, configStore.cfg.wifiPass);
  stateEnteredMs = millis();
}

void enterConnecting() {
  display_show_connecting_wifi(configStore.cfg.wifiSsid);
  WiFi.mode(WIFI_STA);
  applyRadioTuning();
  connectAttempt = 1;
  startStaAttempt();
  state = WifiState::CONNECTING;
}

void enterConnected() {
  // Real-hardware finding (2026-10-14): the ESP32's default WiFi power-save
  // ("modem sleep") buffers inbound traffic until the router's next wake
  // interval. Measured against this device on a home network: ping showed
  // 47% packet loss and 91-407 ms round trips (avg 233 ms), `GET /status`
  // took 0.4-3.9 s, and WebSocket picture acks took 1-10 s (some never
  // arrived), which the web app showed as a black screen between images.
  // The board is USB-powered, so power-save buys nothing - keep the radio on.
  WiFi.setSleep(false);
  MDNS.begin(deviceName.c_str());
  MDNS.addService("http", "tcp", 80);
  // Advertised separately from "http" so the web app's WiFi transport
  // (or avahi-browse/dns-sd) can discover the command-protocol WebSocket
  // port specifically, not just infer it from the fixed port 81 constant
  // in ws_protocol.cpp/wifiLink.ts. "ws"/"tcp" isn't an IANA-registered
  // service name, but is the common convention other WebSocket-based IoT
  // devices already use for exactly this purpose.
  MDNS.addService("ws", "tcp", 81);

  portal.begin(&configStore, false);
  mqttConfigStore.begin();
  portal.setMqttConfig(&mqttConfigStore);
  portalStarted = true;

  // Real-hardware finding (2026-08-13): the connect itself succeeded well
  // under the 20s timeout on the first end-to-end test, but the display
  // never got a corresponding redraw here - it kept showing the stale
  // "Connecting to <ssid>" text indefinitely even though WiFi/mDNS/the
  // portal were all already working. Confirmed via the mDNS/HTTP checks
  // (not the screen) before this call was added.
  display_show_wifi_connected(configStore.cfg.wifiSsid, WiFi.localIP().toString(), deviceName + ".local");

  // WS command protocol is STA-mode only (see CLAUDE.md "Wire protocol
  // over WiFi") - AP mode's only job is WiFi bootstrapping.
  ws_protocol_init();

  // Same STA-only scope cut - MQTT notifications only matter once on the
  // real network. No-ops internally if not configured/enabled.
  mqtt_client_init();

  state = WifiState::CONNECTED;
  stateEnteredMs = millis();
}

} // namespace

void wifi_manager_init() {
  WiFi.onEvent(onWifiEvent);
  configStore.begin();
  deviceName = buildDeviceName();

  if (!configStore.cfg.configured || configStore.cfg.wifiSsid[0] == '\0') {
    enterApMode();
  } else {
    enterConnecting();
  }
}

void wifi_manager_loop() {
  switch (state) {
    case WifiState::INIT:
      // wifi_manager_init() always leaves this immediately; kept only so
      // the enum has a defined starting value.
      break;

    case WifiState::AP_MODE:
      if (portalStarted) portal.loop();
      // A saved network that failed at boot (router still starting, a bad
      // moment on the radio) shouldn't strand the device in AP mode until a
      // human resets it. If nobody has joined the AP and it isn't showing the
      // join QR, reboot to try the saved network again.
      if (configStore.cfg.configured && millis() - stateEnteredMs > AP_RETRY_SAVED_MS &&
          WiFi.softAPgetStationNum() == 0 && !display_wifi_qr_shown()) {
        ESP.restart();
      }
      break;

    case WifiState::CONNECTING:
      if (WiFi.status() == WL_CONNECTED) {
        enterConnected();
      } else if (millis() - stateEnteredMs > WIFI_ATTEMPT_TIMEOUT_MS) {
        if (connectAttempt < WIFI_CONNECT_ATTEMPTS) {
          connectAttempt++;
          WiFi.disconnect(false, false); // drop the half-open association before trying again
          startStaAttempt();
        } else {
          enterApMode();
        }
      }
      break;

    case WifiState::CONNECTED:
      if (portalStarted) portal.loop();
      break;
  }
}

void wifi_manager_set_tx_power(float dbm) {
  if (dbm < 2) dbm = 2;
  if (dbm > 20) dbm = 20;
  WiFi.setTxPower((wifi_power_t)(int)(dbm * 4)); // the driver counts quarter-dBm
}

String wifi_manager_diag_json() {
  return "\"join_ms\":" + String((uint32_t)gotIpMs) + ",\"disc_count\":" + String((uint16_t)discCount) +
         ",\"disc_reason\":" + String((uint8_t)lastDiscReason);
}

float wifi_manager_tx_power() {
  return WiFi.getTxPower() / 4.0f;
}

bool wifi_manager_is_ap_mode() {
  return state == WifiState::AP_MODE;
}

bool wifi_manager_is_connected() {
  return state == WifiState::CONNECTED;
}

void wifi_manager_factory_reset() {
  configStore.reset();
  // A local, separate Preferences instance rather than reusing the
  // file-scope mqttConfigStore - that one may already be begin()'d (see
  // enterConnected()), and NVS handles are cheap/independent, so this
  // avoids reasoning about double-begin() on the shared instance.
  MqttConfigStore mqttConfig;
  mqttConfig.begin();
  mqttConfig.reset();
  delay(500); // let display_show_factory_reset_done() actually show before restarting
  ESP.restart();
}

// Redraws whatever this module considers "the current screen" - used by
// protocol.cpp's long-press-cancel path to undo a factory-reset countdown
// that didn't reach the full hold. Only AP_MODE has a screen protocol.cpp
// doesn't already know how to restore itself (protocol_redisplay_current()
// covers CONNECTED's marquee/corename content); CONNECTING's screen is
// static text with nothing to preserve either way.
void wifi_manager_redraw_screen() {
  if (state == WifiState::AP_MODE) {
    display_show_ap_mode(deviceName, WiFi.softAPIP().toString());
  }
}

bool wifi_manager_test_connection(const String &ssid, const String &pass, String &errorOut) {
  if (ssid.length() == 0) {
    errorOut = "No SSID given";
    return false;
  }

  // Real-hardware finding (2026-10-14): this used to wait 10 s and report
  // "Timed out connecting" for a network the device joins fine at boot -
  // joins here take 3-10 s, and the AP_STA radio is slower than plain STA.
  // So: a clean start, up to 20 s with one fresh attempt halfway (like the
  // boot-time retries), and an immediate answer when the password is rejected.
  constexpr unsigned long kTotalMs = 20000;
  constexpr unsigned long kRetryAtMs = 10000;
  WiFi.setSleep(false);
  WiFi.disconnect(false, false);
  WiFi.begin(ssid.c_str(), pass.c_str());
  unsigned long start = millis();
  bool retried = false;
  wl_status_t status;
  while ((status = WiFi.status()) != WL_CONNECTED && millis() - start < kTotalMs) {
    // Wrong password / auth rejected - waiting won't help. Ignored for the first
    // 1.5 s: begin() can leave the previous attempt's status in place briefly.
    if (status == WL_CONNECT_FAILED && millis() - start > 1500) break;
    if (!retried && millis() - start >= kRetryAtMs) {
      retried = true;
      WiFi.disconnect(false, false);
      WiFi.begin(ssid.c_str(), pass.c_str());
    }
    delay(100);
  }
  bool ok = (status == WL_CONNECTED);
  if (!ok) {
    switch (status) {
      case WL_NO_SSID_AVAIL: errorOut = "Network not found"; break;
      case WL_CONNECT_FAILED: errorOut = "Wrong password or auth rejected"; break;
      default: errorOut = "Timed out connecting (20 s)"; break;
    }
  }
  WiFi.disconnect(false, false); // drop the test association; AP/captive portal stays up either way
  return ok;
}

const String &wifi_manager_device_name() {
  return deviceName;
}

String wifi_manager_status_line() {
  switch (state) {
    case WifiState::AP_MODE:
      return "AP: " + deviceName;
    case WifiState::CONNECTING:
      return "WiFi...";
    case WifiState::CONNECTED:
      return WiFi.localIP().toString();
    default:
      return "";
  }
}
