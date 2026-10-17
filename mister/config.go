// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the settings read from the stock tty2oled ini files (shell syntax,
// KEY="value") plus the WiFi-specific ones in tty2oled-wifi.ini.
type Config map[string]string

// loadIni merges KEY=value lines into c. Comments, blank lines and anything that is
// not a plain assignment (the system ini also holds shell functions) are skipped.
func (c Config) loadIni(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.ContainsAny(k, " \t(){}$") {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) > 0 && (v[0] == '"' || v[0] == '\'') {
			if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
				v = v[1 : 1+end]
			}
		} else if i := strings.IndexAny(v, " \t#"); i >= 0 {
			v = v[:i]
		}
		c[k] = v
	}
}

func (c Config) str(key, def string) string {
	if v, ok := c[key]; ok && v != "" {
		return v
	}
	return def
}

func (c Config) yes(key string, def bool) bool {
	v, ok := c[key]
	if !ok {
		return def
	}
	return strings.EqualFold(v, "yes")
}

func (c Config) num(key string, def int) int {
	n := def
	if v, ok := c[key]; ok {
		if x, err := atoiSafe(v); err == nil {
			n = x
		}
	}
	return n
}

// stockDir is the stock tty2oled folder (its ini files and picture packs). The daemon
// only reads it and lives in its own folder, so the two can run side by side.
var stockDir = "/media/fat/tty2oled"

const wifiIni = "t2o-wifi.ini"

func loadConfig(base string) Config {
	c := Config{}
	c.loadIni(filepath.Join(stockDir, "tty2oled-system.ini"))
	c.loadIni(filepath.Join(stockDir, "tty2oled-user.ini"))
	c.loadIni(filepath.Join(base, wifiIni))
	return c
}

// setIniValue sets KEY="value" in an ini file, replacing an existing assignment or
// appending one.
func setIniValue(path, key, value string) error {
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	set := false
	for i, l := range lines {
		if k, _, ok := strings.Cut(strings.TrimSpace(l), "="); ok && k == key {
			lines[i] = key + "=\"" + value + "\""
			set = true
		}
	}
	if !set {
		lines = append(lines, key+"=\""+value+"\"")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
