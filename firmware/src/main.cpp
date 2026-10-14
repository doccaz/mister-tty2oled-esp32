// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include <Arduino.h>
#include "display.h"
#include "protocol.h"
#include "oled_status.h"
#include "wifi_manager.h"
#include "ws_protocol.h"
#include "mqtt_client.h"
#include "device_settings.h"
#include "diag.h"

// TEMP: hardware stability self-test - cycles solid colors + text via the
// same fillScreenFast()/pushRect() code paths used by real rendering,
// entirely decoupled from JPEGDEC/serial parsing, to isolate whether the
// display/SPI wiring itself is reliable under sustained repeated use.
// Set to false to return to normal protocol operation.
#define RUN_SELF_TEST false

void setup() {
  crumb_init();
  // protocol_init() runs before display_init() so "ttyrdy;" goes out as
  // early as possible. Note: moving it earlier does NOT change how much
  // heap is available for Serial.setRxBufferSize() - display.cpp's large
  // buffers (g_frame etc.) are static globals, so their space is already
  // reserved before setup() even starts, regardless of call order here.
  // (The actual RX-queue fix was making protocol.cpp check
  // setRxBufferSize()'s return value and fall back to a size that reliably
  // fits - see protocol_init() - plus pacing writes on the sender side so
  // the queue is never asked to hold more than it can, see
  // web/src/serial.ts's writePaced().)
#if RUN_SELF_TEST
  display_init();
  display_self_test(); // never returns
#else
  protocol_init(); // sends "ttyrdy;" once Serial is ready
  display_init();
  settingsStore.begin();
  protocol_apply_settings(); // saved contrast/rotation/tint/idle defaults, before anything is drawn
  display_show_start_screen();
  oled_status_init();
  // Runs after display_init() - AP_MODE/CONNECTING immediately draw their
  // own screen over the start splash (brief flash, same as
  // chromecast-esp32's boot flow). Serial protocol/USB-CDC are already up
  // by this point regardless of which WiFi state this lands in.
  wifi_manager_init();
#endif
}

void loop() {
#if !RUN_SELF_TEST
  crumb(C_LOOP_PROTOCOL);
  protocol_process();
  crumb(C_LOOP_BUTTON);
  protocol_button_check();
  crumb(C_LOOP_SAVER);
  protocol_saver_check();
  crumb(C_LOOP_OLED);
  oled_status_loop();
  crumb(C_LOOP_WIFI);
  wifi_manager_loop();
  crumb(C_LOOP_WS);
  ws_protocol_loop();
  crumb(C_LOOP_MQTT);
  mqtt_client_loop();
#endif
}
