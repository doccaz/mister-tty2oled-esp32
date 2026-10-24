// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#include "display.h"
#include "gc9a01.h"
#include "pins.h"
#include "version.h"
#include <JPEGDEC.h>
#include <qrcode.h>
#include <cmath>
#include <cstring>
#if defined(BOARD_CYD)
#include "esp_bt.h"
#endif

namespace {

constexpr int16_t DISP_W = 240;
constexpr int16_t DISP_H = 240;
constexpr int16_t DISP_CX = DISP_W / 2;
constexpr int16_t DISP_CY = DISP_H / 2;

// Largest 4:1 (256:64) rect inscribed in the 240px circle, used for legacy
// grayscale art. LEGACY_W:LEGACY_H (240:60) is exactly the same 4:1 ratio
// as the source LEGACY_SRC_W:LEGACY_SRC_H (256:64), so the whole source
// image is scaled down uniformly (16:15) to fit - no cropping and no
// letterbox bars needed, since the aspect ratios already match exactly.
constexpr int16_t LEGACY_W = 240;
constexpr int16_t LEGACY_H = 60;
constexpr int16_t LEGACY_Y = DISP_CY - LEGACY_H / 2;
constexpr int16_t LEGACY_SRC_W = 256;
constexpr int16_t LEGACY_SRC_H = 64;
constexpr int16_t LEGACY_SRC_LINE_1BPP = LEGACY_SRC_W / 8; // 32
constexpr int16_t LEGACY_SRC_LINE_4BPP = LEGACY_SRC_W / 2; // 128

GC9A01Display gfx;
JPEGDEC jpeg;

uint8_t g_contrast = 127;
uint8_t g_rotation = 0;

// State for display_toggle_wifi_qr(): remembers the AP-status screen's
// ssid/ip (set by display_show_ap_mode()) so a button press can flip to
// the QR view and back without protocol.cpp needing to pass them through.
String g_apSsid;
String g_apIp;
bool g_qrShown = false;

// LEDC backlight PWM channel. Using the legacy ledcSetup/ledcAttachPin/
// ledcWrite(channel,...) API (not the newer ledcAttach(pin,...) API) since
// this targets arduino-esp32 core 2.0.x, which doesn't have the latter.
constexpr int LEDC_CHANNEL_BL = 0;

// Full-frame decode target for CMDCORC JPEGs, so a transition effect can
// reveal it onto the glass in a pattern other than JPEG's natural
// top-to-bottom MCU decode order. 240x240x2 bytes = 112.5KB - fits the
// C3's ~320KB usable DRAM alongside JPEGDEC's own (much smaller) working
// buffers. Colors are stored byte-swapped (big-endian on the wire) ready
// for GC9A01Display::pushRect(), which writes bytes as-is over SPI.
#if !defined(BOARD_CYD)
uint16_t g_frame[DISP_W * DISP_H];
inline uint16_t *frameRow(int y) { return &g_frame[y * DISP_W]; }
inline void pushFrame(int x, int y, int w, int h) { gfx.pushRect(x, y, w, h, &g_frame[y * DISP_W + x], DISP_W); }
void clearFrame() {
  for (int y = 0; y < DISP_H; y++) memset(frameRow(y), 0, DISP_W * sizeof(uint16_t));
}
#endif
// The CYD (classic ESP32) has no decoded frame at all: the stack needs ~80KB of free heap to
// serve TCP at all and a 115KB frame leaves ~60KB. It keeps the last picture's JPEG instead
// and decodes it straight to the glass; see the CYD block before display_draw_jpeg().
#if defined(BOARD_CYD)
constexpr size_t LAST_JPEG_MAX = 20000; // the device's picture limit (COLOR_BUF_MAX)
uint8_t *g_lastJpeg = nullptr;
size_t g_lastJpegLen = 0;
#endif

inline uint16_t swap16(uint16_t v) {
  return (v >> 8) | (v << 8);
}

// Pushes decoded JPEG blocks straight to the physical display instead of
// writing into g_frame - used by display_draw_jpeg_transient() for MQTT
// notification images, which must NOT become the new "last shown
// picture" g_frame/lastPictureKind track (see CLAUDE.md's MQTT
// notifications section: an image notification's whole point is to
// revert to whatever marquee art was showing before it, which is only
// possible if that art's own g_frame content was never overwritten).
//
// One pushRect() per JPEG block (its own transaction), not
// beginBatch()/pushRectBatched()/endBatch() wrapping the whole decode()
// call - found by testing on real hardware that holding an SPI
// transaction open across JPEGDEC's own internal per-block decode
// compute time (not just the pushes) hung the device. Every other
// batched-push case in this file only wraps a loop we fully control
// (fade/iris), never a third-party library's own callback-driven
// timing - this is the one place that pattern doesn't apply. Still
// avoids the old per-ROW transaction-per-call cost (see CLAUDE.md's
// "per-row transaction stall" section): one call per whole MCU block in
// the common (unclipped) case, only falling back to per-row for a
// block that's partially off-screen.
int jpegDrawCallbackTransient(JPEGDRAW *pDraw) {
  int x0 = pDraw->x, y0 = pDraw->y, w = pDraw->iWidth, h = pDraw->iHeight;
  if (x0 >= 0 && y0 >= 0 && x0 + w <= DISP_W && y0 + h <= DISP_H) {
    gfx.pushRect(x0, y0, w, h, pDraw->pPixels, w);
    return 1;
  }
  for (int row = 0; row < h; row++) {
    int y = y0 + row;
    if (y < 0 || y >= DISP_H) continue;
    int rowLen = w;
    int rx0 = x0;
    if (rx0 < 0) rowLen += rx0, rx0 = 0; // clip left overhang
    if (rx0 + rowLen > DISP_W) rowLen = DISP_W - rx0; // clip right overhang
    if (rowLen <= 0) continue;
    const uint16_t *rowPixels = &pDraw->pPixels[row * w + (rx0 - x0)];
    gfx.pushRect(rx0, y, rowLen, 1, rowPixels);
  }
  return 1;
}

#if !defined(BOARD_CYD)
int jpegDrawCallback(JPEGDRAW *pDraw) {
  for (int row = 0; row < pDraw->iHeight; row++) {
    int y = pDraw->y + row;
    if (y < 0 || y >= DISP_H) continue;
    int rowLen = pDraw->iWidth;
    int x0 = pDraw->x;
    if (x0 < 0) rowLen += x0, x0 = 0; // clip left overhang
    if (x0 + rowLen > DISP_W) rowLen = DISP_W - x0; // clip right overhang
    if (rowLen <= 0) continue;
    memcpy(&frameRow(y)[x0], &pDraw->pPixels[row * pDraw->iWidth + (x0 - pDraw->x)], rowLen * sizeof(uint16_t));
  }
  return 1;
}

#endif

#if !defined(BOARD_CYD)
// delay(stepDelay) on its own assumes the pushRect()/compute work above it
// is free, which it isn't - a full-frame pushRect() alone costs ~92ms at
// the driver's 10MHz SPI clock (240*240*2 bytes), so a naive per-step
// delay() on top of that lets a "slow" (1200ms/30-step) fade balloon to
// 4+ seconds of real wall time, blowing well past the 3s ack timeout
// web/src/serial.ts uses to decide a send finished - which then let the
// browser start writing the NEXT queued send while this device was still
// stuck here not reading Serial, wedging its write() on backpressure with
// no timeout of its own. Subtracting elapsed step time keeps total
// transitionReveal() wall time close to the requested durationMs
// regardless of how expensive the step's own draw work is.
void stepDelay(uint32_t stepStart, uint16_t targetStepMs) {
  uint32_t elapsed = millis() - stepStart;
  if (elapsed < targetStepMs) delay(targetStepMs - elapsed);
}

// Curated transition set, mirrored in web/src/effects.ts so the id numbers
// sent over CMDCORC mean the same thing on both ends. Reveals g_frame onto
// the display over durationMs; effect 0 (or durationMs == 0) draws
// instantly. Step counts are capped so effects stay responsive even at
// long durations - each step still redraws its whole visible region
// rather than tracking per-frame deltas, which is simpler and cheap
// enough at 240x240.
void transitionReveal(uint8_t effect, uint16_t durationMs) {
  if (effect == 0 || durationMs == 0) {
    pushFrame(0, 0, DISP_W, DISP_H);
    return;
  }

  const int steps = constrain(durationMs / 30, 4, 30);
  const uint16_t stepMs = durationMs / steps;

  switch (effect) {
    case 1: // wipe left -> right
      for (int s = 1; s <= steps; s++) {
        uint32_t t0 = millis();
        int w = (DISP_W * s) / steps;
        // srcStride=DISP_W required here: g_frame's real row width (240)
        // is wider than this partial-width slice (w), so each row after
        // the first must skip the leftover (240-w) pixels to stay
        // aligned - omitting it was a real bug (see CLAUDE.md).
        pushFrame(0, 0, w, DISP_H);
        stepDelay(t0, stepMs);
      }
      break;

    case 2: // wipe right -> left
      for (int s = 1; s <= steps; s++) {
        uint32_t t0 = millis();
        int w = (DISP_W * s) / steps;
        int x0 = DISP_W - w;
        // One stride-aware call instead of a 240-row loop (each iteration
        // of which used to pay a full SPI transaction's setup/teardown
        // cost) - same fix as case 1, plus the performance fix that
        // stopped this effect from stalling on real hardware.
        pushFrame(x0, 0, w, DISP_H);
        stepDelay(t0, stepMs);
      }
      break;

    case 3: // wipe top -> bottom
      for (int s = 1; s <= steps; s++) {
        uint32_t t0 = millis();
        int h = (DISP_H * s) / steps;
        pushFrame(0, 0, DISP_W, h); // w==DISP_W already, but explicit for clarity
        stepDelay(t0, stepMs);
      }
      break;

    case 4: { // iris: expanding circle from center
      const int cx = DISP_W / 2;
      const int cy = DISP_H / 2;
      const float maxR = sqrtf((float)(cx * cx + cy * cy));
      for (int s = 1; s <= steps; s++) {
        uint32_t t0 = millis();
        float r = maxR * s / steps;
        int yStart = max(0, (int)(cy - r));
        int yEnd = min(DISP_H - 1, (int)(cy + r));
        // The visible x-range differs per row (circular mask), so unlike
        // the wipes this can't collapse into one pushRect() call - but
        // wrapping the per-row calls in one beginBatch()/endBatch() still
        // avoids paying a full SPI transaction's setup/teardown per row,
        // which was expensive enough to stall this effect on real
        // hardware (see CLAUDE.md).
        gfx.beginBatch();
        for (int y = yStart; y <= yEnd; y++) {
          float dy = y - cy;
          float dxf = sqrtf(max(0.0f, r * r - dy * dy));
          int x0 = max(0, (int)(cx - dxf));
          int x1 = min(DISP_W - 1, (int)(cx + dxf));
          int w = x1 - x0 + 1;
          if (w > 0) gfx.pushRectBatched(x0, y, w, 1, &frameRow(y)[x0]);
        }
        gfx.endBatch();
        stepDelay(t0, stepMs);
      }
      break;
    }

    case 5: { // fade: cross-dissolve from black via per-pixel RGB565 blend
      // Unlike the wipes/iris (which only ever push a partial region), fade
      // redraws the FULL frame every step - at the driver's 10MHz SPI
      // clock that's ~92ms of pure SPI time per step regardless of
      // durationMs, so steps is capped much lower here (30 full frames
      // would be a ~2.76s SPI floor alone, uncomfortably close to
      // web/src/serial.ts's ack timeout even before any compute time).
      const int fadeSteps = min(steps, 12);
      const uint16_t fadeStepMs = durationMs / fadeSteps;
      // Blend one row at a time into a DISP_W-wide scratch buffer instead of
      // a full 240x240 static buffer (that was a static ~112.5KB - the
      // single biggest thing standing between this firmware and fitting
      // WiFi's DRAM footprint, see CLAUDE.md WiFi planning notes). Wrapped
      // in beginBatch()/pushRectBatched()/endBatch() so this stays one SPI
      // transaction per step instead of 240.
      static uint16_t blendedRow[DISP_W];
      for (int s = 1; s <= fadeSteps; s++) {
        uint32_t t0 = millis();
        int t256 = (256 * s) / fadeSteps; // fixed-point 0..256 blend factor
        gfx.beginBatch();
        for (int y = 0; y < DISP_H; y++) {
          const uint16_t *srcRow = frameRow(y);
          for (int x = 0; x < DISP_W; x++) {
            uint16_t c = swap16(srcRow[x]); // unswap to compute, reswap to store
            uint8_t r = ((c >> 11) & 0x1F) * t256 / 256;
            uint8_t g = ((c >> 5) & 0x3F) * t256 / 256;
            uint8_t b = (c & 0x1F) * t256 / 256;
            blendedRow[x] = swap16((r << 11) | (g << 5) | b);
          }
          gfx.pushRectBatched(0, y, DISP_W, 1, blendedRow);
        }
        gfx.endBatch();
        stepDelay(t0, fadeStepMs);
      }
      break;
    }

    default:
      pushFrame(0, 0, DISP_W, DISP_H);
      break;
  }
}

#endif

// Shared nearest-neighbor scaler behind display_draw_legacy_xbm/gsc and
// their CMDSSCP "_small" counterparts - same loop, parameterized by
// destination rect instead of the fixed LEGACY_W/H/Y constants, so the
// reduced-size redisplay path doesn't duplicate the pixel-decode logic.
// --- Tint for the legacy monochrome pictures ------------------------------
// The MiSTer's 1bpp/4bpp marquee packs are grayscale; a tint maps each pixel's
// brightness onto a colour instead of white: a fixed colour, a gradient across
// the picture, or a new random colour per picture. Applies only to those legacy
// pictures and to grayscale JPEGs (CMDCORC from the web app, detected per picture); real colour JPEGs are left alone. Off by default; set from
// the device's settings page (display_set_tint()).
uint8_t g_tintMode = 0;     // TintMode in device_settings.h: 0 off, 1 fixed, 2 gradient, 3 random
uint32_t g_tint1 = 0xFFB000;
uint32_t g_tint2 = 0x00A0FF;
uint8_t g_tintDir = 0;      // gradient: 0 top-to-bottom, 1 left-to-right
uint32_t g_tintRandom = 0xFFFFFF; // this picture's colour in random mode, picked by display_new_picture()

inline uint16_t rgb565(uint8_t r, uint8_t g, uint8_t b) {
  return ((r & 0xF8) << 8) | ((g & 0xFC) << 3) | (b >> 3);
}

// Colour for a pixel of brightness `gray` (0-255) at (x, y) within a w x h picture.
uint16_t tintedColor(uint8_t gray, int16_t x, int16_t y, int16_t w, int16_t h) {
  uint32_t base;
  if (g_tintMode == 2) {
    int32_t span = (g_tintDir ? w : h) - 1;
    int32_t pos = g_tintDir ? x : y;
    uint32_t t = span > 0 ? (uint32_t)(pos * 255 / span) : 0;
    auto lerp = [t](uint8_t a, uint8_t b) { return (uint8_t)((a * (255 - t) + b * t) / 255); };
    base = ((uint32_t)lerp(g_tint1 >> 16, g_tint2 >> 16) << 16) |
           ((uint32_t)lerp(g_tint1 >> 8, g_tint2 >> 8) << 8) | lerp(g_tint1, g_tint2);
  } else {
    base = (g_tintMode == 3) ? g_tintRandom : g_tint1;
  }
  return rgb565(((base >> 16) & 0xFF) * gray / 255, ((base >> 8) & 0xFF) * gray / 255, (base & 0xFF) * gray / 255);
}

void drawLegacyScaled(const uint8_t *buf, uint8_t effect, bool isGsc,
                       int16_t dstW, int16_t dstH, int16_t dstX, int16_t dstY) {
  gfx.fillScreenFast(0x0000);
  for (int16_t y = 0; y < dstH; y++) {
    int16_t srcY = (y * LEGACY_SRC_H) / dstH;
    for (int16_t x = 0; x < dstW; x++) {
      int16_t srcX = (x * LEGACY_SRC_W) / dstW;
      uint16_t color;
      if (isGsc) {
        uint8_t byte = buf[(srcX / 2) + srcY * LEGACY_SRC_LINE_4BPP];
        uint8_t nibble = (srcX % 2 == 0) ? (byte >> 4) : (byte & 0x0F); // 4-bit grayscale
        uint8_t gray = nibble * 17; // 0..15 -> 0..255
        color = g_tintMode ? tintedColor(gray, x, y, dstW, dstH)
                           : (((gray >> 3) << 11) | ((gray >> 2) << 5) | (gray >> 3)); // RGB565
      } else {
        uint8_t byte = buf[srcX / 8 + srcY * LEGACY_SRC_LINE_1BPP];
        bool on = bitRead(byte, srcX % 8);
        color = g_tintMode ? tintedColor(on ? 255 : 0, x, y, dstW, dstH) : (on ? 0xFFFF : 0x0000);
      }
      gfx.drawPixel(dstX + x, dstY + y, color);
    }
    if (effect) delay(1); // crude wipe pacing; curated-effect placeholder
  }
}

} // namespace

