// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// t2o-wifi: a companion to the MiSTer's tty2oled.sh that talks to the
// GC9A01 display over WiFi (its WebSocket port) instead of a serial cable. It watches
// /tmp/CORENAME like the stock daemon and sends the core's picture.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var version = "dev"

// daemon drives one display. With several displays there is one per display: they share the
// art finder, the core file and the sleep file, and each has its own settings (cfg).
type daemon struct {
	cfg       Config
	finder    *ArtFinder
	host      string
	coreFile  string
	sleepFile string
	settings  bool
	delay     time.Duration // stagger between displays, so uploads don't all start at once
	multi     bool          // more than one display: prefix log lines with the display name
}

func (d *daemon) logf(format string, a ...any) {
	if d.multi {
		format = "[" + hostKey(d.host) + "] " + format
	}
	log.Printf(format, a...)
}

func (d *daemon) readCore() string {
	for i := 0; i < 3; i++ {
		b, err := os.ReadFile(d.coreFile)
		if s := strings.TrimSpace(string(b)); err == nil && s != "" {
			return s
		}
		time.Sleep(100 * time.Millisecond) // truncate-then-write: the file is briefly empty
	}
	return ""
}

func (d *daemon) asleep() bool {
	_, err := os.Stat(d.sleepFile)
	return err == nil
}

// sendCore shows the picture for core, or just its name when there is none.
func (d *daemon) sendCore(dev *Device, core string) error {
	art, err := d.finder.FindShared(core)
	if err != nil {
		d.logf("art for %q: %v (sending the name only)", core, err)
	}
	if art == nil {
		d.logf("core %q: no picture, sending the name", core)
		return dev.Cmd(core)
	}
	d.logf("core %q: %s (%d bytes)", core, filepath.Base(art.Path), len(art.JPEG))
	return dev.SendJPEG(core, effect(d.cfg.num("TRANSITION", -1)), art.JPEG)
}

// startup mirrors the stock daemon's first transmissions. Contrast, rotation and the
// screensaver are only sent when asked for (WIFI_SEND_SETTINGS="yes"): the display keeps
// its own settings page, and these commands would override them until its next reboot.
func (d *daemon) startup(dev *Device) error {
	if err := dev.Cmd("QWERTZ"); err != nil {
		return err
	}
	if d.settings {
		if err := dev.Cmd(fmt.Sprintf("CMDCON,%d", d.cfg.num("CONTRAST", 100))); err != nil {
			return err
		}
		if d.cfg.yes("ROTATE", false) {
			if err := dev.Cmd("CMDROT,1"); err != nil {
				return err
			}
		}
		if err := dev.Cmd(saverCommand(d.cfg)); err != nil {
			return err
		}
	}
	// The MiSTer's local time as epoch seconds shifted by its UTC offset, as the stock script does.
	_, off := time.Now().Zone()
	return dev.Cmd(fmt.Sprintf("CMDSETTIME,%d", time.Now().Unix()+int64(off)))
}

func saverCommand(c Config) string {
	if !c.yes("SCREENSAVER", false) {
		return "CMDSAVER,0,0,0"
	}
	mode := 0
	for i, k := range []string{"TTY2OLED", "MISTER", "CORE", "TIME", "DATE", "STARS", "TOAST"} {
		if c.yes("SCREENSAVER_SCREEN_"+k, false) {
			mode += 1 << uint(i)
		}
	}
	return fmt.Sprintf("CMDSAVER,%d,%d,%d", mode, c.num("SCREENSAVER_IVAL", 10), c.num("SCREENSAVER_START", 120))
}

// session runs until the link breaks. It always re-sends the current core on entry, so
// a display that rebooted or dropped off WiFi comes back showing the right picture.
func (d *daemon) session(dev *Device, events <-chan string) error {
	time.Sleep(d.delay)
	if err := d.startup(dev); err != nil {
		return err
	}
	pending := true
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		if pending && !d.asleep() {
			if core := d.readCore(); core != "" {
				if err := d.sendCore(dev, core); err != nil {
					return err
				}
			}
			pending = false
		}
		select {
		case <-events:
			time.Sleep(150*time.Millisecond + d.delay) // let a burst of writes settle
		drain:
			for {
				select {
				case <-events:
				default:
					break drain
				}
			}
			pending = true
		case <-tick.C:
			if !dev.Alive() {
				return fmt.Errorf("the display stopped answering")
			}
		}
	}
}

