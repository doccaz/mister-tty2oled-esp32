// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

// t2o-wifi: a companion to the MiSTer's tty2oled.sh that talks to the
// GC9A01 display over WiFi (its WebSocket port) instead of a serial cable. It watches
// /tmp/CORENAME like the stock daemon and sends the core's picture.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var version = "dev"

type daemon struct {
	cfg       Config
	finder    *ArtFinder
	host      string
	coreFile  string
	sleepFile string
	settings  bool
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
	art, err := d.finder.Find(core)
	if err != nil {
		log.Printf("art for %q: %v (sending the name only)", core, err)
	}
	if art == nil {
		log.Printf("core %q: no picture, sending the name", core)
		return dev.Cmd(core)
	}
	log.Printf("core %q: %s (%d bytes)", core, filepath.Base(art.Path), len(art.JPEG))
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
			time.Sleep(150 * time.Millisecond) // let a burst of writes settle
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
	host := flag.String("host", "", "display address, host or host:port (default: HOST in t2o-wifi.ini)")
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
	d := &daemon{
		cfg:       cfg,
		finder:    &ArtFinder{base: *base, stock: stockDir, cfg: cfg},
		host:      *host,
		coreFile:  *coreFile,
		sleepFile: "/tmp/tty2oled_sleep",
		settings:  cfg.yes("WIFI_SEND_SETTINGS", false),
	}

	if *once != "" {
		dev, err := dialDevice(d.host)
		if err != nil {
			log.Fatal(err)
		}
		defer dev.Close()
		if err := d.sendCore(dev, *once); err != nil {
			log.Fatal(err)
		}
		return
	}

	events, err := watchDir(filepath.Dir(d.coreFile), filepath.Base(d.coreFile), filepath.Base(d.sleepFile))
	if err != nil {
		log.Fatalf("can't watch %s: %v", d.coreFile, err)
	}
	log.Printf("t2o-wifi %s: watching %s, display at %s", version, d.coreFile, d.host)
	backoff := 2 * time.Second
	for {
		dev, err := dialDevice(d.host)
		if err == nil {
			log.Printf("connected to %s", d.host)
			backoff = 2 * time.Second
			err = d.session(dev, events)
			dev.Close()
		}
		log.Printf("link down: %v; retrying in %s", err, backoff)
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
	fmt.Print("Use which display? ")
	var n int
	if _, err := fmt.Scan(&n); err != nil || n < 1 || n > len(list) {
		log.Fatal("no such display")
	}
	path := filepath.Join(base, wifiIni)
	if err := setIniValue(path, "HOST", list[n-1].Host()); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("HOST=%q saved in %s\n", list[n-1].Host(), path)
}