void display_prealloc() {
#if defined(BOARD_CYD)
  if (g_lastJpeg) return;
  esp_bt_mem_release(ESP_BT_MODE_BTDM); // Bluetooth is never used: hand its reserved RAM to the heap
  g_lastJpeg = static_cast<uint8_t *>(malloc(LAST_JPEG_MAX));
#endif
}

void display_init() {
#if defined(BOARD_CYD)
  display_prealloc();
  if (!g_lastJpeg) Serial.println("FATAL: no memory for the picture copy");
#endif
  pinMode(PIN_LCD_BL, OUTPUT);
  ledcSetup(LEDC_CHANNEL_BL, 5000, 8); // 5kHz, 8-bit duty
  ledcAttachPin(PIN_LCD_BL, LEDC_CHANNEL_BL);
  ledcWrite(LEDC_CHANNEL_BL, g_contrast);

  gfx.begin();
  gfx.fillScreenFast(0x0000);
#if defined(BOARD_CYD)
  // The RGB LED is active low and lit by the floating pins at power-up: switch it off.
  pinMode(PIN_LED_G, OUTPUT); digitalWrite(PIN_LED_G, HIGH);
  pinMode(PIN_LED_B, OUTPUT); digitalWrite(PIN_LED_B, HIGH);
  pinMode(PIN_LCD_RST, OUTPUT); digitalWrite(PIN_LCD_RST, HIGH);
#endif
}

