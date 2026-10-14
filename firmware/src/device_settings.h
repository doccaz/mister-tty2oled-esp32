// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#pragma once

// User-editable device settings, edited on the web page that runs on the
// device itself once it's on WiFi (web_portal.cpp "/settings") and kept in NVS
// so they survive power cycles. They are *boot defaults*: apply them at power-up
// and whenever the page changes them. A live command from the MiSTer
// (CMDCON/CMDROT/CMDSAVER) still overrides the running value until the next
// reboot, so an unmodified tty2oled.sh keeps working.

#include <Arduino.h>
#include <Preferences.h>

enum TintMode : uint8_t {
  TINT_OFF = 0,
  TINT_FIXED = 1,    // one colour
  TINT_GRADIENT = 2, // colour 1 -> colour 2
  TINT_RANDOM = 3,   // a new random colour for every picture
};

struct DeviceSettings {
  uint8_t contrast;      // 0-255 backlight
  uint8_t rotation;      // 0 normal, 1 rotated 180 degrees

  uint16_t clockIdleMin; // minutes idle before the clock replaces the picture (0 = never)
  uint16_t blankMin;     // further minutes before the screen goes dark (0 = never); counted from idle start when the clock is off
  uint16_t oledIdleSec;  // onboard OLED blanks after this many idle seconds (0 = never)

  bool ntpEnabled;
  char ntpServer[48];
  char tz[48];           // POSIX TZ string, e.g. "<-03>3" or "CET-1CEST,M3.5.0,M10.5.0/3"

  char adminPass[32];    // empty = the settings page is open on the LAN

  uint8_t tintMode;      // TintMode, applies to the legacy 1bpp/4bpp pictures only
  uint32_t tintColor1;   // 0xRRGGBB
  uint32_t tintColor2;   // 0xRRGGBB, gradient end
  uint8_t tintDir;       // gradient: 0 top-to-bottom, 1 left-to-right

  void setDefaults() {
    contrast = 255;
    rotation = 0;
    clockIdleMin = 0;
    blankMin = 0;
    oledIdleSec = 60;
    ntpEnabled = true;
    strncpy(ntpServer, "pool.ntp.org", sizeof(ntpServer));
    strncpy(tz, "UTC0", sizeof(tz));
    adminPass[0] = '\0';
    tintMode = TINT_OFF;
    tintColor1 = 0xFFB000; // warm amber
    tintColor2 = 0x00A0FF; // blue
    tintDir = 0;
  }
};

class DeviceSettingsStore {
 public:
  DeviceSettings cfg;

  void begin() {
    cfg.setDefaults();
    _prefs.begin("tty2oled", false); // same namespace as the WiFi config; keys are prefixed "s_"
    load();
  }

  void load() {
    cfg.contrast = _prefs.getUChar("s_contrast", cfg.contrast);
    cfg.rotation = _prefs.getUChar("s_rot", cfg.rotation) ? 1 : 0;
    cfg.clockIdleMin = _prefs.getUShort("s_clockMin", cfg.clockIdleMin);
    cfg.blankMin = _prefs.getUShort("s_blankMin", cfg.blankMin);
    cfg.oledIdleSec = _prefs.getUShort("s_oledSec", cfg.oledIdleSec);
    cfg.ntpEnabled = _prefs.getBool("s_ntp", cfg.ntpEnabled);
    getStr("s_ntpSrv", cfg.ntpServer, sizeof(cfg.ntpServer));
    getStr("s_tz", cfg.tz, sizeof(cfg.tz));
    getStr("s_admin", cfg.adminPass, sizeof(cfg.adminPass));
    cfg.tintMode = _prefs.getUChar("s_tintMode", cfg.tintMode);
    cfg.tintColor1 = _prefs.getUInt("s_tint1", cfg.tintColor1);
    cfg.tintColor2 = _prefs.getUInt("s_tint2", cfg.tintColor2);
    cfg.tintDir = _prefs.getUChar("s_tintDir", cfg.tintDir);
  }

  void save() {
    _prefs.putUChar("s_contrast", cfg.contrast);
    _prefs.putUChar("s_rot", cfg.rotation);
    _prefs.putUShort("s_clockMin", cfg.clockIdleMin);
    _prefs.putUShort("s_blankMin", cfg.blankMin);
    _prefs.putUShort("s_oledSec", cfg.oledIdleSec);
    _prefs.putBool("s_ntp", cfg.ntpEnabled);
    _prefs.putString("s_ntpSrv", cfg.ntpServer);
    _prefs.putString("s_tz", cfg.tz);
    _prefs.putString("s_admin", cfg.adminPass);
    _prefs.putUChar("s_tintMode", cfg.tintMode);
    _prefs.putUInt("s_tint1", cfg.tintColor1);
    _prefs.putUInt("s_tint2", cfg.tintColor2);
    _prefs.putUChar("s_tintDir", cfg.tintDir);
  }

 private:
  void getStr(const char *key, char *out, size_t size) {
    if (!_prefs.isKey(key)) return; // keep the default
    _prefs.getString(key, out, size);
  }
  Preferences _prefs;
};

// The one shared instance (defined in device_settings.cpp), loaded by main.cpp's setup().
extern DeviceSettingsStore settingsStore;
