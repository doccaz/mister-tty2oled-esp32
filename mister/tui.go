// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

//go:build linux

package main

// The configuration menu (tty2oled-wifi -menu): plain ANSI, no libraries. It draws a status
// panel and a menu, reads the arrow keys / Enter / Esc that the MiSTer maps its d-pad and
// A/B buttons to, and edits tty2oled-wifi.ini and the start-at-boot line.

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	fgCy  = "\x1b[96m"
	fgGr  = "\x1b[92m"
	fgRd  = "\x1b[91m"
	fgYl  = "\x1b[93m"
	fgWh  = "\x1b[97m"
	bgBl  = "\x1b[44m"
	bgSel = "\x1b[46;30m"
)

type key int

const (
	kNone key = iota
	kUp
	kDown
	kLeft
	kRight
	kEnter
	kEsc
	kBackspace
	kRune
)

type keyEvent struct {
	k key
	r rune
}

type box struct{ tl, tr, bl, br, h, v string }

var uniBox = box{"┌", "┐", "└", "┘", "─", "│"}
var ascBox = box{"+", "+", "+", "+", "-", "|"}

// linkState is the last reachability check of one display.
type linkState struct {
	host, state, ip string // state: "", "online", "offline"
}

type tui struct {
	base, startup, initPath, logPath, pidPath string
	version                                   string
	w, h                                      int
	b                                         box
	dot                                       string
	keys                                      chan keyEvent
	mu                                        sync.Mutex
	links                                     []linkState // one per display, "" state = not checked yet
	notice                                    string
	out                                       *strings.Builder
}

func readKeys(ch chan<- keyEvent) {
	buf := make([]byte, 16)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			close(ch)
			return
		}
		b := buf[:n]
		// one read can hold several keys (key repeat, pasted input): split them
		for len(b) > 0 {
			switch {
			case b[0] == 27 && len(b) >= 3 && (b[1] == '[' || b[1] == 'O'):
				switch b[2] {
				case 'A':
					ch <- keyEvent{k: kUp}
				case 'B':
					ch <- keyEvent{k: kDown}
				case 'C':
					ch <- keyEvent{k: kRight}
				case 'D':
					ch <- keyEvent{k: kLeft}
				}
				b = b[3:]
			case b[0] == 27:
				ch <- keyEvent{k: kEsc}
				b = b[1:]
			case b[0] == 13 || b[0] == 10:
				ch <- keyEvent{k: kEnter}
				b = b[1:]
			case b[0] == 127 || b[0] == 8:
				ch <- keyEvent{k: kBackspace}
				b = b[1:]
			default:
				r, size := utf8.DecodeRune(b)
				if r >= 32 {
					ch <- keyEvent{k: kRune, r: r}
				}
				b = b[size:]
			}
		}
	}
}

// ---- drawing ---------------------------------------------------------------------------

func (t *tui) at(r, c int, s string) { fmt.Fprintf(t.out, "\x1b[%d;%dH%s", r, c, s) }

func (t *tui) rep(s string, n int) string {
	if n < 0 {
		n = 0
	}
	return strings.Repeat(s, n)
}

func (t *tui) frame(r, c, w, h int, title string) {
	t.at(r, c, fgCy+t.b.tl+t.b.h+" "+bold+title+reset+fgCy+" "+t.rep(t.b.h, w-len(title)-5)+t.b.tr+reset)
	for i := 1; i < h-1; i++ {
		t.at(r+i, c, fgCy+t.b.v+reset+t.rep(" ", w-2)+fgCy+t.b.v+reset)
	}
	t.at(r+h-1, c, fgCy+t.b.bl+t.rep(t.b.h, w-2)+t.b.br+reset)
}

// vlen is the printed length of s (without colour codes).
func vlen(s string) int {
	n, esc := 0, false
	for _, r := range s {
		switch {
		case r == 27:
			esc = true
		case esc && r == 'm':
			esc = false
		case !esc:
			n++
		}
	}
	return n
}

func (t *tui) layout() (w, left int) {
	t.w, t.h = termSize()
	w = t.w - 4
	if w > 72 {
		w = 72
	}
	if w < 40 {
		w = 40
	}
	return w, (t.w-w)/2 + 1
}

