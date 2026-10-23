// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDisplay is a tiny WebSocket server that records what the device would receive.
type fakeDisplay struct {
	ln    net.Listener
	texts chan string
	bins  chan []byte
}

func newFakeDisplay(t *testing.T) *fakeDisplay {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeDisplay{ln: ln, texts: make(chan string, 64), bins: make(chan []byte, 64)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeDisplay) serve(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	key := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(k, "Sec-WebSocket-Key") {
			key = strings.TrimSpace(v)
		}
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	fmt.Fprintf(c, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
		base64.StdEncoding.EncodeToString(sum[:]))
	srv := &wsConn{c: c, br: br}
	pendingPic := 0
	for {
		op, data, err := srv.ReadMessage(0)
		if err != nil {
			return
		}
		if op == opBinary {
			f.bins <- data
			pendingPic -= len(data)
			if pendingPic <= 0 {
				srv.writeServer("ttyack;")
			}
			continue
		}
		s := string(data)
		f.texts <- s
		if strings.HasPrefix(s, "CMDCORC") {
			parts := strings.Split(s, ",")
			fmt.Sscan(parts[4], &pendingPic)
			continue
		}
		srv.writeServer("ttyack;")
	}
}

// writeServer sends an unmasked text frame (servers don't mask).
func (w *wsConn) writeServer(s string) {
	w.c.Write(append([]byte{0x81, byte(len(s))}, s...))
}

func testGSC() []byte {
	var b bytes.Buffer
	b.WriteString("// test\n// test\nconst unsigned char X_bits[] PROGMEM = {\n")
	for i := 0; i < 8192; i++ {
		fmt.Fprintf(&b, "0X%02X,", (i*7)&0xFF)
	}
	b.WriteString("};\n")
	return b.Bytes()
}

func TestParseLegacy(t *testing.T) {
	raw, err := parseLegacy(testGSC())
	if err != nil || len(raw) != 8192 {
		t.Fatalf("gsc: %v len %d", err, len(raw))
	}
	img := renderLegacy(raw)
	if img.RGBAAt(0, 0) != (color.RGBA{0, 0, 0, 255}) {
		t.Error("top of the canvas should be black")
	}
	jpg, err := encodeFit(img)
	if err != nil || len(jpg) > maxJPEG {
		t.Fatalf("encode: %v len %d", err, len(jpg))
	}
	if _, err := jpeg.Decode(bytes.NewReader(jpg)); err != nil {
		t.Fatal(err)
	}
	// 6 extra header bytes get stripped
	with := append([]byte("0X00,0X00,0X00,0X00,0X00,0X00,"), testGSC()...)
	if raw2, err := parseLegacy(with); err != nil || len(raw2) != 8192 {
		t.Fatalf("8198 variant: %v %d", err, len(raw2))
	}
	if _, err := parseLegacy([]byte("0X00,0X01")); err == nil {
		t.Error("a 2-byte file should be rejected")
	}
}

func TestFinderOrder(t *testing.T) {
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "art"), 0o755)
	os.MkdirAll(filepath.Join(base, "pics", "GSC"), 0o755)
	os.MkdirAll(filepath.Join(base, "pics", "XBM"), 0o755)
	var jb bytes.Buffer
	jpeg.Encode(&jb, image.NewRGBA(image.Rect(0, 0, 240, 240)), nil)
	os.WriteFile(filepath.Join(base, "art", "NES.jpg"), jb.Bytes(), 0o644)
	os.WriteFile(filepath.Join(base, "pics", "GSC", "NES.gsc"), testGSC(), 0o644)
	os.WriteFile(filepath.Join(base, "pics", "GSC", "jackal.gsc"), testGSC(), 0o644)
	f := &ArtFinder{base: base, cfg: Config{}}

	a, err := f.Find("nes") // case-insensitive, colour art wins over the legacy pack
	if err != nil || a == nil || !strings.HasSuffix(a.Path, "NES.jpg") {
		t.Fatalf("nes: %v %+v", err, a)
	}
	a, err = f.Find("jackalfixed") // name shortened until it matches
	if err != nil || a == nil || !strings.HasSuffix(a.Path, "jackal.gsc") {
		t.Fatalf("jackalfixed: %v %+v", err, a)
	}
	if a, _ := f.Find("zzz"); a != nil {
		t.Error("unknown core should have no art")
	}
}

