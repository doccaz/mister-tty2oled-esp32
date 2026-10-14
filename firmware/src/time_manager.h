// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#pragma once

// One place that knows what time it is. Two sources, in this order:
//   1. NTP, once on WiFi (settings: ntpEnabled/ntpServer/tz). The system clock
//      then holds true UTC and local time is derived with the POSIX TZ string.
//   2. CMDSETTIME from the MiSTer's tty2oled.sh, which sends *local* time as
//      epoch seconds already shifted by its timezone (no TZ on this side). Only
//      used until NTP has synced (or whenever NTP is off); ignored once NTP
//      has the clock, so the two can't fight.
// Nothing here is persisted: there is no battery-backed RTC, so a cold boot
// starts without a time until NTP (or the MiSTer) provides one.

#include <Arduino.h>
#include <time.h>

// (Re)configures SNTP and the timezone from settingsStore. Safe to call at any
// time; does nothing useful until WiFi is up, and wifi_manager calls it again
// once connected.
void time_apply_settings();

// Local broken-down time from whichever source is available; false if none.
bool time_get_local(struct tm &out);
// "HH:MM" and "dd-Mon-yy" for the clock screen; false if no time yet.
bool time_get_strings(String &hm, String &date);

// MiSTer CMDSETTIME: local epoch seconds. Returns false (and ignores it) when
// NTP currently owns the clock.
bool time_set_manual_local(time_t localEpoch);

bool time_ntp_synced();
const char *time_ntp_state(); // "off", "waiting" or "synced"