func (t *tui) begin() (w, left int) {
	t.out = &strings.Builder{}
	w, left = t.layout()
	t.out.WriteString("\x1b[?25l\x1b[2J")
	title := " tty2oled over WiFi"
	ver := t.version + " "
	t.at(1, 1, bgBl+fgWh+bold+title+t.rep(" ", t.w-len(title)-len(ver))+ver+reset)
	return
}

func (t *tui) end(footer string) {
	t.at(t.h, 1, dim+" "+footer+reset)
	os.Stdout.WriteString(t.out.String())
}

func (t *tui) status(w, left int) {
	c := loadConfig(t.base)
	hostStr := c.str("HOST", "(not set)")
	run := t.running()
	t.frame(3, left, w, 7, "Status")
	t.at(4, left+2, bold+"Display "+reset+truncate(hostStr, w-14))
	links := t.getLinks()
	hosts, _ := hostList(c.str("HOST", ""))
	var parts []string
	for _, h := range hosts {
		st := ""
		for _, l := range links {
			if l.host == h {
				st = l.state
			}
		}
		name := hostKey(h)
		switch st {
		case "online":
			parts = append(parts, fgGr+t.dot+reset+" "+name)
		case "offline":
			parts = append(parts, fgRd+t.dot+reset+" "+name)
		default:
			parts = append(parts, dim+t.dot+" "+name+reset)
		}
	}
	switch {
	case len(hosts) == 0 || strings.EqualFold(hostStr, "auto"):
		t.at(5, left+2, bold+"Link    "+reset+dim+"no display chosen"+reset)
	case len(hosts) == 1 && len(links) == 1 && links[0].state == "online":
		t.at(5, left+2, bold+"Link    "+reset+fgGr+t.dot+" reachable"+reset+dim+"  "+links[0].ip+reset)
	case len(hosts) == 1 && len(links) == 1 && links[0].state == "offline":
		t.at(5, left+2, bold+"Link    "+reset+fgRd+t.dot+" not reachable"+reset)
	default:
		t.at(5, left+2, bold+"Link    "+reset+strings.Join(parts, "  "))
	}
	if run {
		t.at(6, left+2, bold+"Daemon  "+reset+fgGr+t.dot+" running"+reset)
	} else {
		t.at(6, left+2, bold+"Daemon  "+reset+fgRd+t.dot+" stopped"+reset)
	}
	boot := "no"
	if t.atBoot() {
		boot = "yes"
	}
	core := "-"
	if b, err := os.ReadFile("/tmp/CORENAME"); err == nil && strings.TrimSpace(string(b)) != "" {
		core = strings.TrimSpace(string(b))
	}
	t.at(7, left+2, bold+"At boot "+reset+boot+"      "+bold+"Core "+reset+core)
	t.at(8, left+2, dim+truncate(t.lastLog(), w-4)+reset)
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n && n > 1 {
		return string(r[:n-1]) + "~"
	}
	return s
}

// menuBox draws items with the selected row highlighted.
func (t *tui) menuBox(r, left, w int, title string, items []string, sel int) {
	t.frame(r, left, w, len(items)+2, title)
	for i, it := range items {
		line := " " + it
		line += t.rep(" ", w-4-vlen(line))
		if i == sel {
			t.at(r+1+i, left+1, bgSel+bold+" "+line+" "+reset)
		} else {
			t.at(r+1+i, left+1, " "+line+" ")
		}
	}
}

// ---- state -----------------------------------------------------------------------------

func (t *tui) getLinks() []linkState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]linkState(nil), t.links...)
}

func (t *tui) running() bool {
	b, err := os.ReadFile(t.pidPath)
	if err != nil {
		return false
	}
	_, err = os.Stat("/proc/" + strings.TrimSpace(string(b)))
	return err == nil
}

func (t *tui) lastLog() string {
	b, err := os.ReadFile(t.logPath)
	if err != nil || len(b) == 0 {
		return "no log yet"
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	l := lines[len(lines)-1]
	if len(l) > 20 && l[4] == '/' { // drop the date and time
		l = l[20:]
	}
	return l
}

func (t *tui) atBoot() bool {
	b, _ := os.ReadFile(t.startup)
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "S60t2o-wifi") && !strings.HasPrefix(strings.TrimSpace(l), "#") {
			return true
		}
	}
	return false
}

