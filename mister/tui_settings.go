// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// deviceStatus reads GET /status from one display (a small JSON object over plain HTTP, no
// WebSocket slot needed, so it can't disturb the daemon's own connection).
func deviceStatus(host string) (map[string]any, error) {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	addr, err := resolveHost(net.JoinHostPort(h, "80"))
	if err != nil {
		return nil, err
	}
	ip, _, _ := net.SplitHostPort(addr)
	c, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte("GET /status HTTP/1.0\r\nHost: " + ip + "\r\n\r\n")); err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(io.LimitReader(c, 8192))
	i := bytes.IndexByte(body, '{')
	if i < 0 {
		return nil, fmt.Errorf("no status in the reply")
	}
	var st map[string]any
	if err := json.Unmarshal(body[i:], &st); err != nil {
		return nil, fmt.Errorf("unreadable status: %v", err)
	}
	return st, nil
}

// statusLine formats one /status field for the screen ("" when the display doesn't report it).
func statusLine(st map[string]any, key string) string {
	switch v := st[key].(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// displaySettings reads one display's state and sends it live settings. Pick a display
// (skipped when HOST has one), then Up/Down/Left/Right/Enter on the list. Brightness and
// rotation go out as CMDCON/CMDROT and last until the display restarts; its saved defaults
// live on its own /settings page.
func (t *tui) displaySettings() {
	cfg := loadConfig(t.base)
	hosts, _ := hostList(cfg.str("HOST", ""))
	if len(hosts) == 0 || strings.EqualFold(hosts[0], "auto") {
		t.notice = "Pick a display first."
		return
	}
	host := hosts[0]
	if len(hosts) > 1 {
		host = ""
		items := make([]string, 0, len(hosts)+1)
		for _, h := range hosts {
			items = append(items, h)
		}
		items = append(items, "Back")
		sel := 0
		for host == "" {
			w, left := t.begin()
			t.menuBox(4, left, w, "Which display?", items, sel)
			t.end("Up/Down move   Enter/A select   Esc/B back")
			ev, ok := t.wait(time.Hour)
			if !ok || ev.k == kEsc {
				return
			}
			switch ev.k {
			case kUp:
				sel = (sel + len(items) - 1) % len(items)
			case kDown:
				sel = (sel + 1) % len(items)
			case kEnter:
				if sel == len(hosts) {
					return
				}
				host = hosts[sel]
			}
		}
	}
	c := cfg.forHost(host)
	bright := c.num("CONTRAST", 100) // last value sent; the display doesn't report its own
	rotated := c.yes("ROTATE", false)
	var st map[string]any
	var stErr error
	read := func() {
		w, left := t.begin()
		t.frame(4, left, w, 5, "Reading")
		t.at(6, left+3, "Asking "+hostKey(host)+" for its status...")
		t.end("")
		st, stErr = deviceStatus(host)
	}
	read()
	sel := 0
	msg := ""
	for {
		items := []string{
			fmt.Sprintf("%-22s %s", "Brightness (0-255)", bold+strconv.Itoa(bright)+reset),
			fmt.Sprintf("%-22s %s", "Rotate 180 degrees", map[bool]string{true: fgGr + "[x] yes" + reset, false: dim + "[ ] no" + reset}[rotated]),
			"Clear the screen",
			"Read the status again",
			"Back",
		}
		w, left := t.begin()
		t.menuBox(3, left, w, "Display "+strings.TrimSuffix(host, ".local"), items, sel)
		row := 3 + len(items) + 3
		if stErr != nil {
			t.at(row, left+1, fgRd+truncate("Cannot read status: "+stErr.Error(), w-2)+reset)
		} else {
			parts := []string{}
			add := func(label, key, unit string) {
				if v := statusLine(st, key); v != "" {
					parts = append(parts, label+" "+v+unit)
				}
			}
			add("fw", "ver", "")
			add("ip", "ip", "")
			add("wifi", "rssi", " dBm")
			add("up", "uptime", " s")
			t.at(row, left+1, truncate(strings.Join(parts, "  "), w-2))
			if v, ok := st["heap"].(float64); ok {
				t.at(row+1, left+1, fmt.Sprintf("free heap %.0f KB", v/1024))
			}
		}
		if msg != "" {
			t.at(row+3, left+1, fgYl+truncate(msg, w-2)+reset)
		}
		t.at(row+5, left+1, dim+truncate("Live settings last until the display restarts.", w-2)+reset)
		t.end("Up/Down move   Left/Right change   Enter/A select   Esc/B back")
		ev, ok := t.wait(time.Hour)
		if !ok {
			return
		}
		msg = ""
		send := func(line string) {
			dev, err := dialDevice(host)
			if err == nil {
				err = dev.Cmd(line)
				dev.Close()
			}
			if err != nil {
				msg = "Failed: " + err.Error()
			} else {
				msg = "Sent " + line
			}
		}
		switch ev.k {
		case kUp:
			sel = (sel + len(items) - 1) % len(items)
		case kDown:
			sel = (sel + 1) % len(items)
		case kEsc:
			return
		case kLeft, kRight, kEnter:
			step := 1
			if ev.k == kLeft {
				step = -1
			}
			switch sel {
			case 0:
				n := bright + step*16
				if ev.k == kEnter {
					v, ok := t.input("Brightness", "0-255", strconv.Itoa(bright))
					if !ok {
						continue
					}
					m, err := strconv.Atoi(v)
					if err != nil || m < 0 || m > 255 {
						msg = "Brightness must be 0-255."
						continue
					}
					n = m
				}
				if n < 0 {
					n = 0
				}
				if n > 255 {
					n = 255
				}
				bright = n
				send(fmt.Sprintf("CMDCON,%d", bright))
			case 1:
				rotated = !rotated
				send(fmt.Sprintf("CMDROT,%d", boolInt(rotated)))
			case 2:
				if ev.k == kEnter {
					send("CMDCLS")
				}
			case 3:
				if ev.k == kEnter {
					read()
				}
			case 4:
				if ev.k == kEnter {
					return
				}
			}
		}
	}
}
