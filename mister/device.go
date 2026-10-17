// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

const (
	chunkBytes = 2048 // the firmware reassembles pictures from 2 KB binary frames
	ackTimeout = 8 * time.Second
	picDurMs   = 600
)

// Device is one WebSocket session with the display (port 81).
type Device struct {
	ws    *wsConn
	pongs int
}

// lastGood remembers the address a ".local" name last resolved to, so a missed mDNS
// answer (UDP multicast is lossy on WiFi) doesn't stop a reconnect.
var lastGood = map[string]string{}

func dialDevice(host string) (*Device, error) {
	if !strings.Contains(host, ":") {
		host += ":81"
	}
	if addr, err := resolveHost(host); err == nil {
		if addr != host {
			lastGood[host] = addr
		}
		host = addr
	} else if cached, ok := lastGood[host]; ok {
		host = cached
	} else {
		return nil, err
	}
	ws, err := wsDial(host, 6*time.Second)
	if err != nil {
		return nil, err
	}
	return &Device{ws: ws}, nil
}

func (d *Device) Close() { d.ws.Close() }

// waitAck reads replies until "ttyack;" (ok) or "ERR ..." (refused).
func (d *Device) waitAck() error {
	deadline := time.Now().Add(ackTimeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return fmt.Errorf("no ack from the device")
		}
		op, data, err := d.ws.ReadMessage(left)
		if err != nil {
			return err
		}
		if op != opText {
			continue
		}
		s := strings.TrimSpace(string(data))
		switch {
		case strings.HasPrefix(s, "ERR"):
			return fmt.Errorf("device refused: %s", s)
		case strings.HasPrefix(s, "ttyack"):
			return nil
		}
	}
}

// Cmd sends a text command and waits for the ack.
func (d *Device) Cmd(line string) error {
	if err := d.ws.WriteText(line); err != nil {
		return err
	}
	return d.waitAck()
}

// SendJPEG transfers a picture: header line, then 2 KB binary chunks, then the ack
// (which the device sends once the picture is drawn).
func (d *Device) SendJPEG(name string, effect int, jpg []byte) error {
	name = strings.NewReplacer(",", " ", "\n", " ", "\r", " ").Replace(name)
	if err := d.ws.WriteText(fmt.Sprintf("CMDCORC,%s,%d,%d,%d", name, effect, picDurMs, len(jpg))); err != nil {
		return err
	}
	for off := 0; off < len(jpg); off += chunkBytes {
		end := off + chunkBytes
		if end > len(jpg) {
			end = len(jpg)
		}
		if err := d.ws.WriteBinary(jpg[off:end]); err != nil {
			return err
		}
	}
	return d.waitAck()
}

// Alive pings the socket; false means the link is dead and needs reconnecting.
func (d *Device) Alive() bool {
	before := d.ws.pongs
	if err := d.ws.writeFrame(opPing, nil); err != nil {
		return false
	}
	_, _, err := d.ws.ReadMessage(3 * time.Second)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return d.ws.pongs > before
	}
	return err == nil && d.ws.pongs > before
}

// effect maps the stock TRANSITION value (-1 auto, 0 none, 1..23 the OLED's set) onto
// the display's curated 0..5.
func effect(transition int) int {
	switch {
	case transition < 0:
		return 1 + rand.Intn(5)
	case transition == 0:
		return 0
	default:
		return (transition-1)%5 + 1
	}
}
