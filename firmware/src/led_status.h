// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#pragma once

#include <Arduino.h>

// Picture-transfer indicator on the CYD's RGB LED (boards without PIN_LED_G/PIN_LED_B compile this
// to no-ops). Blue is solid while a picture is being received (USB, WiFi or an MQTT image fetch),
// green flashes when it arrived complete, cyan double-flashes when it failed or timed out.
// The red LED is GPIO4, which is also the panel's reset line, so it is never touched.
//
// The transfer paths block loop() (serial reads), so begin/end only switch pins; led_status_loop()
// finishes the flashes and, after a safety timeout, turns a stuck "receiving" blue into a failure.

bool led_status_present();                 // this board has the LED
void led_status_set_enabled(bool enabled); // the settings page's "Transfer LED" checkbox
void led_transfer_begin();
void led_transfer_end(bool ok);
void led_status_loop();                    // call from loop(): cheap when idle