func TestFinderPackOrder(t *testing.T) {
	base := t.TempDir()
	for _, d := range []string{"art", "art-pd", "pics_pri", filepath.Join("pics", "GSC")} {
		os.MkdirAll(filepath.Join(base, d), 0o755)
	}
	var jb bytes.Buffer
	jpeg.Encode(&jb, image.NewRGBA(image.Rect(0, 0, 240, 240)), nil)
	put := func(rel string, b []byte) { os.WriteFile(filepath.Join(base, rel), b, 0o644) }
	put("art/NES.jpg", jb.Bytes())     // the user's own art ...
	put("art-pd/NES.jpg", jb.Bytes())  // ... and the pack's, same name
	put("art-pd/SNES.jpg", jb.Bytes()) // pack only, and a stock picture exists too
	put("pics/GSC/SNES.gsc", testGSC())
	put("art-pd/marioworld.jpg", jb.Bytes()) // a long pack match ...
	put("art/mario.jpg", jb.Bytes())         // ... against a shorter user match
	put("art-pd/Genesis.jpg", jb.Bytes())    // pack vs the stock priority folder
	put("pics_pri/Genesis.gsc", testGSC())
	put("art-pd/VIC20.jpg", jb.Bytes())
	put("art-pd/VIC20_alt1.jpg", jb.Bytes())
	f := &ArtFinder{base: base, cfg: Config{}}
	want := func(core, dir string) {
		t.Helper()
		got := f.findFile(core)
		if got == "" || filepath.Base(filepath.Dir(got)) != dir {
			t.Errorf("%s: found %q, want it in %s/", core, got, dir)
		}
	}
	want("NES", "art")           // user art wins at the same name length
	want("SNES", "art-pd")       // the pack beats the stock grayscale
	want("marioworld", "art-pd") // a longer pack match beats a shorter user match
	want("Genesis", "pics_pri")  // the stock priority folder still beats the pack
	want("SNESfoo", "art-pd")    // name shortening reaches the pack too
	names := f.Names()
	has := func(n string) bool {
		for _, x := range names {
			if x == n {
				return true
			}
		}
		return false
	}
	if !has("NES") || !has("SNES") || !has("VIC20") || has("VIC20_alt1") {
		t.Errorf("Names() = %v (want NES/SNES/VIC20 once, no _alt)", names)
	}
	for i := 1; i < len(names); i++ {
		if strings.ToLower(names[i-1]) >= strings.ToLower(names[i]) {
			t.Errorf("Names() not sorted/unique at %q, %q", names[i-1], names[i])
		}
	}
}

func TestSendOverWebSocket(t *testing.T) {
	srv := newFakeDisplay(t)
	defer srv.ln.Close()
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "pics", "GSC"), 0o755)
	os.WriteFile(filepath.Join(base, "pics", "GSC", "C64.gsc"), testGSC(), 0o644)
	d := &daemon{cfg: Config{"TRANSITION": "0"}, finder: &ArtFinder{base: base, cfg: Config{}}}

	dev, err := dialDevice(srv.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	if err := d.sendCore(dev, "C64"); err != nil {
		t.Fatal(err)
	}
	hdr := <-srv.texts
	if !strings.HasPrefix(hdr, "CMDCORC,C64,0,600,") {
		t.Fatalf("header %q", hdr)
	}
	var got int
	for len(srv.bins) > 0 {
		got += len(<-srv.bins)
	}
	var want int
	fmt.Sscan(strings.Split(hdr, ",")[4], &want)
	if got != want || want == 0 {
		t.Fatalf("sent %d bytes, header says %d", got, want)
	}
	// no art -> bare name
	if err := d.sendCore(dev, "UNKNOWN"); err != nil {
		t.Fatal(err)
	}
	if s := <-srv.texts; s != "UNKNOWN" {
		t.Fatalf("got %q", s)
	}
	if !dev.Alive() {
		t.Error("ping should be answered")
	}
}