void display_clear() {
  gfx.fillScreenFast(0x0000);
}

void display_flush() {
  // Draws happen immediately; nothing to flush. Present for CMDDUPD compat.
}

void display_set_orientation(uint8_t quarterTurns) {
  g_rotation = quarterTurns & 3;
  gfx.setRotation(g_rotation);
}

void display_set_rotation(uint8_t rot) {
  g_rotation = rot ? 1 : 0;
  gfx.setRotation(g_rotation ? 2 : 0); // 180 degree flip, matches CMDROT semantics
}

void display_set_tint(uint8_t mode, uint32_t color1, uint32_t color2, uint8_t dir) {
  g_tintMode = mode > 3 ? 0 : mode;
  g_tint1 = color1 & 0xFFFFFF;
  g_tint2 = color2 & 0xFFFFFF;
  g_tintDir = dir ? 1 : 0;
  if (g_tintMode == 3) display_new_picture(); // make sure random mode has a colour before the first picture
}

// A new legacy picture is about to be drawn (not a replay of the last one).
// In random mode this is where its colour is chosen: a fully saturated, fully
// bright hue, so a random tint never comes out muddy or dark.
void display_new_picture() {
  if (g_tintMode != 3) return;
  uint32_t hue = esp_random() % 360, x = (60 - abs((int)((hue % 120)) - 60)) * 255 / 60; // 0..255 ramp inside each 120-degree sector
  uint8_t r = 0, g = 0, b = 0;
  switch (hue / 60) {
    case 0: r = 255; g = x; break;
    case 1: r = x; g = 255; break;
    case 2: g = 255; b = x; break;
    case 3: g = x; b = 255; break;
    case 4: r = x; b = 255; break;
    default: r = 255; b = x; break;
  }
  g_tintRandom = ((uint32_t)r << 16) | ((uint32_t)g << 8) | b;
}