// probe checks the display every few seconds: a TCP connect to its WebSocket port.
func (t *tui) probe(stop <-chan struct{}) {
	for {
		hosts, _ := hostList(loadConfig(t.base).str("HOST", ""))
		var links []linkState
		for _, host := range hosts {
			if strings.EqualFold(host, "auto") {
				continue
			}
			state, ip := "offline", ""
			hp := host
			if !strings.Contains(hp, ":") {
				hp += ":81"
			}
			addr := lastGood[hp]
			if addr == "" {
				addr, _ = resolveHost(hp)
			}
			if addr != "" {
				c, err := net.DialTimeout("tcp", addr, 2*time.Second)
				if err != nil && lastGood[hp] != "" { // the address may have changed
					delete(lastGood, hp)
					if a, e := resolveHost(hp); e == nil {
						lastGood[hp] = a
						c, err = net.DialTimeout("tcp", a, 2*time.Second)
						addr = a
					}
				}
				if err == nil {
					c.Close()
					state, ip = "online", addr
					if hp != addr {
						lastGood[hp] = addr
					}
				}
			}
			links = append(links, linkState{host, state, ip})
		}
		t.mu.Lock()
		t.links = links
		t.mu.Unlock()
		select {
		case <-stop:
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// ---- screens ---------------------------------------------------------------------------

func (t *tui) wait(d time.Duration) (keyEvent, bool) {
	select {
	case k, ok := <-t.keys:
		return k, ok
	case <-time.After(d):
		return keyEvent{}, true
	}
}

func (t *tui) mainMenu() {
	sel := 0
	for {
		boot := "Start at boot: turn on"
		if t.atBoot() {
			boot = "Start at boot: turn off"
		}
		run := "Start daemon"
		if t.running() {
			run = "Stop daemon"
		}
		items := []string{
			"Find a display on the network",
			run,
			"Restart daemon",
			boot,
			"Options",
			"Send a test picture",
			"View log",
			"Quit",
		}
		w, left := t.begin()
		t.status(w, left)
		t.menuBox(10, left, w, "Menu", items, sel)
		if t.notice != "" {
			t.at(10+len(items)+3, left+1, fgYl+t.notice+reset)
		}
		t.end("Up/Down move   Enter/A select   Esc/B quit")
		ev, ok := t.wait(1500 * time.Millisecond)
		if !ok {
			return
		}
		t.notice = ""
		switch ev.k {
		case kUp:
			sel = (sel + len(items) - 1) % len(items)
		case kDown:
			sel = (sel + 1) % len(items)
		case kEsc:
			return
		case kEnter:
			switch sel {
			case 0:
				t.pickDisplay()
			case 1:
				if t.running() {
					t.notice = t.initDo("stop")
				} else {
					t.notice = t.initDo("start")
				}
			case 2:
				t.notice = t.initDo("restart")
			case 3:
				t.notice = t.toggleBoot()
			case 4:
				t.options()
			case 5:
				t.sendTest()
			case 6:
				t.viewLog()
			case 7:
				return
			}
		}
	}
}

func (t *tui) initDo(action string) string {
	if err := exec.Command(t.initPath, action).Run(); err != nil {
		return "Could not run " + filepath.Base(t.initPath) + " " + action + ": " + err.Error()
	}
	time.Sleep(time.Second)
	if action == "stop" {
		return "Daemon stopped."
	}
	if t.running() {
		return "Daemon running."
	}
	return "The daemon did not stay running - see the log."
}

// pickDisplay scans the network and lets the user choose; 'm' types an address instead.
func (t *tui) pickDisplay() {
	type result struct {
		list []DisplayInfo
		err  error
	}
	done := make(chan result, 1)
	go func() { l, e := findDisplays(); done <- result{l, e} }()
	frames := []string{"|", "/", "-", "\\"}
	var res result
	for i := 0; ; i++ {
		w, left := t.begin()
		t.frame(4, left, w, 5, "Searching")
		t.at(6, left+3, fgCy+frames[i%4]+reset+" Looking for displays on the network...")
		t.end("Esc/B cancel")
		select {
		case res = <-done:
		case ev, ok := <-t.keys:
			if !ok || ev.k == kEsc {
				return
			}
			continue
		case <-time.After(150 * time.Millisecond):
			continue
		}
		break
	}
	if res.err != nil {
		t.notice = res.err.Error()
		return
	}
	cur := loadConfig(t.base).str("HOST", "")
	curHosts, _ := hostList(cur)
	chosen := map[string]bool{} // display Host() -> ticked
	for _, d := range res.list {
		for _, h := range curHosts {
			if strings.EqualFold(d.Host(), h) {
				chosen[d.Host()] = true
			}
		}
	}
	msg := ""
	sel := 0
	count := func() (n int) {
		for _, v := range chosen {
			if v {
				n++
			}
		}
		return
	}
	save := func(hosts []string) {
		list, _ := hostList(strings.Join(hosts, ","))
		setIniValue(filepath.Join(t.base, wifiIni), "HOST", strings.Join(list, ","))
		t.notice = "Display(s): " + strings.Join(list, ", ") + ". Restart the daemon to use them."
	}
	for {
		w, left := t.begin()
		var items []string
		for _, d := range res.list {
			mark := dim + "[ ]" + reset
			if chosen[d.Host()] {
				mark = fgGr + "[x]" + reset
			}
			ver := d.Ver
			if ver == "" {
				ver = "?"
			}
			items = append(items, fmt.Sprintf("%s %-15s %-15s fw %s", mark, d.Name, d.IP, ver))
		}
		items = append(items, fmt.Sprintf("Save selection (%d of %d)", count(), maxDisplays), "Type an address by hand...")
		if len(res.list) == 0 {
			t.at(4, left+1, fgYl+"No display answered. Is it powered and on this WiFi?"+reset)
			t.menuBox(6, left, w, "Displays", items, sel)
		} else {
			t.menuBox(4, left, w, "Displays found", items, sel)
		}
		if msg != "" {
			t.at(4+len(items)+3, left+1, fgYl+msg+reset)
		}
		t.end("Up/Down move   Enter/A/Space tick   Esc/B back")
		ev, ok := t.wait(time.Hour)
		if !ok {
			return
		}
		msg = ""
		toggle := func() {
			h := res.list[sel].Host()
			if !chosen[h] && count() >= maxDisplays {
				msg = fmt.Sprintf("At most %d displays.", maxDisplays)
				return
			}
			chosen[h] = !chosen[h]
		}
		switch {
		case ev.k == kUp:
			sel = (sel + len(items) - 1) % len(items)
		case ev.k == kDown:
			sel = (sel + 1) % len(items)
		case ev.k == kEsc:
			return
		case ev.k == kRune && ev.r == ' ' && sel < len(res.list):
			toggle()
		case ev.k == kEnter && sel < len(res.list):
			toggle()
		case ev.k == kEnter && sel == len(res.list): // save
			var hosts []string
			for _, d := range res.list {
				if chosen[d.Host()] {
					hosts = append(hosts, d.Host())
				}
			}
			if len(hosts) == 0 {
				msg = "Tick at least one display."
				continue
			}
			save(hosts)
			return
		case ev.k == kEnter:
			if v, ok := t.input("Display address", "Name or IP; commas for several; or auto", cur); ok && v != "" {
				save([]string{v})
			}
			return
		}
	}
}

// input is a one-line text editor.
func (t *tui) input(title, prompt, def string) (string, bool) {
	val := []rune(def)
	for {
		w, left := t.begin()
		t.frame(4, left, w, 6, title)
		t.at(5, left+2, prompt)
		field := string(val)
		t.at(7, left+2, bgSel+" "+field+t.rep(" ", w-8-len([]rune(field)))+" "+reset)
		t.at(7, left+3+len([]rune(field)), bgSel+fgWh+"_"+reset)
		t.end("type the text   Enter save   Esc cancel")
		ev, ok := t.wait(time.Hour)
		if !ok {
			return "", false
		}
		switch ev.k {
		case kEsc:
			return "", false
		case kEnter:
			return strings.TrimSpace(string(val)), true
		case kBackspace:
			if len(val) > 0 {
				val = val[:len(val)-1]
			}
		case kRune:
			val = append(val, ev.r)
		}
	}
}

func (t *tui) options() {
	ini := filepath.Join(t.base, wifiIni)
	trans := []string{"-1", "0", "1", "2", "3", "4", "5"}
	transName := map[string]string{"-1": "random", "0": "none", "1": "wipe left to right", "2": "wipe right to left", "3": "wipe top to bottom", "4": "iris", "5": "fade"}
	sel := 0
	scope := 0 // 0 = all displays, 1.. = one display of HOST
	for {
		all := loadConfig(t.base)
		hosts, _ := hostList(all.str("HOST", ""))
		if scope > len(hosts) {
			scope = 0
		}
		c, prefix, scopeName := all, "", "all displays"
		if scope > 0 {
			c, prefix, scopeName = all.forHost(hosts[scope-1]), hostKey(hosts[scope-1])+"_", hostKey(hosts[scope-1])
		}
		yn := func(k string, def bool) string {
			if c.yes(k, def) {
				return fgGr + "[x] yes" + reset
			}
			return dim + "[ ] no" + reset
		}
		items := []string{}
		first := 0
		if len(hosts) > 1 {
			items = append(items, fmt.Sprintf("%-34s %s", "Settings for", bold+scopeName+reset))
			first = 1
		}
		items = append(items,
			fmt.Sprintf("%-34s %s", "Send contrast and rotation", yn("WIFI_SEND_SETTINGS", false)),
			fmt.Sprintf("%-34s %s", "Contrast (0-255)", bold+strconv.Itoa(c.num("CONTRAST", 100))+reset),
			fmt.Sprintf("%-34s %s", "Rotate 180 degrees", yn("ROTATE", false)),
			fmt.Sprintf("%-34s %s", "Transition", bold+transName[c.str("TRANSITION", "-1")]+reset),
			fmt.Sprintf("%-34s %s", "Random _alt pictures (all)", yn("USE_RANDOM_ALT", true)),
			"Back",
		)
		w, left := t.begin()
		t.menuBox(3, left, w, "Options", items, sel)
		note := "Saved in t2o-wifi.ini. Restart the daemon to apply."
		if first == 1 {
			note = "Left/Right on the first line picks a display. " + note
		}
		t.at(3+len(items)+3, left+1, dim+truncate(note, w-2)+reset)
		t.end("Up/Down move   Left/Right/Enter change   Esc/B back")
		ev, ok := t.wait(time.Hour)
		if !ok {
			return
		}
		// keys are written with the display's prefix when one display is selected
		pk := func(k string) string {
			if k == "USE_RANDOM_ALT" { // the art is shared by every display
				return k
			}
			return prefix + k
		}
		flip := func(k string, def bool) {
			v := "yes"
			if c.yes(k, def) {
				v = "no"
			}
			setIniValue(ini, pk(k), v)
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
			if first == 1 && sel == 0 {
				scope = (scope + step + len(hosts) + 1) % (len(hosts) + 1)
				continue
			}
			switch sel - first {
			case 0:
				flip("WIFI_SEND_SETTINGS", false)
			case 1:
				if ev.k == kEnter {
					if v, ok := t.input("Contrast", "0-255 (used when 'send contrast' is on)", strconv.Itoa(c.num("CONTRAST", 100))); ok {
						if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 255 {
							setIniValue(ini, pk("CONTRAST"), strconv.Itoa(n))
						}
					}
				} else {
					n := c.num("CONTRAST", 100) + step*10
					if n < 0 {
						n = 0
					}
					if n > 255 {
						n = 255
					}
					setIniValue(ini, pk("CONTRAST"), strconv.Itoa(n))
				}
			case 2:
				flip("ROTATE", false)
			case 3:
				i := 0
				for j, v := range trans {
					if v == c.str("TRANSITION", "-1") {
						i = j
					}
				}
				setIniValue(ini, pk("TRANSITION"), trans[(i+step+len(trans))%len(trans)])
			case 4:
				flip("USE_RANDOM_ALT", true)
			case 5:
				if ev.k == kEnter {
					return
				}
			}
		}
	}
}

func (t *tui) sendTest() {
	core := "NES"
	if b, err := os.ReadFile("/tmp/CORENAME"); err == nil && strings.TrimSpace(string(b)) != "" {
		core = strings.TrimSpace(string(b))
	}
	v, ok := t.input("Send a test", "Core name to show on the display", core)
	if !ok || v == "" {
		return
	}
	cfg := loadConfig(t.base)
	hosts, _ := hostList(cfg.str("HOST", ""))
	if len(hosts) == 0 || strings.EqualFold(hosts[0], "auto") {
		t.notice = "Pick a display first."
		return
	}
	var failed []string
	for _, host := range hosts {
		w, left := t.begin()
		t.frame(4, left, w, 5, "Sending")
		t.at(6, left+3, "Sending "+v+" to "+host+"...")
		t.end("")
		dev, err := dialDevice(host)
		if err == nil {
			hc := cfg.forHost(host)
			d := &daemon{cfg: hc, finder: &ArtFinder{base: t.base, stock: stockDir, cfg: cfg}}
			err = d.sendCore(dev, v)
			dev.Close()
		}
		if err != nil {
			failed = append(failed, hostKey(host)+": "+err.Error())
		}
	}
	if len(failed) > 0 {
		t.notice = "Failed: " + strings.Join(failed, "; ")
	} else {
		t.notice = "Sent " + v + "."
	}
}

func (t *tui) viewLog() {
	b, _ := os.ReadFile(t.logPath)
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(b) == 0 {
		lines = []string{"No log yet (the daemon has not run)."}
	}
	top := 0
	for {
		w, left := t.begin()
		rows := t.h - 6
		if rows < 3 {
			rows = 3
		}
		if top > len(lines)-rows {
			top = len(lines) - rows
		}
		if top < 0 {
			top = 0
		}
		t.frame(3, left, w, rows+2, "Log")
		for i := 0; i < rows && top+i < len(lines); i++ {
			t.at(4+i, left+2, truncate(lines[top+i], w-4))
		}
		t.end("Up/Down scroll   Esc/B back")
		ev, ok := t.wait(time.Hour)
		if !ok {
			return
		}
		switch ev.k {
		case kUp:
			if top > 0 {
				top--
			}
		case kDown:
			top++
		case kEsc, kEnter:
			return
		}
	}
}

// toggleBoot adds or removes the daemon's start line in user-startup.sh. The stock
// tty2oled daemon's line is never touched: the two run side by side.
func (t *tui) toggleBoot() string {
	b, _ := os.ReadFile(t.startup)
	var lines []string
	if len(b) > 0 {
		lines = strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	} else {
		lines = []string{"#!/bin/bash"}
	}
	on := t.atBoot()
	var out []string
	for _, l := range lines {
		if !strings.Contains(l, "S60t2o-wifi") {
			out = append(out, l)
		}
	}
	msg := "The WiFi daemon no longer starts at boot."
	if !on {
		out = append(out, fmt.Sprintf("[[ -e %s ]] && %s start", t.initPath, t.initPath))
		msg = "The WiFi daemon will start at boot (the stock tty2oled daemon is untouched)."
	}
	if err := os.WriteFile(t.startup, []byte(strings.Join(out, "\n")+"\n"), 0o755); err != nil {
		return "Could not write " + t.startup + ": " + err.Error()
	}
	return msg
}

func runMenu(base string) {
	restore, err := rawMode()
	if err != nil {
		fmt.Fprintln(os.Stderr, "the menu needs a terminal:", err)
		os.Exit(1)
	}
	t := &tui{
		base:     base,
		startup:  envOr("TTY2OLED_STARTUP", "/media/fat/linux/user-startup.sh"),
		initPath: filepath.Join(base, "S60t2o-wifi"),
		logPath:  "/tmp/t2o-wifi.log",
		pidPath:  "/run/t2o-wifi.pid",
		version:  version,
		keys:     make(chan keyEvent, 16),
	}
	t.b, t.dot = ascBox, "*"
	if os.Getenv("TTY2OLED_ASCII") == "" && strings.Contains(strings.ToUpper(os.Getenv("LC_ALL")+os.Getenv("LANG")), "UTF") {
		t.b, t.dot = uniBox, "●"
	}
	go readKeys(t.keys)
	stop := make(chan struct{})
	go t.probe(stop)
	defer func() {
		close(stop)
		os.Stdout.WriteString("\x1b[?25h\x1b[0m\x1b[2J\x1b[H")
		restore()
	}()
	t.mainMenu()
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