func TestIni(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.ini")
	os.WriteFile(p, []byte("# c\nCONTRAST=\"150\"   # note\nROTATE='yes'\nfoo() {\n  a=1\nHOST=10.0.0.5\n"), 0o644)
	c := Config{}
	c.loadIni(p)
	if c.num("CONTRAST", 0) != 150 || !c.yes("ROTATE", false) || c.str("HOST", "") != "10.0.0.5" {
		t.Fatalf("%v", c)
	}
	_ = time.Second
}

func TestMDNS(t *testing.T) {
	// a responder that answers with a compressed name, like the ESP32's
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	go func() {
		buf := make([]byte, 512)
		n, from, err := srv.ReadFromUDP(buf)
		if err != nil || n < 12 {
			return
		}
		q := buf[:n]
		resp := []byte{0, 0, 0x84, 0, 0, 0, 0, 1, 0, 0, 0, 0}
		resp = append(resp, q[12:n-4]...) // the name, uncompressed
		resp = append(resp, 0, 1, 0x80, 1, 0, 0, 0, 120, 0, 4, 192, 168, 7, 9)
		srv.WriteToUDP(resp, from)
	}()
	cl, err := net.ListenUDP("udp4", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ip, err := mdnsQuery("Tty2oled-AB12.local", srv.LocalAddr().(*net.UDPAddr), []*net.UDPConn{cl}, 2*time.Second)
	if err != nil || ip.String() != "192.168.7.9" {
		t.Fatalf("%v %v", ip, err)
	}
	// a compressed name (pointer back to the question) parses too
	msg := []byte{0, 0, 0x84, 0, 0, 1, 0, 1, 0, 0, 0, 0}
	msg = append(msg, buildQuery("x.local")[12:]...)
	msg = append(msg, 0xC0, 12, 0, 1, 0, 1, 0, 0, 0, 120, 0, 4, 10, 0, 0, 5)
	if ip, ok := parseAnswer(msg, "x.local"); !ok || ip.String() != "10.0.0.5" {
		t.Fatalf("compressed: %v %v", ip, ok)
	}
	if h, err := resolveHost("10.1.2.3:81"); err != nil || h != "10.1.2.3:81" {
		t.Fatal("plain addresses must pass through")
	}
}

func TestBrowseParse(t *testing.T) {
	// one response: PTR + SRV + TXT + A, with names compressed against the service name
	svc := "_tty2oled._tcp.local"
	name := func(s string) []byte {
		var b []byte
		for _, l := range strings.Split(s, ".") {
			b = append(b, byte(len(l)))
			b = append(b, l...)
		}
		return append(b, 0)
	}
	msg := []byte{0, 0, 0x84, 0, 0, 0, 0, 4, 0, 0, 0, 0}
	rr := func(n []byte, typ byte, rdata []byte) {
		msg = append(msg, n...)
		msg = append(msg, 0, typ, 0x80, 1, 0, 0, 0, 120, byte(len(rdata)>>8), byte(len(rdata)))
		msg = append(msg, rdata...)
	}
	inst := "tty2oled-AB12." + svc
	rr(name(svc), 12, name(inst))
	rr(name(inst), 33, append([]byte{0, 0, 0, 0, 0, 81}, name("tty2oled-AB12.local")...))
	rr(name(inst), 16, []byte{7, 'v', 'e', 'r', '=', '0', '.', '6'})
	rr(name("tty2oled-AB12.local"), 1, []byte{10, 1, 2, 3})

	var got []DisplayInfo
	srv, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer srv.Close()
	go func() {
		buf := make([]byte, 512)
		_, from, err := srv.ReadFromUDP(buf)
		if err == nil {
			srv.WriteToUDP(msg, from)
		}
	}()
	cl, _ := net.ListenUDP("udp4", nil)
	defer cl.Close()
	old := mdnsGroup
	mdnsGroup = srv.LocalAddr().(*net.UDPAddr)
	got = browse([]string{svc}, []*net.UDPConn{cl}, 900*time.Millisecond)
	mdnsGroup = old
	if len(got) != 1 || got[0].Name != "tty2oled-AB12" || got[0].IP.String() != "10.1.2.3" || got[0].Port != 81 || got[0].Ver != "0.6" {
		t.Fatalf("%+v", got)
	}
}

func TestHostList(t *testing.T) {
	h, extra := hostList("a.local, b.local;c.local  d.local a.local")
	if strings.Join(h, "|") != "a.local|b.local|c.local" || extra != 1 {
		t.Fatalf("got %v extra %d", h, extra)
	}
	if h, _ := hostList(""); len(h) != 0 {
		t.Fatalf("empty gave %v", h)
	}
}

func TestPerDisplaySettings(t *testing.T) {
	for host, want := range map[string]string{
		"tty2oled-CD20.local":    "CD20",
		"tty2oled-b610.local:81": "B610",
		"192.168.86.27":          "192_168_86_27",
		"192.168.86.27:81":       "192_168_86_27",
		"Tty2oled-CD20.LOCAL":    "CD20",
	} {
		if got := hostKey(host); got != want {
			t.Errorf("hostKey(%q) = %q, want %q", host, got, want)
		}
	}
	c := Config{"CONTRAST": "100", "ROTATE": "no", "CD20_CONTRAST": "40", "B610_ROTATE": "yes", "CD20_": "x"}
	cd := c.forHost("tty2oled-CD20.local")
	if cd.num("CONTRAST", 0) != 40 || cd.yes("ROTATE", true) {
		t.Errorf("CD20 settings wrong: %v", cd)
	}
	b := c.forHost("tty2oled-B610.local")
	if b.num("CONTRAST", 0) != 100 || !b.yes("ROTATE", false) {
		t.Errorf("B610 settings wrong: %v", b)
	}
}

// Two displays: one core change reaches both, and each gets its own contrast.
func TestTwoDisplays(t *testing.T) {
	a, b := newFakeDisplay(t), newFakeDisplay(t)
	defer a.ln.Close()
	defer b.ln.Close()
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "pics", "GSC"), 0o755)
	os.WriteFile(filepath.Join(base, "pics", "GSC", "C64.gsc"), testGSC(), 0o644)
	core := filepath.Join(base, "CORENAME")
	os.WriteFile(core, []byte("C64\n"), 0o644)
	fin := &ArtFinder{base: base, cfg: Config{}}
	cfg := Config{"TRANSITION": "0", "WIFI_SEND_SETTINGS": "yes"}
	events := make(chan string, 1)
	for i, addr := range []string{a.ln.Addr().String(), b.ln.Addr().String()} {
		// (both fakes are on 127.0.0.1, so the per-host prefix can't tell them apart here;
		// forHost is covered by TestPerDisplaySettings)
		hc := Config{}
		for k, v := range cfg {
			hc[k] = v
		}
		hc["CONTRAST"] = []string{"40", "200"}[i]
		d := &daemon{cfg: hc, finder: fin, host: addr, coreFile: core, sleepFile: filepath.Join(base, "sleep"),
			settings: true, delay: time.Duration(i) * 50 * time.Millisecond, multi: true}
		go d.run(events)
	}
	for name, f := range map[string]*fakeDisplay{"A": a, "B": b} {
		sawCorc, contrast := false, ""
		deadline := time.After(5 * time.Second)
		for !sawCorc {
			select {
			case s := <-f.texts:
				if strings.HasPrefix(s, "CMDCON,") {
					contrast = s
				}
				if strings.HasPrefix(s, "CMDCORC,C64,") {
					sawCorc = true
				}
			case <-deadline:
				t.Fatalf("display %s never got the picture", name)
			}
		}
		want := map[string]string{"A": "CMDCON,40", "B": "CMDCON,200"}[name]
		if contrast != want {
			t.Errorf("display %s got %q, want %q", name, contrast, want)
		}
	}
}
