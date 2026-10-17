// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

// A minimal RFC 6455 WebSocket client - just enough for the display's port-81 server
// (text and binary frames, ping/pong, close). Standard library only, so the daemon
// stays one static binary with no module downloads.

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	opText   = 0x1
	opBinary = 0x2
	opClose  = 0x8
	opPing   = 0x9
	opPong   = 0xA
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type wsConn struct {
	c     net.Conn
	br    *bufio.Reader
	pongs int
}

func wsDial(addr string, timeout time.Duration) (*wsConn, error) {
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	keyRaw := make([]byte, 16)
	rand.Read(keyRaw)
	key := base64.StdEncoding.EncodeToString(keyRaw)
	req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", addr, key)
	c.SetDeadline(time.Now().Add(timeout))
	if _, err := c.Write([]byte(req)); err != nil {
		c.Close()
		return nil, err
	}
	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	if err != nil {
		c.Close()
		return nil, err
	}
	if !strings.Contains(status, " 101 ") {
		c.Close()
		return nil, fmt.Errorf("websocket upgrade refused: %s", strings.TrimSpace(status))
	}
	want := sha1.Sum([]byte(key + wsGUID))
	wantAccept := base64.StdEncoding.EncodeToString(want[:])
	gotAccept := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			c.Close()
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(k, "Sec-WebSocket-Accept") {
			gotAccept = strings.TrimSpace(v)
		}
	}
	if gotAccept != "" && gotAccept != wantAccept { // some embedded servers omit it; a wrong one is an error
		c.Close()
		return nil, errors.New("websocket handshake: bad Sec-WebSocket-Accept")
	}
	c.SetDeadline(time.Time{})
	return &wsConn{c: c, br: br}, nil
}

func (w *wsConn) Close() error { return w.c.Close() }

func (w *wsConn) writeFrame(op byte, payload []byte) error {
	hdr := []byte{0x80 | op}
	n := len(payload)
	switch {
	case n < 126:
		hdr = append(hdr, 0x80|byte(n))
	case n < 65536:
		hdr = append(hdr, 0x80|126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 0x80|127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(hdr, b[:]...)
	}
	var mask [4]byte
	rand.Read(mask[:])
	hdr = append(hdr, mask[:]...)
	buf := make([]byte, len(hdr)+n)
	copy(buf, hdr)
	for i, b := range payload {
		buf[len(hdr)+i] = b ^ mask[i%4]
	}
	w.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := w.c.Write(buf)
	return err
}

func (w *wsConn) WriteText(s string) error   { return w.writeFrame(opText, []byte(s)) }
func (w *wsConn) WriteBinary(b []byte) error { return w.writeFrame(opBinary, b) }

// ReadMessage returns the next text/binary message, answering pings on the way.
func (w *wsConn) ReadMessage(timeout time.Duration) (op byte, data []byte, err error) {
	for {
		if timeout > 0 {
			w.c.SetReadDeadline(time.Now().Add(timeout))
		}
		var h [2]byte
		if _, err = io.ReadFull(w.br, h[:]); err != nil {
			return
		}
		fop := h[0] & 0x0F
		n := uint64(h[1] & 0x7F)
		switch n {
		case 126:
			var b [2]byte
			if _, err = io.ReadFull(w.br, b[:]); err != nil {
				return
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err = io.ReadFull(w.br, b[:]); err != nil {
				return
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if n > 1<<20 {
			return 0, nil, errors.New("websocket frame too large")
		}
		var mask [4]byte
		masked := h[1]&0x80 != 0
		if masked {
			if _, err = io.ReadFull(w.br, mask[:]); err != nil {
				return
			}
		}
		payload := make([]byte, n)
		if _, err = io.ReadFull(w.br, payload); err != nil {
			return
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch fop {
		case opPing:
			w.writeFrame(opPong, payload)
		case opPong:
			w.pongs++
		case opClose:
			w.writeFrame(opClose, nil)
			return fop, nil, io.EOF
		default:
			return fop, payload, nil
		}
	}
}