void display_set_contrast(uint8_t level) {
  g_contrast = level;
  ledcWrite(LEDC_CHANNEL_BL, g_contrast);
}

void display_on() {
  ledcWrite(LEDC_CHANNEL_BL, g_contrast);
}

void display_off() {
  ledcWrite(LEDC_CHANNEL_BL, 0);
  // Backlight PWM alone is confirmed (on real hardware, 2026-08-13) to
  // have no visible effect on at least one GC9A01 module variant - almost
  // certainly BL wired straight to VCC rather than through a
  // GPIO-controlled transistor, not a software bug (CMDCON's contrast
  // level has the exact same ledcWrite() call and is equally unaffected).
  // Blanking by content works regardless of BL wiring, so do that too;
  // display_on() doesn't restore it directly (display.cpp has no
  // generic "last screen" concept beyond g_frame's JPEG-only content) -
  // protocol.cpp's callers are responsible for redrawing after display_on().
  gfx.fillScreenFast(0x0000);
}

void display_show_start_screen() {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  int16_t x1, y1;
  uint16_t w, h;

  gfx.setTextSize(2);
  gfx.getTextBounds("tty2oled", 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 40);
  gfx.print("tty2oled");

  gfx.setTextSize(1);
  gfx.getTextBounds("GC9A01  v" FW_VERSION, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 5);
  gfx.print("GC9A01  v" FW_VERSION);

  gfx.getTextBounds(REPO_URL, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY + 20);
  gfx.print(REPO_URL);
}

void display_show_corename(const String &name) {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  gfx.setTextSize(2);
  int16_t x1, y1;
  uint16_t w, h;
  gfx.getTextBounds(name, 0, 0, &x1, &y1, &w, &h);
  int16_t x = DISP_CX - w / 2;
  int16_t y = DISP_CY - h / 2;
  if (x < 10) x = 10; // keep inside the circle's safe area
  gfx.setCursor(x, y);
  gfx.print(name);
}

void display_show_error(const String &msg) {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xF800); // red
  gfx.setTextSize(1);
  gfx.setCursor(20, DISP_CY);
  gfx.print(msg);
}

void display_show_notification_text(const String &text) {
  // A banner drawn OVER whatever's already on screen, not a full clear -
  // this is a transient overlay, the caller reverts to prior content
  // afterward (see mqtt_client.h). Filled via beginBatch()/
  // pushRectBatched() row-by-row rather than Adafruit_GFX's default
  // fillRect() (which falls back to one drawPixel()/SPI-transaction per
  // pixel on this driver, since GC9A01Display doesn't override it) -
  // 240x60px of individual-pixel SPI transactions is exactly the kind of
  // per-call transaction cost that stalled the iris/wipe effects before
  // they were batched (see CLAUDE.md's "per-row transaction stall"
  // section).
  constexpr int16_t bannerH = 64;
  const int16_t bannerY = DISP_CY - bannerH / 2;

  static uint16_t rowBuf[DISP_W];
  for (int i = 0; i < DISP_W; i++) rowBuf[i] = 0x0000; // black, byte-swap invariant
  gfx.beginBatch();
  for (int16_t y = 0; y < bannerH; y++) {
    gfx.pushRectBatched(0, bannerY + y, DISP_W, 1, rowBuf);
  }
  gfx.endBatch();

  gfx.setTextColor(0xFFFF);
  gfx.setTextSize(2);
  int16_t x1, y1;
  uint16_t w, h;

  // Single truncated line, no word wrap (v1 scope cut) - keep shortening
  // with an ellipsis until it fits the banner's safe width.
  String shown = text;
  gfx.getTextBounds(shown, 0, 0, &x1, &y1, &w, &h);
  while (w > DISP_W - 30 && shown.length() > 1) {
    shown = shown.substring(0, shown.length() - 1);
    gfx.getTextBounds(shown + "...", 0, 0, &x1, &y1, &w, &h);
  }
  if (shown.length() < text.length()) shown += "...";
  gfx.getTextBounds(shown, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - h / 2);
  gfx.print(shown);
}

