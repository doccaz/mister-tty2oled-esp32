// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// The device settings page: live status, display/idle/tint defaults, time (NTP
// + timezone), the optional admin password and changing the WiFi network. It is
// served by the device itself (http://<device>/settings once on WiFi) rather
// than by the marquee web app, because an https-hosted page can't reach a plain
// http device on the LAN. The page is streamed in pieces: with ~30 KB of free
// heap, building it as one String would be asking for trouble.

#include "web_portal.h"
#include "web_portal_common.h"
#include "device_settings.h"
#include "protocol.h"
#include "time_manager.h"
#include "version.h"
#include "diag.h"
#include <WiFi.h>
#include <lwip/sockets.h>
#include <errno.h>
#include <lwip/tcp.h>
#include <netinet/tcp.h>

namespace {

struct TzEntry {
  const char *label;
  const char *posix;
};

// Common zones with their daylight-saving rules (POSIX TZ strings). Anything
// else can be typed into the field below the list.
const TzEntry kZones[] = {
    {"UTC", "UTC0"},
    {"Europe/London, Lisbon (UK, Portugal)", "GMT0BST,M3.5.0/1,M10.5.0"},
    {"Europe/Berlin, Paris, Madrid, Rome, Amsterdam", "CET-1CEST,M3.5.0,M10.5.0/3"},
    {"Europe/Athens, Helsinki, Kyiv", "EET-2EEST,M3.5.0/3,M10.5.0/4"},
    {"Europe/Moscow", "MSK-3"},
    {"Europe/Istanbul", "<+03>-3"},
    {"Africa/Cairo", "EET-2EEST,M4.5.5/0,M10.5.4/24"},
    {"Africa/Johannesburg", "SAST-2"},
    {"Africa/Lagos", "WAT-1"},
    {"Africa/Nairobi", "EAT-3"},
    {"Asia/Dubai", "<+04>-4"},
    {"Asia/Karachi", "PKT-5"},
    {"Asia/Kolkata (India)", "IST-5:30"},
    {"Asia/Dhaka", "<+06>-6"},
    {"Asia/Bangkok, Jakarta", "<+07>-7"},
    {"Asia/Singapore, Kuala Lumpur, Perth", "<+08>-8"},
    {"Asia/Hong Kong, Shanghai, Taipei, Manila", "CST-8"},
    {"Asia/Tokyo, Seoul", "JST-9"},
    {"Australia/Adelaide", "ACST-9:30ACDT,M10.1.0,M4.1.0/3"},
    {"Australia/Brisbane", "AEST-10"},
    {"Australia/Sydney, Melbourne", "AEST-10AEDT,M10.1.0,M4.1.0/3"},
    {"Pacific/Auckland", "NZST-12NZDT,M9.5.0,M4.1.0/3"},
    {"Pacific/Honolulu", "HST10"},
    {"America/Anchorage", "AKST9AKDT,M3.2.0,M11.1.0"},
    {"America/Los_Angeles, Vancouver", "PST8PDT,M3.2.0,M11.1.0"},
    {"America/Phoenix", "MST7"},
    {"America/Denver", "MST7MDT,M3.2.0,M11.1.0"},
    {"America/Chicago", "CST6CDT,M3.2.0,M11.1.0"},
    {"America/Mexico_City", "CST6"},
    {"America/New_York, Toronto", "EST5EDT,M3.2.0,M11.1.0"},
    {"America/Bogota, Lima", "<-05>5"},
    {"America/Caracas", "<-04>4"},
    {"America/Halifax", "AST4ADT,M3.2.0,M11.1.0"},
    {"America/Santiago", "<-04>4<-03>,M9.1.6/24,M4.1.6/24"},
    {"America/St_Johns", "NST3:30NDT,M3.2.0,M11.1.0"},
    {"America/Sao_Paulo (Brazil)", "<-03>3"},
    {"America/Argentina/Buenos_Aires", "<-03>3"},
    {"America/Montevideo", "<-03>3"},
};
constexpr size_t kZoneCount = sizeof(kZones) / sizeof(kZones[0]);

String hexColor(uint32_t c) {
  char b[8];
  snprintf(b, sizeof(b), "#%06x", (unsigned)(c & 0xFFFFFF));
  return String(b);
}

uint32_t parseHexColor(const String &s, uint32_t fallback) {
  if (s.length() != 7 || s[0] != '#') return fallback;
  char *end = nullptr;
  unsigned long v = strtoul(s.c_str() + 1, &end, 16);
  return (end && *end == '\0') ? (uint32_t)v : fallback;
}

uint16_t clampArg(const String &s, uint16_t lo, uint16_t hi, uint16_t fallback) {
  if (s.length() == 0) return fallback;
  long v = s.toInt();
  if (v < lo) v = lo;
  if (v > hi) v = hi;
  return (uint16_t)v;
}

const char SETTINGS_CSS[] PROGMEM =
    "<style>"
    "body{max-width:560px}"
    ".card{background:var(--card);border:1px solid var(--border);border-radius:10px;padding:.2em 1em 1em;margin:1em 0}"
    ".card h1{margin-top:.8em}"
    ".grid{display:grid;grid-template-columns:auto 1fr;gap:.2em 1em;font-size:.92em}"
    ".grid span:nth-child(odd){color:var(--muted)}"
    ".two{display:grid;grid-template-columns:1fr 1fr;gap:.6em}"
    "input[type=color]{padding:.1em;height:2.4em}"
    "input[type=range]{padding:0}"
    ".ok{color:var(--accent2)}.warn{color:#f0c36b}"
    "nav a{color:var(--accent2);margin-right:1em}"
    "</style>";

// WiFiClient::write() blocks in select() and was seen hanging the whole loop
// task for minutes mid-page (device unreachable except ping). This sends with
// MSG_DONTWAIT and a hard deadline, and drops the connection on timeout.
} // namespace

