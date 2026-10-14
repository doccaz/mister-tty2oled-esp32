// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#pragma once

// "Breadcrumb" for finding hangs: the last step the firmware reached, kept in RTC
// no-init memory so it survives a reset. After a hang, reset the board and read
// `crumb_prev` from GET /status: it names the step that never finished. Costs
// one volatile store per call. See CLAUDE.md "Hang found on 2026-10-14".

#include <stdint.h>

enum Crumb : uint16_t {
  C_NONE = 0,
  C_LOOP_PROTOCOL = 1,
  C_LOOP_BUTTON,
  C_LOOP_SAVER,
  C_LOOP_OLED,
  C_LOOP_WIFI,
  C_LOOP_WS,
  C_LOOP_MQTT,
  C_PORTAL_CLIENT = 20, // WebServer::handleClient()
  C_SETTINGS_BEGIN = 30,
  C_SETTINGS_FLUSH,
  C_SETTINGS_STATUS_ROWS,
  C_SETTINGS_END,
  C_SAVE_BEGIN = 40,
  C_SAVE_NVS,
  C_SAVE_APPLY,
  C_SAVE_TIME,
  C_SAVE_REDISPLAY,
  C_SAVE_DONE,
  C_STATUS_BEGIN = 50,
  C_STATUS_DONE,
  C_SAVER_CLOCK = 60,
  C_SAVER_BLANK,
  C_SAVER_NORMAL,
  C_TIME_APPLY = 70,
  C_SNTP_CB,
};

extern volatile uint16_t g_crumb;
extern volatile uint16_t g_crumbCheck;
inline void crumb(uint16_t c) {
  g_crumb = c;
  g_crumbCheck = (uint16_t)(c ^ 0xA5A5);
}

// Value of g_crumb at the previous run, captured at boot (0 if unknown).
uint16_t crumb_prev();
// esp_reset_reason() of this boot, as the raw enum value.
int reset_reason_raw();
void crumb_init(); // call first thing in setup()
