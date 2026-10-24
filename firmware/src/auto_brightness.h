// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#pragma once

#include <Arduino.h>

// Follows the room light with the backlight, on boards that have a light sensor (the CYD).
// Off by default; the settings page turns it on and sets the brightness range.

// Reads the saved settings (enabled, min, max). Call after settingsStore.begin() and whenever
// the settings page saves.
void auto_brightness_apply_settings();

// A live CMDCON from the MiSTer wins over auto brightness until the next reboot, like it wins
// over every other saved default.
void auto_brightness_override();

// Call from loop(): cheap when disabled.
void auto_brightness_loop();

// The mapping from a raw sensor reading to a brightness, exposed so it can be unit-checked.
uint8_t auto_brightness_curve(uint16_t raw, uint8_t minLevel, uint8_t maxLevel);