func main() {
	base := flag.String("base", "/media/fat/t2o-wifi", "this daemon's folder (t2o-wifi.ini, art/)")
	stock := flag.String("stock", stockDir, "the stock tty2oled folder (its ini files and pics/), read only")
	host := flag.String("host", "", "display address, host or host:port; up to 3 separated by commas (default: HOST in t2o-wifi.ini)")
	coreFile := flag.String("corefile", "/tmp/CORENAME", "file holding the current core name")
	once := flag.String("once", "", "send this core name once and exit (testing)")
	menu := flag.Bool("menu", false, "open the configuration menu")
	list := flag.Bool("list", false, "list the displays found on the network and exit")
	plain := flag.Bool("plain", false, "with -list: one tab-separated line per display (name, ip, port, version), for scripts")
	selectDev := flag.Bool("select", false, "list the displays, pick one and save it as HOST in t2o-wifi.ini")
	showVersion := flag.Bool("version", false, "print the version")
	flag.Parse()
	stockDir = *stock
	if *showVersion {
		fmt.Println("t2o-wifi", version)
		return
	}

	if *menu {
		runMenu(*base)
		return
	}
	cfg := loadConfig(*base)
	if *list || *selectDev {
		listDisplays(*base, *selectDev, *plain)
		return
	}
	if *host == "" {
		*host = cfg.str("HOST", "")
	}
	if *host == "" {
		log.Fatalf("no display address: set HOST=\"192.168.x.y\" in %s or pass -host", filepath.Join(*base, wifiIni))
	}
	if strings.EqualFold(*host, "auto") {
		*host = autoHost()
	}
	hosts, extra := hostList(*host)
	if extra > 0 {
		log.Printf("HOST lists more than %d displays: ignoring the last %d", maxDisplays, extra)
	}
	finder := &ArtFinder{base: *base, stock: stockDir, cfg: cfg}
	var ds []*daemon
	for i, h := range hosts {
		hc := cfg.forHost(h)
		ds = append(ds, &daemon{
			cfg:       hc,
			finder:    finder,
			host:      h,
			coreFile:  *coreFile,
			sleepFile: "/tmp/tty2oled_sleep",
			settings:  hc.yes("WIFI_SEND_SETTINGS", false),
			delay:     time.Duration(i) * 400 * time.Millisecond,
			multi:     len(hosts) > 1,
		})
	}

	if *once != "" {
		for _, d := range ds {
			dev, err := dialDevice(d.host)
			if err != nil {
				log.Fatal(err)
			}
			err = d.sendCore(dev, *once)
			dev.Close()
			if err != nil {
				log.Fatal(err)
			}
		}
		return
	}

	events, err := watchDir(filepath.Dir(ds[0].coreFile), filepath.Base(ds[0].coreFile), filepath.Base(ds[0].sleepFile))
	if err != nil {
		log.Fatalf("can't watch %s: %v", ds[0].coreFile, err)
	}
	log.Printf("t2o-wifi %s: watching %s, display(s) at %s", version, ds[0].coreFile, strings.Join(hosts, ", "))
	// one inotify watcher, one event channel per display
	chans := make([]chan string, len(ds))
	for i := range ds {
		chans[i] = make(chan string, 1)
	}
	go func() {
		for ev := range events {
			for _, c := range chans {
				select {
				case c <- ev:
				default: // an event is already waiting for that display
				}
			}
		}
	}()
	for i := 1; i < len(ds); i++ {
		go ds[i].run(chans[i])
	}
	ds[0].run(chans[0])
}

// run keeps one display connected for ever, re-trying with a growing pause.
func (d *daemon) run(events <-chan string) {
	backoff := 2 * time.Second
	for {
		dev, err := dialDevice(d.host)
		if err == nil {
			d.logf("connected to %s", d.host)
			backoff = 2 * time.Second
			err = d.session(dev, events)
			dev.Close()
		}
		d.logf("link down: %v; retrying in %s", err, backoff)
		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// autoHost picks the display for HOST="auto": the only one on the network. With several it
// refuses to guess (-select saves a choice).
func autoHost() string {
	for {
		list, err := findDisplays()
		switch {
		case err != nil:
			log.Printf("looking for displays: %v", err)
		case len(list) == 1:
			log.Printf("found %s (%s)", list[0].Name, list[0].IP)
			return list[0].Host()
		case len(list) > 1:
			log.Fatalf("HOST=\"auto\" but %d displays answered; run `t2o-wifi -select` and pick one", len(list))
		default:
			log.Printf("no display found yet, retrying")
		}
		time.Sleep(10 * time.Second)
	}
}

func listDisplays(base string, choose, plain bool) {
	list, err := findDisplays()
	if err != nil {
		log.Fatal(err)
	}
	if plain {
		for _, d := range list {
			fmt.Printf("%s\t%s\t%d\t%s\n", d.Name, d.IP, d.Port, d.Ver)
		}
		return
	}
	if len(list) == 0 {
		log.Fatal("no displays found")
	}
	for i, d := range list {
		fmt.Printf("%d) %-16s %-15s port %d  fw %s\n", i+1, d.Name, d.IP, d.Port, d.Ver)
	}
	if !choose {
		return
	}
	fmt.Printf("Use which display (up to %d, e.g. \"1 2\")? ", maxDisplays)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	var picked []string
	for _, f := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(list) {
			log.Fatal("no such display: " + f)
		}
		picked = append(picked, list[n-1].Host())
	}
	if len(picked) == 0 {
		log.Fatal("no display chosen")
	}
	picked, _ = hostList(strings.Join(picked, ","))
	path := filepath.Join(base, wifiIni)
	if err := setIniValue(path, "HOST", strings.Join(picked, ",")); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("HOST=%q saved in %s\n", strings.Join(picked, ","), path)
}