void display_draw_text(int16_t x, int16_t y, uint8_t fontSize, const String &text) {
  gfx.setTextColor(0xFFFF);
  gfx.setTextSize(fontSize == 0 ? 1 : fontSize);
  gfx.setCursor(x, y);
  gfx.print(text);
}

void display_draw_rect(int16_t x, int16_t y, int16_t w, int16_t h, bool fill) {
  if (fill) gfx.fillRect(x, y, w, h, 0xFFFF);
  else gfx.drawRect(x, y, w, h, 0xFFFF);
}

void display_draw_circle(int16_t x, int16_t y, int16_t r, bool fill) {
  if (fill) gfx.fillCircle(x, y, r, 0xFFFF);
  else gfx.drawCircle(x, y, r, 0xFFFF);
}

void display_draw_line(int16_t x0, int16_t y0, int16_t x1, int16_t y1) {
  gfx.drawLine(x0, y0, x1, y1, 0xFFFF);
}

void display_draw_legacy_xbm(const uint8_t *buf, uint8_t effect) {
  drawLegacyScaled(buf, effect, false, LEGACY_W, LEGACY_H, 0, LEGACY_Y);
}

void display_draw_legacy_gsc(const uint8_t *buf, uint8_t effect) {
  drawLegacyScaled(buf, effect, true, LEGACY_W, LEGACY_H, 0, LEGACY_Y);
}

void display_draw_legacy_xbm_small(const uint8_t *buf) {
  constexpr int16_t w = LEGACY_W / 2, h = LEGACY_H / 2; // half res each way = ~1/4 area
  drawLegacyScaled(buf, 0, false, w, h, DISP_CX - w / 2, DISP_CY - h / 2);
}

void display_draw_legacy_gsc_small(const uint8_t *buf) {
  constexpr int16_t w = LEGACY_W / 2, h = LEGACY_H / 2;
  drawLegacyScaled(buf, 0, true, w, h, DISP_CX - w / 2, DISP_CY - h / 2);
}

#if defined(BOARD_CYD)
// ---- CYD: no decoded frame; the last picture's JPEG is decoded straight to the glass ----------
// Transitions re-decode the stored JPEG once per step, with a mask (wipes, iris) or a brightness
// scale (fade) applied to each decoded block before it is pushed. Steps follow elapsed time, not
// a fixed count, so a slow decode just means fewer steps and the total stays near durationMs.
bool g_lastGray = false;  // the last picture is all grayscale (decided once, only when tint is on)
bool g_grayBad = false;

enum RenderMode : uint8_t { R_FULL, R_WIPE_LR, R_WIPE_RL, R_WIPE_TB, R_IRIS, R_FADE };
struct RenderCtx {
  RenderMode mode = R_FULL;
  int limit = 0;      // wipes: the revealed column/row limit
  float radius = 0;   // iris
  int t256 = 256;     // fade: 0..256
  bool tint = false;
} g_rc;

// Pushes the part of a decoded block that lies inside [cx0,cx1) x [cy0,cy1) and on the display.
void pushClipped(const uint16_t *px, int x0, int y0, int w, int h, int cx0, int cy0, int cx1, int cy1) {
  int ax0 = max(max(x0, cx0), 0), ay0 = max(max(y0, cy0), 0);
  int ax1 = min(min(x0 + w, cx1), (int)DISP_W), ay1 = min(min(y0 + h, cy1), (int)DISP_H);
  if (ax1 <= ax0 || ay1 <= ay0) return;
  gfx.pushRect(ax0, ay0, ax1 - ax0, ay1 - ay0, px + (ay0 - y0) * w + (ax0 - x0), w);
}

int jpegGrayCheckCallback(JPEGDRAW *pDraw) {
  int n = pDraw->iWidth * pDraw->iHeight;
  for (int i = 0; i < n; i++) {
    uint16_t c = swap16(pDraw->pPixels[i]);
    int r = (c >> 11) << 3, g = ((c >> 5) & 0x3F) << 2, b = (c & 0x1F) << 3;
    if (abs(r - g) > 20 || abs(g - b) > 20 || abs(r - b) > 20) {
      g_grayBad = true;
      return 0; // stop decoding
    }
  }
  return 1;
}

int jpegDrawCallbackCyd(JPEGDRAW *pDraw) {
  const int x0 = pDraw->x, y0 = pDraw->y, w = pDraw->iWidth, h = pDraw->iHeight;
  uint16_t *px = pDraw->pPixels; // the decoder's buffer: ours to modify until it is reused
  if (g_rc.tint) {
    for (int i = 0; i < w * h; i++) {
      uint16_t c = swap16(px[i]);
      int r = (c >> 11) << 3, g = ((c >> 5) & 0x3F) << 2, b = (c & 0x1F) << 3;
      uint8_t gray = (uint8_t)((r * 30 + g * 59 + b * 11) / 100);
      px[i] = swap16(tintedColor(gray, x0 + i % w, y0 + i / w, DISP_W, DISP_H));
    }
  }
  switch (g_rc.mode) {
    case R_FULL:
      pushClipped(px, x0, y0, w, h, 0, 0, DISP_W, DISP_H);
      break;
    case R_FADE:
      for (int i = 0; i < w * h; i++) {
        uint16_t c = swap16(px[i]);
        uint8_t r = ((c >> 11) & 0x1F) * g_rc.t256 / 256;
        uint8_t g = ((c >> 5) & 0x3F) * g_rc.t256 / 256;
        uint8_t b = (c & 0x1F) * g_rc.t256 / 256;
        px[i] = swap16((r << 11) | (g << 5) | b);
      }
      pushClipped(px, x0, y0, w, h, 0, 0, DISP_W, DISP_H);
      break;
    case R_WIPE_LR:
      pushClipped(px, x0, y0, w, h, 0, 0, g_rc.limit, DISP_H);
      break;
    case R_WIPE_RL:
      pushClipped(px, x0, y0, w, h, DISP_W - g_rc.limit, 0, DISP_W, DISP_H);
      break;
    case R_WIPE_TB:
      pushClipped(px, x0, y0, w, h, 0, 0, DISP_W, g_rc.limit);
      break;
    case R_IRIS:
      for (int row = 0; row < h; row++) {
        int y = y0 + row;
        float dy = y - DISP_CY;
        float dxf = sqrtf(max(0.0f, g_rc.radius * g_rc.radius - dy * dy));
        pushClipped(px + row * w, x0, y, w, 1, (int)(DISP_CX - dxf), 0, (int)(DISP_CX + dxf) + 1, DISP_H);
      }
      break;
  }
  return 1;
}

