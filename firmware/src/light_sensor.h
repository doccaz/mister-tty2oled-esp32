// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#pragma once

#include <Arduino.h>

// Ambient light sensor: the CYD's photoresistor on GPIO34. Boards without one report
// light_sensor_present() == false and the rest of the firmware never reads it.
bool light_sensor_present();

// Raw 12-bit ADC reading (0..4095), averaged over a few samples. On the CYD the divider reads
// LOWER in bright light (to be confirmed from measurements), so callers must not assume a
// direction without checking this file's calibration notes.
uint16_t light_sensor_raw();