namespace {
const char SETTINGS_JS[] PROGMEM =
    "function g(i){return document.getElementById(i)}"
    "function refresh(){fetch('/status').then(function(r){return r.json()}).then(function(j){"
    "g('s_ver').textContent=j.ver;g('s_name').textContent=j.name+'.local';"
    "g('s_up').textContent=Math.floor(j.uptime/86400)+'d '+Math.floor(j.uptime%86400/3600)+'h '+Math.floor(j.uptime%3600/60)+'m';"
    "g('s_wifi').textContent=j.ssid+' ('+j.rssi+' dBm)';g('s_ip').textContent=j.ip;"
    "g('s_time').textContent=(j.time||'not set yet')+' \\u00b7 NTP '+j.ntp;"
    "g('s_stage').textContent=j.stage;g('s_mqtt').textContent=j.mqtt;"
    "g('s_heap').textContent=Math.round(j.heap/1024)+' KB (largest block '+Math.round(j.heap_max/1024)+' KB)'"
    "}).catch(function(){})}"
    "refresh();setInterval(refresh,3000);"
    "function doScan(){var s=g('ssidList');s.style.display='block';s.innerHTML='<option>Scanning...</option>';"
    "fetch('/scan').then(function(r){return r.json()}).then(function(j){"
    "s.innerHTML='<option value=\"\">Select a network...</option>';"
    "j.networks.forEach(function(n){var o=document.createElement('option');o.value=n.ssid;"
    "o.textContent=(n.open?'':'\\uD83D\\uDD12 ')+n.ssid+' ('+n.rssi+'dBm)';s.appendChild(o)})"
    "}).catch(function(){s.innerHTML='<option>Scan failed</option>'})}"
    "var P=[1,2,3,4,5];function done(){g('zone').addEventListener('change',function(){if(this.value)g('tz').value=this.value});g('ssidList').addEventListener('change',function(){if(this.value)g('ssid').value=this.value})}function load(i){if(i>=P.length){done();return}fetch('/settings-part?p='+P[i]).then(function(r){return r.text()}).then(function(h){g('p'+P[i]).innerHTML=h;load(i+1)}).catch(function(){setTimeout(function(){load(i)},1500)})}load(0);";
} // namespace

// The page is served as a small shell plus five fragments that the script in
// /settings.js loads one after another. Every response stays under the TCP send
// buffer (~5.7 KB), so the handler returns without waiting for ACKs: one big
// chunked page stalled on this device's marginal WiFi and blocked the loop.
void WebPortal::handleSettings() {
  if (!authorized()) return;
  crumb(C_SETTINGS_BEGIN);
  String s;
  s.reserve(3600);
  s += FPSTR(PAGE_HEAD);
  s += FPSTR(SETTINGS_CSS);
  auto out = [&s](const String &piece) { s += piece; };
  out(F("<nav><a href='/'>WiFi &amp; MQTT</a><b>Settings</b></nav>"
        "<h1>tty2oled settings</h1>"));
  if (_server.hasArg("saved")) out(F("<p class='ok'>Saved.</p>"));
  out(F("<div class='card'><h1>Status</h1><div class='grid'>"
                        "<span>Firmware</span><span id='s_ver'>" FW_VERSION "</span>"
                        "<span>Device</span><span id='s_name'>-</span>"
                        "<span>Uptime</span><span id='s_up'>-</span>"
                        "<span>WiFi</span><span id='s_wifi'>-</span>"
                        "<span>IP address</span><span id='s_ip'>-</span>"
                        "<span>Time</span><span id='s_time'>-</span>"
                        "<span>Idle stage</span><span id='s_stage'>-</span>"
                        "<span>MQTT</span><span id='s_mqtt'>-</span>"
                        "<span>Free memory</span><span id='s_heap'>-</span>"
                        "</div></div>"));

  out(F("<form method='POST' action='/settings-save'>"
        "<div id='p1'></div><div id='p2'></div><div id='p3'></div><div id='p4'></div></form>"
        "<div id='p5'></div><script src='/settings.js'></script>"));
  s += FPSTR(PAGE_TAIL);
  crumb(C_SETTINGS_END);
  _server.send(200, "text/html", s);
}