// One full decode of the stored picture through jpegDrawCallbackCyd, with an optional scale.
bool renderLastJpeg(int scaleOpt = 0, int dstX = 0, int dstY = 0) {
  if (!g_lastJpeg || g_lastJpegLen == 0) return false;
  if (!jpeg.openRAM(g_lastJpeg, g_lastJpegLen, jpegDrawCallbackCyd)) return false;
  jpeg.setPixelType(RGB565_BIG_ENDIAN);
  int offX = scaleOpt ? dstX : (DISP_W - jpeg.getWidth()) / 2;
  int offY = scaleOpt ? dstY : (DISP_H - jpeg.getHeight()) / 2;
  jpeg.decode(offX, offY, scaleOpt);
  jpeg.close();
  return true;
}

void cydReveal(uint8_t effect, uint16_t durationMs) {
  g_rc.tint = g_tintMode && g_lastGray;
  if (effect == 0 || durationMs == 0 || effect > 5) {
    g_rc.mode = R_FULL;
    renderLastJpeg();
    return;
  }
  const float maxR = sqrtf((float)(DISP_CX * DISP_CX + DISP_CY * DISP_CY));
  const uint32_t t0 = millis();
  for (;;) {
    uint32_t el = millis() - t0;
    float p = el >= durationMs ? 1.0f : (float)el / durationMs;
    if (p >= 1.0f) { // always finish on the whole picture
      g_rc.mode = R_FULL;
      renderLastJpeg();
      return;
    }
    switch (effect) {
      case 1: g_rc.mode = R_WIPE_LR; g_rc.limit = (int)(DISP_W * p); break;
      case 2: g_rc.mode = R_WIPE_RL; g_rc.limit = (int)(DISP_W * p); break;
      case 3: g_rc.mode = R_WIPE_TB; g_rc.limit = (int)(DISP_H * p); break;
      case 4: // the circle must cover the corners before the end, or they pop in with the final draw
        g_rc.mode = R_IRIS;
        g_rc.radius = maxR * p / 0.85f;
        if (g_rc.radius >= maxR) { // fully open already: skip straight to the final full draw
          g_rc.mode = R_FULL;
          renderLastJpeg();
          return;
        }
        break;
      default: g_rc.mode = R_FADE; g_rc.t256 = (int)(256 * p); break;
    }
    renderLastJpeg();
  }
}

void display_draw_jpeg(const uint8_t *buf, size_t len, uint8_t effect, uint16_t durationMs) {
  if (!g_lastJpeg || len == 0 || len > LAST_JPEG_MAX) {
    display_show_error("Picture too big");
    return;
  }
  display_new_picture(); // random tint: a new colour per picture
  memcpy(g_lastJpeg, buf, len);
  g_lastJpegLen = len;
  g_lastGray = false;
  if (g_tintMode && jpeg.openRAM(g_lastJpeg, len, jpegGrayCheckCallback)) {
    g_grayBad = false;
    jpeg.setPixelType(RGB565_BIG_ENDIAN);
    jpeg.decode(0, 0, 0);
    jpeg.close();
    g_lastGray = !g_grayBad;
  }
  if (!jpeg.openRAM(g_lastJpeg, len, jpegDrawCallbackCyd)) {
    display_show_error("JPEG decode failed");
    g_lastJpegLen = 0;
    return;
  }
  jpeg.close();
  cydReveal(effect, durationMs);
}
#else
// If the tint is on and the decoded picture is grayscale (every pixel has
// R~=G~=B, which is what the marquee app's black-and-white library images are),
// recolour it by brightness like the legacy pictures. Real colour art is left alone.
static void tintGrayFrame() {
  if (!g_tintMode) return;
  for (int y = 0; y < DISP_H; y++) {
    const uint16_t *row = frameRow(y);
    for (int x = 0; x < DISP_W; x++) {
      uint16_t c = swap16(row[x]);
      int r = (c >> 11) << 3, g = ((c >> 5) & 0x3F) << 2, b = (c & 0x1F) << 3;
      if (abs(r - g) > 20 || abs(g - b) > 20 || abs(r - b) > 20) return;
    }
  }
  for (int y = 0; y < DISP_H; y++) {
    for (int x = 0; x < DISP_W; x++) {
      uint16_t c = swap16(frameRow(y)[x]);
      int r = (c >> 11) << 3, g = ((c >> 5) & 0x3F) << 2, b = (c & 0x1F) << 3;
      uint8_t gray = (uint8_t)((r * 30 + g * 59 + b * 11) / 100);
      frameRow(y)[x] = swap16(tintedColor(gray, x, y, DISP_W, DISP_H));
    }
  }
}

void display_draw_jpeg(const uint8_t *buf, size_t len, uint8_t effect, uint16_t durationMs) {
  display_new_picture(); // random tint: a new colour per picture
  clearFrame();
  if (!jpeg.openRAM(const_cast<uint8_t *>(buf), len, jpegDrawCallback)) {
    display_show_error("JPEG decode failed");
    return;
  }
  int imgW = jpeg.getWidth();
  int imgH = jpeg.getHeight();
  jpeg.setPixelType(RGB565_BIG_ENDIAN);
  // Center-crop: the web app is expected to export already-square,
  // circle-safe 240x240 JPEGs, but center anyway to tolerate other sizes.
  int offX = (DISP_W - imgW) / 2;
  int offY = (DISP_H - imgH) / 2;
  jpeg.decode(offX, offY, 0);
  jpeg.close();
  tintGrayFrame();

  transitionReveal(effect, durationMs);
}

#endif

