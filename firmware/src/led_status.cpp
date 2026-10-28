// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "led_status.h"
#include "pins.h"

#if defined(PIN_LED_G) && defined(PIN_LED_B)

namespace {

// The LED is active low: LOW lights a colour.
constexpr uint8_t COLOR_NONE = 0, COLOR_GREEN = 1, COLOR_BLUE = 2, COLOR_CYAN = 3;
constexpr uint32_t OK_FLASH_MS = 150;
constexpr uint32_t FAIL_FLASH_MS = 120; // each on/off half of the double flash
constexpr uint32_t RECEIVE_TIMEOUT_MS = 15000; // a transfer that never ends counts as failed

enum class Phase : uint8_t { IDLE, RECEIVING, OK_FLASH, FAIL_ON1, FAIL_OFF, FAIL_ON2 };

bool g_enabled = true;
Phase g_phase = Phase::IDLE;
uint32_t g_phaseStart = 0;

void show(uint8_t color) {
  digitalWrite(PIN_LED_G, (color == COLOR_GREEN || color == COLOR_CYAN) ? LOW : HIGH);
  digitalWrite(PIN_LED_B, (color == COLOR_BLUE || color == COLOR_CYAN) ? LOW : HIGH);
}

void enter(Phase p, uint8_t color) {
  g_phase = p;
  g_phaseStart = millis();
  show(color);
}

} // namespace

bool led_status_present() { return true; }

void led_status_set_enabled(bool enabled) {
  g_enabled = enabled;
  if (!enabled) {
    g_phase = Phase::IDLE;
    show(COLOR_NONE);
  }
}

void led_transfer_begin() {
  if (!g_enabled) return;
  pinMode(PIN_LED_G, OUTPUT);
  pinMode(PIN_LED_B, OUTPUT);
  enter(Phase::RECEIVING, COLOR_BLUE);
}

void led_transfer_end(bool ok) {
  if (!g_enabled) return;
  if (ok) enter(Phase::OK_FLASH, COLOR_GREEN);
  else enter(Phase::FAIL_ON1, COLOR_CYAN);
}

void led_status_loop() {
  if (g_phase == Phase::IDLE) return;
  uint32_t el = millis() - g_phaseStart;
  switch (g_phase) {
    case Phase::RECEIVING:
      if (el > RECEIVE_TIMEOUT_MS) enter(Phase::FAIL_ON1, COLOR_CYAN);
      break;
    case Phase::OK_FLASH:
      if (el >= OK_FLASH_MS) { g_phase = Phase::IDLE; show(COLOR_NONE); }
      break;
    case Phase::FAIL_ON1:
      if (el >= FAIL_FLASH_MS) enter(Phase::FAIL_OFF, COLOR_NONE);
      break;
    case Phase::FAIL_OFF:
      if (el >= FAIL_FLASH_MS) enter(Phase::FAIL_ON2, COLOR_CYAN);
      break;
    case Phase::FAIL_ON2:
      if (el >= FAIL_FLASH_MS) { g_phase = Phase::IDLE; show(COLOR_NONE); }
      break;
    default:
      break;
  }
}

#else // boards without the LED

bool led_status_present() { return false; }
void led_status_set_enabled(bool) {}
void led_transfer_begin() {}
void led_transfer_end(bool) {}
void led_status_loop() {}

#endif
