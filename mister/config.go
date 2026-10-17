// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

import (
	"bufio"
	"net"
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

// maxDisplays is how many displays one daemon drives: each is a connection and a picture
// upload per core change, which is kept small on purpose for the MiSTer.
const maxDisplays = 3

// hostList splits HOST (comma, semicolon or space separated) into at most maxDisplays
// distinct addresses; extra is how many were left out.
func hostList(s string) (hosts []string, extra int) {
	seen := map[string]bool{}
	for _, h := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' }) {
		if seen[strings.ToLower(h)] {
			continue
		}
		seen[strings.ToLower(h)] = true
		if len(hosts) == maxDisplays {
			extra++
			continue
		}
		hosts = append(hosts, h)
	}
	return hosts, extra
}

// hostKey is the prefix of a display's own settings in t2o-wifi.ini: the host name without
// ":port", ".local" and the "tty2oled-" prefix, upper case, other characters as "_".
// "tty2oled-CD20.local" gives "CD20" (CD20_CONTRAST="80"); 192.168.86.27 gives "192_168_86_27".
func hostKey(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".local")
	host = strings.TrimPrefix(host, "tty2oled-")
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 32
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '_'
	}, host)
}

// forHost returns the settings for one display: every "<KEY>_NAME" entry overrides "NAME".
// The art (shared by all displays) is not affected, only what is sent to this display.
func (c Config) forHost(host string) Config {
	out := Config{}
	for k, v := range c {
		out[k] = v
	}
	p := hostKey(host) + "_"
	for k, v := range c {
		if strings.HasPrefix(k, p) && len(k) > len(p) {
			out[k[len(p):]] = v
		}
	}
	return out
}