void display_draw_jpeg_transient(const uint8_t *buf, size_t len) {
  if (!jpeg.openRAM(const_cast<uint8_t *>(buf), len, jpegDrawCallbackTransient)) {
    display_show_error("JPEG decode failed");
    return;
  }
  int imgW = jpeg.getWidth();
  int imgH = jpeg.getHeight();
  jpeg.setPixelType(RGB565_BIG_ENDIAN);
  int offX = (DISP_W - imgW) / 2;
  int offY = (DISP_H - imgH) / 2;
  jpeg.decode(offX, offY, 0);
  jpeg.close();
}

#if defined(BOARD_CYD)
void display_replay_jpeg(uint8_t effect, uint16_t durationMs) {
  cydReveal(effect, durationMs); // re-decodes the stored picture; the tint colour stays as it was
}

void display_draw_jpeg_small() {
  constexpr int16_t w = DISP_W / 2, h = DISP_H / 2;
  gfx.fillScreenFast(0x0000);
  g_rc.mode = R_FULL;
  g_rc.tint = g_tintMode && g_lastGray;
  renderLastJpeg(JPEG_SCALE_HALF, (DISP_W - w) / 2, (DISP_H - h) / 2);
}
#else
void display_replay_jpeg(uint8_t effect, uint16_t durationMs) {
  // g_frame already holds the last-decoded CMDCORC frame - no redecode
  // needed. Matches the reference's own CMDSPIC, which also just re-runs
  // its reveal routine over the still-resident picture data rather than
  // clearing first: transitionReveal() only ever draws g_frame's pixels
  // over whatever's already on the glass, so replaying it here reveals
  // the same content it's already showing, exactly as upstream does.
  transitionReveal(effect, durationMs);
}

void display_draw_jpeg_small() {
  // Downsampled directly from g_frame (nearest-neighbor, factor 2) rather
  // than through a new intermediate buffer - a 120x120 scratch buffer
  // would cost another ~28KB of static RAM we don't have to spare (see
  // CLAUDE.md "RAM headroom").
  constexpr int16_t w = DISP_W / 2, h = DISP_H / 2;
  constexpr int16_t dstX = (DISP_W - w) / 2, dstY = (DISP_H - h) / 2;
  gfx.fillScreenFast(0x0000);
  for (int16_t y = 0; y < h; y++) {
    int16_t srcY = y * 2;
    for (int16_t x = 0; x < w; x++) {
      int16_t srcX = x * 2;
      uint16_t c = swap16(frameRow(srcY)[srcX]); // the frame is byte-swapped, see its declaration comment
      gfx.drawPixel(dstX + x, dstY + y, c);
    }
  }
}
#endif

void display_show_bye() {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  gfx.drawCircle(DISP_CX, DISP_CY - 30, 20, 0xFFFF);
  gfx.setTextSize(2);
  int16_t x1, y1;
  uint16_t w, h;
  gfx.getTextBounds("Bye!", 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY + 20);
  gfx.print("Bye!");
}

void display_show_test_pattern() {
  const uint16_t colors[] = {0xF800, 0xFFE0, 0x07E0, 0x07FF, 0x001F, 0xF81F};
  constexpr int rings = 6;
  gfx.fillScreenFast(0x0000);
  for (int i = 0; i < rings; i++) {
    int16_t r = (DISP_CX * (rings - i)) / rings;
    gfx.fillCircle(DISP_CX, DISP_CY, r, colors[i]);
  }
}

void display_show_sysinfo(const String &fwVersion) {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  gfx.setTextSize(1);
  gfx.setCursor(20, DISP_CY - 40);
  gfx.print("SysInfo");
  gfx.setCursor(20, DISP_CY - 20);
  gfx.print("FW: ");
  gfx.print(fwVersion);
  gfx.setCursor(20, DISP_CY);
  gfx.print("Chip: ");
  gfx.print(ESP.getChipModel());
  gfx.setCursor(20, DISP_CY + 20);
  gfx.print("Free heap: ");
  gfx.print(ESP.getFreeHeap());
}

void display_show_clock(const String &timeStr, const String &dateStr) {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  int16_t x1, y1;
  uint16_t w, h;

  gfx.setTextSize(5);
  gfx.getTextBounds(timeStr, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 20);
  gfx.print(timeStr);

  gfx.setTextSize(2);
  gfx.getTextBounds(dateStr, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY + 35);
  gfx.print(dateStr);
}