void WebPortal::handleSettingsJs() {
  if (!authorized()) return;
  _server.send_P(200, "application/javascript", SETTINGS_JS);
}

void WebPortal::handleSettingsPart() {
  if (!authorized()) return;
  crumb(C_SETTINGS_FLUSH);
  const DeviceSettings &c = settingsStore.cfg;
  int part = _server.arg("p").toInt();
  String s;
  s.reserve(3600);
  auto out = [&s](const String &piece) { s += piece; };
  switch (part) {
    case 1: {
  // ---- Display
  out(F("<div class='card'><h1>Display</h1>"
                        "<label>Brightness: <b id='cv'>"));
  out(String(c.contrast));
  out(F("</b></label><input type='range' name='contrast' min='0' max='255' value='"));
  out(String(c.contrast));
  out(F("' oninput=\"cv.textContent=this.value\">"
                        "<label>Orientation</label><select name='rotation'><option value='0'"));
  out(c.rotation ? "" : " selected");
  out(F(">Normal</option><option value='1'"));
  out(c.rotation ? " selected" : "");
  out(F(">Rotated 180&deg;</option></select>"
                        "<p class='muted'>Saved values are the defaults at power-up. A MiSTer running "
                        "tty2oled.sh sends its own brightness/rotation and those win until the next reboot.</p></div>"));

  // ---- Idle
  out(F("<div class='card'><h1>When idle</h1>"
                        "<label>Show the clock after (minutes idle, 0 = never)</label>"
                        "<input type='number' name='clockMin' min='0' max='1440' value='"));
  out(String(c.clockIdleMin));
  out(F("'><label>Then blank the display after a further (minutes, 0 = never)</label>"
                        "<input type='number' name='blankMin' min='0' max='1440' value='"));
  out(String(c.blankMin));
  out(F("'><label>Blank the status OLED after (seconds idle, 0 = never)</label>"
                        "<input type='number' name='oledSec' min='0' max='3600' value='"));
  out(String(c.oledIdleSec));
  out(F("'><p class='muted'>Any command from the MiSTer, WiFi or MQTT, or a press of the "
                        "BOOT button, brings the picture back. The clock needs the time from NTP "
                        "(below) or from the MiSTer. A MiSTer with its screensaver set to &ldquo;no&rdquo; "
                        "turns the blank step off at start-up; the clock is not affected.</p></div>"));

      break;
    }
    case 2: {
  // ---- Tint
  out(F("<div class='card'><h1>Tint for monochrome pictures</h1>"
                        "<label>Mode</label><select name='tintMode'>"));
  const char *modes[] = {"Off", "Fixed colour", "Gradient", "Random colour per picture"};
  for (int i = 0; i < 4; i++) {
    out("<option value='" + String(i) + "'" + (c.tintMode == i ? " selected" : "") + ">" + modes[i] + "</option>");
  }
  out(F("</select><div class='two'><div><label>Colour</label>"
                        "<input type='color' name='tint1' value='"));
  out(hexColor(c.tintColor1));
  out(F("'></div><div><label>Gradient end</label><input type='color' name='tint2' value='"));
  out(hexColor(c.tintColor2));
  out(F("'></div></div><label>Gradient direction</label><select name='tintDir'><option value='0'"));
  out(c.tintDir ? "" : " selected");
  out(F(">Top to bottom</option><option value='1'"));
  out(c.tintDir ? " selected" : "");
  out(F(">Left to right</option></select>"
                        "<p class='muted'>Colours black-and-white marquee pictures "
                        "(the MiSTer's .gsc / .xbm, and grayscale pictures from the web app) by their brightness. Random picks a new bright colour for every "
                        "picture. Full-colour pictures are left alone.</p></div>"));

      break;
    }
    case 3: {
  // ---- Time
  out(F("<div class='card'><h1>Time</h1>"
                        "<label><input type='checkbox' name='ntp' value='1'"));
  out(c.ntpEnabled ? " checked" : "");
  out(F("> Set the clock from the internet (NTP)</label>"
                        "<label>NTP server</label><input name='ntpServer' maxlength='47' value='"));
  out(htmlEscape(c.ntpServer));
  out(F("'><label>Timezone</label><select id='zone'>"));
  bool known = false;
  for (size_t i = 0; i < kZoneCount; i++) {
    bool sel = strcmp(c.tz, kZones[i].posix) == 0 && !known; // first match wins (several zones share a rule)
    if (sel) known = true;
    out("<option value='" + htmlEscape(kZones[i].posix) + "'" + (sel ? " selected" : "") + ">" +
                        htmlEscape(kZones[i].label) + "</option>");
  }
  out(String("<option value=''") + (known ? "" : " selected") + ">Custom (POSIX TZ, below)</option></select>");
  out(F("<label>POSIX TZ string</label><input name='tz' id='tz' maxlength='47' value='"));
  out(htmlEscape(c.tz));
  out(F("'><p class='muted'>Without internet the clock falls back to the time the MiSTer "
                        "sends, if any. The device has no battery clock, so it starts without a time "
                        "until NTP answers.</p></div>"));

      break;
    }
    case 4: {
  // ---- Security
  out(F("<div class='card'><h1>Access</h1>"
                        "<p class='muted'>Optional password for this page, the WiFi/MQTT page and resets "
                        "(user name <code>admin</code>). Sent over plain HTTP, so it keeps casual visitors "
                        "out; it is not protection against someone watching your network.</p>"
                        "<label>New password (leave empty to keep the current one)</label>"
                        "<input type='password' name='adminPass' maxlength='31' autocomplete='new-password'>"));
  if (c.adminPass[0]) out(F("<label><input type='checkbox' name='adminClear' value='1'> Remove the password</label>"));
  out(F("</div><button type='submit'>Save settings</button>"));

      break;
    }
    case 5: {
  // ---- WiFi
  out(F("<div class='card'><h1>WiFi network</h1><p class='muted'>Connected to <b>"));
  out(htmlEscape(WiFi.SSID()));
  out(F("</b>. Joining a different network restarts the device; if it can't join, it falls back "
                        "to its own setup network and keeps retrying the saved one.</p>"
                        "<button type='button' class='secondary' style='margin-left:0' onclick='doScan()'>Scan for networks</button>"
                        "<select id='ssidList' style='display:none'></select>"
                        "<form method='POST' action='/save'><label>Network name (SSID)</label>"
                        "<input name='ssid' id='ssid' maxlength='63' required><label>Password</label>"
                        "<input name='pass' type='password' maxlength='63'>"
                        "<button type='submit'>Save &amp; reconnect</button></form></div>"));

      break;
    }
    default:
      _server.send(404, "text/plain", "no such part");
      return;
  }
  crumb(C_SETTINGS_END);
  _server.send(200, "text/html", s);
}

