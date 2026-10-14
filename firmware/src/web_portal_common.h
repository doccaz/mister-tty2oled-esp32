// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

#pragma once

#include <Arduino.h>

// The portal's shared look (dark theme CSS + <head>) and HTML escaping, used by
// both web_portal.cpp (setup/status page) and web_portal_settings.cpp (the
// settings page).
extern const char PAGE_HEAD[] PROGMEM;
extern const char PAGE_TAIL[] PROGMEM;
String htmlEscape(const String &s);
