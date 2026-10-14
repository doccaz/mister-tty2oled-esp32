// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "oled_status.h"

// HAS_ONBOARD_OLED is set by platformio.ini (1 for env:esp32c3, 0 for
// env:esp32c3_nooled) - the same ESP32-C3-mini board is sold both with
// and without the built-in 0.42" status OLED. When it's 0, U8g2/Wire are
// never #included, so PlatformIO's Library Dependency Finder doesn't pull
// either into the build - real RAM/flash savings on boards without the
// panel, not just a runtime no-op (see CLAUDE.md "RAM headroom").
#if HAS_ONBOARD_OLED

#include "pins.h"
#include "protocol.h"
#include "version.h"
#include <Arduino.h>
#include <U8g2lib.h>
#include <Wire.h>

namespace {

// 128x64 controller RAM, 72x40 visible glass - same "drive it as the
// bigger panel, offset into the visible window" approach as the reference
// WLED usermod this was ported from (see CLAUDE.md). Offsets copied
// verbatim from there: they reflect the panel's actual RAM window, not a
// geometric centering formula (a naive (128-72)/2, (64-40)/2 == 28,12
// clips on real hardware).
U8G2_SSD1306_128X64_NONAME_F_HW_I2C u8g2(U8G2_R0, U8X8_PIN_NONE);
constexpr int kOffX = 28;
constexpr int kOffY = 24;

bool ready = false;
uint8_t suspendCount = 0;
bool blanked = false;     // CMDSAVER screensaver (oled_status_off()/_on())
bool idleBlanked = false; // no command/button for kIdleBlankMs, independent of CMDSAVER
// An always-on static dashboard burns in on a real OLED, so blank it when
// nothing has talked to the device for a while. Counted from boot too, so the
// IP/version stay readable after power-up. Any command (serial, WiFi, MQTT) or
// a GPIO9 press wakes it; tune here.
unsigned long g_idleBlankMs = 60000; // 0 = never; set from the device settings (oled_status_set_idle_sec())
unsigned long lastDraw = 0;
constexpr unsigned long kRedrawIntervalMs = 100;

unsigned long g_bootMs = 0;

void drawDashboard() {
  u8g2.clearBuffer();

  u8g2.setFont(u8g2_font_5x7_tf);
  u8g2.drawStr(kOffX, kOffY + 7, "tty2oled");
  u8g2.drawHLine(kOffX, kOffY + 9, 72);

  u8g2.setFont(u8g2_font_4x6_tf);
  char line[17];
  protocol_get_corename().substring(0, 16).toCharArray(line, sizeof(line));
  u8g2.drawStr(kOffX, kOffY + 20, line);

  // Last command received, with a trailing "*" while it's fresh (<2 s).
  unsigned long sinceActivity = millis() - protocol_last_activity_ms();
  u8g2.setCursor(kOffX, kOffY + 30);
  u8g2.print("> ");
  const String &cmd = protocol_last_command();
  u8g2.print(cmd.length() ? cmd : String("-"));
  u8g2.print(sinceActivity < 2000 ? "*" : "");

  unsigned long uptimeSec = (millis() - g_bootMs) / 1000;
  u8g2.setCursor(kOffX, kOffY + 38);
  u8g2.print("v" FW_VERSION " ");
  u8g2.print(uptimeSec);
  u8g2.print("s");

  u8g2.sendBuffer();
}

} // namespace

void oled_status_init() {
  Wire.begin(PIN_OLED_SDA, PIN_OLED_SCL);
  Wire.setClock(400000);

  ready = u8g2.begin();
  if (!ready) return; // no OLED on this board - status display simply stays off

  u8g2.setContrast(255);
  u8g2.clearBuffer();
  u8g2.sendBuffer();
  g_bootMs = millis();
}

void oled_status_loop() {
  if (!ready || suspendCount > 0 || blanked) return;

  // Idle blank: clear the glass once, then stay off until something happens.
  unsigned long lastEvent = max(protocol_last_wake_ms(), g_bootMs);
  if (g_idleBlankMs > 0 && millis() - lastEvent >= g_idleBlankMs) {
    if (!idleBlanked) {
      u8g2.clearBuffer();
      u8g2.sendBuffer(); // by content, not setPowerSave() - that hangs this panel (see oled_status_off())
      idleBlanked = true;
    }
    return;
  }
  idleBlanked = false;

  if (millis() - lastDraw < kRedrawIntervalMs) return;
  lastDraw = millis();
  drawDashboard();
}

void oled_status_suspend() {
  suspendCount++;
}

void oled_status_resume() {
  if (suspendCount > 0) suspendCount--;
}

void oled_status_set_idle_sec(uint16_t seconds) {
  g_idleBlankMs = (unsigned long)seconds * 1000UL;
}

void oled_status_off() {
  if (!ready) return;
  u8g2.clearBuffer();
  u8g2.sendBuffer();
  blanked = true;
}

void oled_status_on() {
  blanked = false; // next oled_status_loop() redraws on its own ~100ms tick
}

#else // !HAS_ONBOARD_OLED

void oled_status_init() {}
void oled_status_loop() {}
void oled_status_suspend() {}
void oled_status_resume() {}
void oled_status_set_idle_sec(uint16_t) {}
void oled_status_off() {}
void oled_status_on() {}

#endif
