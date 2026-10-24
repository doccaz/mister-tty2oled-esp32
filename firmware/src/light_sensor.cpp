// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "light_sensor.h"
#include "pins.h"

#if defined(PIN_LDR)
bool light_sensor_present() { return true; }

uint16_t light_sensor_raw() {
  static bool inited = false;
  if (!inited) {
    pinMode(PIN_LDR, INPUT);
    analogSetPinAttenuation(PIN_LDR, ADC_11db); // a lit room reads 0 on any range; dusk ~170, covered ~700 at this one
    inited = true;
  }
  uint32_t sum = 0;
  for (int i = 0; i < 16; i++) sum += analogRead(PIN_LDR);
  return (uint16_t)(sum / 16);
}
#else
bool light_sensor_present() { return false; }
uint16_t light_sensor_raw() { return 0; }
#endif
