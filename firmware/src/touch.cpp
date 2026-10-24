// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "touch.h"
#include "pins.h"

#if defined(PIN_TOUCH_CS)
namespace {
// Software SPI: five pins at a few hundred kHz is plenty for reading a pressure now and then,
// and it keeps the touch chip off the display's bus (and off the SD card's).
constexpr uint8_t kCmdZ1 = 0xB1, kCmdZ2 = 0xC1;
constexpr uint16_t kPressThreshold = 250; // measured on the CYD: idle <= ~30, a finger resting <= ~30, any real press >= ~2700

uint16_t transfer(uint8_t cmd) {
  uint16_t v = 0;
  digitalWrite(PIN_TOUCH_CS, LOW);
  for (int i = 7; i >= 0; i--) {
    digitalWrite(PIN_TOUCH_MOSI, (cmd >> i) & 1);
    digitalWrite(PIN_TOUCH_CLK, HIGH);
    delayMicroseconds(2);
    digitalWrite(PIN_TOUCH_CLK, LOW);
    delayMicroseconds(2);
  }
  for (int i = 0; i < 16; i++) {
    digitalWrite(PIN_TOUCH_CLK, HIGH);
    delayMicroseconds(2);
    digitalWrite(PIN_TOUCH_CLK, LOW);
    v = (v << 1) | digitalRead(PIN_TOUCH_MISO);
    delayMicroseconds(2);
  }
  digitalWrite(PIN_TOUCH_CS, HIGH);
  return (v >> 3) & 0x0FFF; // 12 bits, left-justified after the busy bit
}

void begin() {
  static bool done = false;
  if (done) return;
  done = true;
  pinMode(PIN_TOUCH_CS, OUTPUT);
  pinMode(PIN_TOUCH_CLK, OUTPUT);
  pinMode(PIN_TOUCH_MOSI, OUTPUT);
  pinMode(PIN_TOUCH_MISO, INPUT);
  pinMode(PIN_TOUCH_IRQ, INPUT);
  digitalWrite(PIN_TOUCH_CS, HIGH);
  digitalWrite(PIN_TOUCH_CLK, LOW);
}
}  // namespace

bool touch_present() { return true; }

uint16_t touch_pressure() {
  begin();
  uint16_t z1 = transfer(kCmdZ1);
  uint16_t z2 = transfer(kCmdZ2);
  if (z1 == 0) return 0;
  int z = (int)z1 + 4095 - (int)z2;
  return z < 0 ? 0 : (uint16_t)z;
}

bool touch_pressed() {
  static uint32_t lastPollMs = 0;
  static bool state = false;
  static uint8_t hits = 0;
  uint32_t now = millis();
  if (now - lastPollMs < 25) return state; // ~40 polls a second is plenty
  lastPollMs = now;
  bool raw = touch_pressure() > kPressThreshold;
  hits = raw ? (hits < 3 ? hits + 1 : 3) : 0;
  state = hits >= 2; // two polls in a row: ignore a single noisy sample
  return state;
}
#else
bool touch_present() { return false; }
uint16_t touch_pressure() { return 0; }
bool touch_pressed() { return false; }
#endif
