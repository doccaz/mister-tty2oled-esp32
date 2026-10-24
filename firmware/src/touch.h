// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#pragma once

#include <Arduino.h>

// Resistive touch (XPT2046) on the CYD. Only the touch itself is used, not the coordinates:
// it wakes the display, like the BOOT button. Boards without a touch controller report
// touch_present() == false and touch_pressed() is always false.
bool touch_present();

// Raw pressure reading (0 when nothing presses), for /status and calibration.
uint16_t touch_pressure();

// True while the screen is pressed (debounced). Cheap enough to call every loop().
bool touch_pressed();
