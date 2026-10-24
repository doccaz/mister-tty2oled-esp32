// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "auto_brightness.h"
#include "device_settings.h"
#include "display.h"
#include "light_sensor.h"

namespace {
bool g_enabled = false;
bool g_overridden = false;
uint8_t g_min = 16, g_max = 255;
float g_smooth = -1;   // smoothed raw reading, <0 until the first sample
float g_level = 127;   // the brightness currently applied
uint32_t g_lastSampleMs = 0, g_lastStepMs = 0;

// Measured on the owner's CYD (2026-10-24): HIGHER raw = DARKER. A lit room and a flashlight
// both read 0 (below the ADC floor on any range), the room at dusk with the lamp off ~170 and
// the sensor covered ~700. Anchors (raw -> 0..1 share of the brightness range):
struct Anchor { uint16_t raw; float t; };
constexpr Anchor kCurve[] = {{0, 1.0f}, {170, 0.5f}, {650, 0.0f}};
}  // namespace

uint8_t auto_brightness_curve(uint16_t raw, uint8_t minLevel, uint8_t maxLevel) {
  float t = kCurve[0].t;
  if (raw >= kCurve[2].raw) {
    t = kCurve[2].t;
  } else {
    for (int i = 0; i < 2; i++) {
      if (raw <= kCurve[i + 1].raw) {
        float f = (float)(raw - kCurve[i].raw) / (kCurve[i + 1].raw - kCurve[i].raw);
        t = kCurve[i].t + f * (kCurve[i + 1].t - kCurve[i].t);
        break;
      }
    }
  }
  if (minLevel > maxLevel) minLevel = maxLevel;
  return (uint8_t)(minLevel + t * (maxLevel - minLevel) + 0.5f);
}

void auto_brightness_apply_settings() {
  const DeviceSettings &c = settingsStore.cfg;
  g_enabled = light_sensor_present() && c.autoBright;
  g_overridden = false; // saving the settings page is an explicit choice, so it wins again
  g_min = c.autoMin;
  g_max = c.autoMax < c.autoMin ? c.autoMin : c.autoMax;
  g_smooth = -1;
  g_level = c.contrast;
}

void auto_brightness_override() { g_overridden = true; }

void auto_brightness_loop() {
  if (!g_enabled || g_overridden) return;
  uint32_t now = millis();
  if (now - g_lastSampleMs >= 250) {
    g_lastSampleMs = now;
    float raw = (float)light_sensor_raw();
    g_smooth = g_smooth < 0 ? raw : g_smooth * 0.9f + raw * 0.1f; // ~2.5 s time constant
  }
  if (g_smooth < 0 || now - g_lastStepMs < 50) return;
  g_lastStepMs = now;
  float target = auto_brightness_curve((uint16_t)g_smooth, g_min, g_max);
  float diff = target - g_level;
  // A dead-band keeps the backlight from twitching as the light drifts, then it fades
  // gradually instead of jumping.
  if (fabsf(diff) < 6) return;
  g_level += diff > 0 ? min(diff, 3.0f) : max(diff, -3.0f);
  display_set_contrast_auto((uint8_t)(g_level + 0.5f));
}