namespace {
void drawApStatusScreen() {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  int16_t x1, y1;
  uint16_t w, h;

  gfx.setTextSize(2);
  gfx.getTextBounds("AP Mode", 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 40);
  gfx.print("AP Mode");

  gfx.setTextSize(1);
  gfx.getTextBounds(g_apSsid, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 5);
  gfx.print(g_apSsid);

  gfx.getTextBounds(g_apIp, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY + 15);
  gfx.print(g_apIp);

  const char *hint = "Press button for QR";
  gfx.getTextBounds(hint, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY + 60);
  gfx.print(hint);
}

// Renders a WiFi-join QR (open network, per the confirmed AP design) for
// g_apSsid. Builds the module grid into g_frame as scratch (safe to
// clobber - AP mode never has a decoded CMDCORC JPEG on screen at the
// same time) and pushes it in one pushRect() call, rather than one
// fillRect()/SPI-transaction per module - that per-call transaction cost
// is exactly what made the iris/wipe effects stall before they were
// batched (see CLAUDE.md's "per-row transaction stall" section).
void drawWifiQrScreen() {
  String payload = "WIFI:T:nopass;S:" + g_apSsid + ";;";

  QRCode qrcode;
  uint8_t qrData[qrcode_getBufferSize(4)];
  qrcode_initText(&qrcode, qrData, 4, ECC_LOW, payload.c_str());

  // The QR is a square, but this is a ROUND display - a square sized to
  // the naive 200px "safe area" put its corners (exactly where the
  // finder patterns live) past the physical circular bezel, which
  // clipped them on real hardware. A square centered on a circle of
  // radius R fits entirely inside it only up to side <= R*sqrt(2); using
  // a conservative R (not the full 120px display radius) leaves margin
  // for the glass/bezel being a little smaller than the panel's nominal
  // active area. No vertical offset either - shifting the square off
  //-center for the SSID text below only made the clipping worse on one
  // edge; the SSID text is thin and tolerates sitting near the edge far
  // better than the QR's finder-pattern corners do.
  constexpr int kQrSafeRadius = 100;
  int scale = (int)((kQrSafeRadius * 1.4142f) / qrcode.size);
  if (scale < 1) scale = 1;
  int qrPx = qrcode.size * scale;
  int ox = DISP_CX - qrPx / 2;
  int oy = DISP_CY - qrPx / 2;

#if defined(BOARD_CYD)
  // No frame buffer here: build and push one row at a time.
  gfx.fillScreenFast(0x0000);
  static uint16_t qrRow[DISP_W];
  for (int y = 0; y < qrPx; y++) {
    for (int x = 0; x < qrPx; x++) qrRow[x] = qrcode_getModule(&qrcode, x / scale, y / scale) ? 0xFFFF : 0x0000;
    gfx.pushRect(ox, oy + y, qrPx, 1, qrRow);
  }
#else
  clearFrame();
  for (int y = 0; y < qrPx; y++) {
    uint16_t *row = &frameRow(oy + y)[ox];
    for (int x = 0; x < qrPx; x++) {
      // 0x0000/0xFFFF are byte-swap invariant, so no swap16() needed here
      // unlike g_frame's usual JPEG-decoded content.
      row[x] = qrcode_getModule(&qrcode, x / scale, y / scale) ? 0xFFFF : 0x0000;
    }
  }

  gfx.fillScreenFast(0x0000);
  pushFrame(ox, oy, qrPx, qrPx);
#endif

  gfx.setTextColor(0xFFFF);
  gfx.setTextSize(1);
  int16_t x1, y1;
  uint16_t w, h;
  gfx.getTextBounds(g_apSsid, 0, 0, &x1, &y1, &w, &h);
  int16_t textY = oy + qrPx + 16;
  if (textY > DISP_H - 20) textY = DISP_H - 20; // keep inside the circle's safe area
  gfx.setCursor(DISP_CX - w / 2, textY);
  gfx.print(g_apSsid);
}
} // namespace

void display_show_ap_mode(const String &ssid, const String &ip) {
  g_apSsid = ssid;
  g_apIp = ip;
  g_qrShown = false;
  drawApStatusScreen();
}

void display_show_connecting_wifi(const String &ssid) {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  gfx.setTextSize(1);
  int16_t x1, y1;
  uint16_t w, h;
  String line = "Connecting to " + ssid;
  gfx.getTextBounds(line, 0, 0, &x1, &y1, &w, &h);
  int16_t x = w > DISP_W - 20 ? 10 : DISP_CX - w / 2; // keep inside the circle's safe area
  gfx.setCursor(x, DISP_CY);
  gfx.print(line);
}

void display_show_wifi_connected(const String &ssid, const String &ip, const String &hostname) {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  int16_t x1, y1;
  uint16_t w, h;

  gfx.setTextSize(2);
  const char *title = "Connected";
  gfx.getTextBounds(title, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 40);
  gfx.print(title);

  gfx.setTextSize(1);
  gfx.getTextBounds(ssid, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 12);
  gfx.print(ssid);

  gfx.getTextBounds(ip, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY + 8);
  gfx.print(ip);

  gfx.getTextBounds(hostname, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY + 28);
  gfx.print(hostname);
}

void display_show_factory_reset_hold(uint8_t secondsLeft) {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xF800); // red - same warning color as display_show_error()
  int16_t x1, y1;
  uint16_t w, h;

  gfx.setTextSize(1);
  const char *title = "Factory reset";
  gfx.getTextBounds(title, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 40);
  gfx.print(title);

  gfx.setTextSize(3);
  char secBuf[4];
  snprintf(secBuf, sizeof(secBuf), "%u", secondsLeft);
  gfx.getTextBounds(secBuf, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY - 5);
  gfx.print(secBuf);

  gfx.setTextSize(1);
  const char *hint = "Release to cancel";
  gfx.getTextBounds(hint, 0, 0, &x1, &y1, &w, &h);
  gfx.setCursor(DISP_CX - w / 2, DISP_CY + 40);
  gfx.print(hint);
}

void display_show_factory_reset_done() {
  gfx.fillScreenFast(0x0000);
  gfx.setTextColor(0xFFFF);
  gfx.setTextSize(1);
  int16_t x1, y1;
  uint16_t w, h;
  const char *msg = "Factory reset - restarting...";
  gfx.getTextBounds(msg, 0, 0, &x1, &y1, &w, &h);
  int16_t x = w > DISP_W - 20 ? 10 : DISP_CX - w / 2;
  gfx.setCursor(x, DISP_CY);
  gfx.print(msg);
}

bool display_wifi_qr_shown() { return g_qrShown; }

void display_toggle_wifi_qr() {
  g_qrShown = !g_qrShown;
  if (g_qrShown) {
    drawWifiQrScreen();
  } else {
    drawApStatusScreen();
  }
}

void display_self_test() {
  const uint16_t colors[] = {0xF800, 0x07E0, 0x001F, 0xFFE0, 0xF81F, 0x07FF, 0xFFFF, 0x0000};
  const char *names[] = {"RED", "GREEN", "BLUE", "YELLOW", "MAGENTA", "CYAN", "WHITE", "BLACK"};
  int cycle = 0;
  while (true) {
    for (int i = 0; i < 8; i++) {
      gfx.fillScreenFast(colors[i]);
      gfx.setTextColor(colors[i] == 0x0000 ? 0xFFFF : 0x0000);
      gfx.setTextSize(2);
      gfx.setCursor(20, 100);
      gfx.print(names[i]);
      gfx.setTextSize(1);
      gfx.setCursor(20, 130);
      gfx.print("cycle ");
      gfx.print(cycle);
      // Also exercise the exact pushRect() path used for real JPEG frames
      // (not just fillScreenFast's per-row loop), same buffer size as a
      // full 240x240 frame.
#if !defined(BOARD_CYD)
      for (int y = 0; y < DISP_H; y++)
        for (int x = 0; x < DISP_W; x++) frameRow(y)[x] = colors[i];
      pushFrame(0, 0, DISP_W, DISP_H);
#endif
      delay(600);
    }
    cycle++;
  }
}