void WebPortal::handleSettingsSave() {
  if (!authorized()) return;
  crumb(C_SAVE_BEGIN);
  DeviceSettings &c = settingsStore.cfg;

  c.contrast = (uint8_t)clampArg(_server.arg("contrast"), 0, 255, c.contrast);
  c.rotation = _server.arg("rotation") == "1" ? 1 : 0;
  c.clockIdleMin = clampArg(_server.arg("clockMin"), 0, 1440, c.clockIdleMin);
  c.blankMin = clampArg(_server.arg("blankMin"), 0, 1440, c.blankMin);
  c.oledIdleSec = clampArg(_server.arg("oledSec"), 0, 3600, c.oledIdleSec);

  c.tintMode = (uint8_t)clampArg(_server.arg("tintMode"), 0, 3, c.tintMode);
  c.tintColor1 = parseHexColor(_server.arg("tint1"), c.tintColor1);
  c.tintColor2 = parseHexColor(_server.arg("tint2"), c.tintColor2);
  c.tintDir = _server.arg("tintDir") == "1" ? 1 : 0;

  c.ntpEnabled = _server.hasArg("ntp");
  String srv = _server.arg("ntpServer");
  srv.trim();
  if (srv.length() > 0 && srv.length() < sizeof(c.ntpServer)) strncpy(c.ntpServer, srv.c_str(), sizeof(c.ntpServer));
  String tz = _server.arg("tz");
  tz.trim();
  if (tz.length() > 0 && tz.length() < sizeof(c.tz)) strncpy(c.tz, tz.c_str(), sizeof(c.tz));

  if (_server.hasArg("adminClear")) {
    c.adminPass[0] = '\0';
  } else {
    String pw = _server.arg("adminPass");
    if (pw.length() > 0 && pw.length() < sizeof(c.adminPass)) strncpy(c.adminPass, pw.c_str(), sizeof(c.adminPass));
  }

  crumb(C_SAVE_NVS);
  settingsStore.save();
  crumb(C_SAVE_APPLY);
  protocol_apply_settings();
  crumb(C_SAVE_TIME);
  time_apply_settings();
  crumb(C_SAVE_REDISPLAY);
  protocol_redisplay_current(0);
  crumb(C_SAVE_DONE); // so a changed orientation or tint shows straight away

  _server.sendHeader("Location", "/settings?saved=1");
  _server.send(303, "text/plain", "");
}

