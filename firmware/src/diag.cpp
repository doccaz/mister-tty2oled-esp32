// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "diag.h"
#include <esp_attr.h>
#include <esp_system.h>

RTC_NOINIT_ATTR volatile uint16_t g_crumb;
RTC_NOINIT_ATTR volatile uint16_t g_crumbCheck;

static uint16_t g_prev = 0;
static int g_reason = 0;

void crumb_init() {
  g_reason = (int)esp_reset_reason();
  // RTC no-init memory is garbage after a power-on reset; only trust it otherwise.
  if (g_reason != ESP_RST_POWERON && g_crumbCheck == (uint16_t)(g_crumb ^ 0xA5A5)) g_prev = g_crumb;
  g_crumb = C_NONE;
}

uint16_t crumb_prev() { return g_prev; }
int reset_reason_raw() { return g_reason; }
