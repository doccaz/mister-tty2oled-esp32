// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "time_manager.h"
#include "device_settings.h"
#include "diag.h"
#include <WiFi.h>
#include <esp_sntp.h>
#include <sys/time.h>

namespace {

volatile bool g_synced = false; // system clock holds real UTC (set by an SNTP sync)
bool g_manual = false;          // system clock holds the MiSTer's local time (CMDSETTIME)

void onSync(struct timeval *) {
  crumb(C_SNTP_CB);
  g_synced = true;
  g_manual = false;
}

} // namespace

void time_apply_settings() {
  crumb(C_TIME_APPLY);
  const DeviceSettings &s = settingsStore.cfg;
  // The TZ only matters for converting NTP's UTC; set it either way so a
  // timezone change applies immediately without waiting for the next sync.
  setenv("TZ", s.tz[0] ? s.tz : "UTC0", 1);
  tzset();

  if (!s.ntpEnabled) {
    if (sntp_enabled()) sntp_stop();
    return;
  }
  sntp_set_time_sync_notification_cb(onSync);
  // configTzTime() (re)starts SNTP with this server; the lwIP client then
  // re-syncs on its own roughly hourly.
  configTzTime(s.tz[0] ? s.tz : "UTC0", s.ntpServer[0] ? s.ntpServer : "pool.ntp.org");
}

bool time_get_local(struct tm &out) {
  time_t now = time(nullptr);
  if (g_synced) {
    localtime_r(&now, &out);
    return true;
  }
  if (g_manual) {
    gmtime_r(&now, &out); // already local - see time_manager.h
    return true;
  }
  return false;
}

bool time_get_strings(String &hm, String &date) {
  struct tm t;
  if (!time_get_local(t)) return false;
  char a[8], b[16];
  strftime(a, sizeof(a), "%H:%M", &t);
  strftime(b, sizeof(b), "%d-%b-%y", &t);
  hm = a;
  date = b;
  return true;
}

bool time_set_manual_local(time_t localEpoch) {
  if (settingsStore.cfg.ntpEnabled && g_synced) return false; // NTP owns the clock
  struct timeval tv = {localEpoch, 0};
  settimeofday(&tv, nullptr);
  g_manual = true;
  g_synced = false; // the system clock is no longer real UTC
  return true;
}

bool time_ntp_synced() {
  return g_synced;
}

const char *time_ntp_state() {
  if (!settingsStore.cfg.ntpEnabled) return "off";
  return g_synced ? "synced" : "waiting";
}
