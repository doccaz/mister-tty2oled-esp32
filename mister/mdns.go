// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

// A minimal mDNS resolver (RFC 6762): looks up the IPv4 address of a ".local" name, which
// the MiSTer's own libc cannot do. It sends a one-shot multicast query for an A record
// from port 5353 on each interface and reads the (multicast) answer.

import (
	"encoding/binary"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

func buildQuery(name string) []byte { return buildQueryType(name, 1) }

func buildQueryType(name string, qtype uint16) []byte {
	q := []byte{0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0} // id 0, standard query, 1 question
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	return append(q, 0, byte(qtype>>8), byte(qtype), 0, 1) // root, type, class IN
}

// readName decodes a (possibly compressed) domain name at off, returning the name and the
// offset just past it in the original position.
func readName(msg []byte, off int) (string, int, error) {
	var parts []string
	next := -1
	for hops := 0; hops < 20; hops++ {
		if off >= len(msg) {
			return "", 0, errors.New("short name")
		}
		l := int(msg[off])
		switch {
		case l == 0:
			if next < 0 {
				next = off + 1
			}
			return strings.Join(parts, "."), next, nil
		case l&0xC0 == 0xC0:
			if off+1 >= len(msg) {
				return "", 0, errors.New("short pointer")
			}
			if next < 0 {
				next = off + 2
			}
			off = int(binary.BigEndian.Uint16(msg[off:])) & 0x3FFF
		default:
			if off+1+l > len(msg) {
				return "", 0, errors.New("short label")
			}
			parts = append(parts, string(msg[off+1:off+1+l]))
			off += 1 + l
		}
	}
	return "", 0, errors.New("name loop")
}

// parseAnswer returns the IPv4 address of name from a DNS response, if it has one.
func parseAnswer(msg []byte, name string) (net.IP, bool) {
	if len(msg) < 12 || msg[2]&0x80 == 0 { // not a response
		return nil, false
	}
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	rr := int(binary.BigEndian.Uint16(msg[6:])) + int(binary.BigEndian.Uint16(msg[8:])) + int(binary.BigEndian.Uint16(msg[10:]))
	off := 12
	for i := 0; i < qd; i++ {
		_, n, err := readName(msg, off)
		if err != nil || n+4 > len(msg) {
			return nil, false
		}
		off = n + 4
	}
	want := strings.ToLower(strings.TrimSuffix(name, "."))
	for i := 0; i < rr; i++ {
		got, n, err := readName(msg, off)
		if err != nil || n+10 > len(msg) {
			return nil, false
		}
		typ := binary.BigEndian.Uint16(msg[n:])
		rdlen := int(binary.BigEndian.Uint16(msg[n+8:]))
		off = n + 10
		if off+rdlen > len(msg) {
			return nil, false
		}
		if typ == 1 && rdlen == 4 && strings.ToLower(got) == want {
			return net.IPv4(msg[off], msg[off+1], msg[off+2], msg[off+3]), true
		}
		off += rdlen
	}
	return nil, false
}

// mdnsQuery sends the query to dest from every conn and returns the first answer. The
// display's responder only answers queries that come from port 5353, so the conns are
// bound to it (see resolveMDNS).
func mdnsQuery(name string, dest *net.UDPAddr, conns []*net.UDPConn, timeout time.Duration) (net.IP, error) {
	found := make(chan net.IP, len(conns))
	deadline := time.Now().Add(timeout)
	query := buildQuery(name)
	var wg sync.WaitGroup
	defer stopReaders(conns, &wg)
	for _, c := range conns {
		c.SetReadDeadline(deadline)
		c.WriteToUDP(query, dest)
		wg.Add(1)
		go func(c *net.UDPConn) {
			defer wg.Done()
			buf := make([]byte, 1500)
			for {
				n, _, err := c.ReadFromUDP(buf)
				if err != nil {
					return
				}
				if ip, ok := parseAnswer(buf[:n], name); ok {
					found <- ip
					return
				}
			}
		}(c)
	}
	select {
	case ip := <-found:
		return ip, nil
	case <-time.After(timeout):
		return nil, errors.New("no mDNS answer for " + name)
	}
}

// openMDNSConns binds port 5353 on every up, multicast-capable interface.
func openMDNSConns() []*net.UDPConn {
	var conns []*net.UDPConn
	ifaces, _ := net.Interfaces()
	for i := range ifaces {
		ifi := &ifaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if c, err := net.ListenMulticastUDP("udp4", ifi, mdnsGroup); err == nil {
			conns = append(conns, c)
		}
	}
	return conns
}

// stopReaders ends the reader goroutines (deadline in the past) and waits for them.
func stopReaders(conns []*net.UDPConn, wg *sync.WaitGroup) {
	for _, c := range conns {
		c.SetReadDeadline(time.Now())
	}
	wg.Wait()
}

func closeConns(conns []*net.UDPConn) {
	for _, c := range conns {
		c.Close()
	}
}

// resolveMDNS looks up name (e.g. "tty2oled-1a2b.local") with a few retries, asking on
// every up, multicast-capable interface.
func resolveMDNS(name string) (net.IP, error) {
	conns := openMDNSConns()
	if len(conns) == 0 {
		return nil, errors.New("no interface can do mDNS (port 5353 busy or no multicast)")
	}
	defer closeConns(conns)
	var err error
	for i := 0; i < 3; i++ {
		var ip net.IP
		if ip, err = mdnsQuery(name, mdnsGroup, conns, 1500*time.Millisecond); err == nil {
			return ip, nil
		}
	}
	return nil, err
}

// resolveHost turns "name.local[:port]" into "ip[:port]"; anything else passes through.
func resolveHost(hostport string) (string, error) {
	host, port := hostport, ""
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		host, port = h, p
	}
	if !strings.HasSuffix(strings.ToLower(host), ".local") {
		return hostport, nil
	}
	ip, err := resolveMDNS(host)
	if err != nil {
		return "", err
	}
	if port != "" {
		return net.JoinHostPort(ip.String(), port), nil
	}
	return ip.String(), nil
}

// ---- Browsing for services -------------------------------------------------------------

// DisplayInfo is one display found on the network.
type DisplayInfo struct {
	Name string // instance name, also its hostname: tty2oled-B610
	IP   net.IP
	Port int
	Ver  string
}

func (d DisplayInfo) Host() string { return d.Name + ".local" }

type record struct {
	name string
	typ  uint16
	data []byte // raw rdata
	off  int    // rdata offset in msg (names inside rdata may be compressed)
}

func parseRecords(msg []byte) []record {
	if len(msg) < 12 || msg[2]&0x80 == 0 {
		return nil
	}
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	n := int(binary.BigEndian.Uint16(msg[6:])) + int(binary.BigEndian.Uint16(msg[8:])) + int(binary.BigEndian.Uint16(msg[10:]))
	off := 12
	for i := 0; i < qd; i++ {
		_, o, err := readName(msg, off)
		if err != nil || o+4 > len(msg) {
			return nil
		}
		off = o + 4
	}
	var out []record
	for i := 0; i < n; i++ {
		name, o, err := readName(msg, off)
		if err != nil || o+10 > len(msg) {
			break
		}
		typ := binary.BigEndian.Uint16(msg[o:])
		rdlen := int(binary.BigEndian.Uint16(msg[o+8:]))
		if o+10+rdlen > len(msg) {
			break
		}
		out = append(out, record{name: name, typ: typ, data: msg[o+10 : o+10+rdlen], off: o + 10})
		off = o + 10 + rdlen
	}
	return out
}

// browse collects everything the answers to a PTR query for service (such as
// "_tty2oled._tcp.local") say, for the given time, from all conns.
func browse(services []string, conns []*net.UDPConn, window time.Duration) []DisplayInfo {
	var mu sync.Mutex
	var msgs [][]byte
	deadline := time.Now().Add(window)
	var wg sync.WaitGroup
	for _, c := range conns {
		c.SetReadDeadline(deadline)
		wg.Add(1)
		go func(c *net.UDPConn) {
			defer wg.Done()
			buf := make([]byte, 2000)
			for {
				n, _, err := c.ReadFromUDP(buf)
				if err != nil {
					return
				}
				mu.Lock()
				msgs = append(msgs, append([]byte(nil), buf[:n]...))
				mu.Unlock()
			}
		}(c)
	}
	// Multicast over WiFi drops packets, so ask again a few times inside the window.
	ask := func() {
		for _, c := range conns {
			for _, svc := range services {
				c.WriteToUDP(buildQueryType(svc, 12), mdnsGroup)
			}
		}
	}
	for t := time.Duration(0); t < window; t += 800 * time.Millisecond {
		ask()
		time.Sleep(800 * time.Millisecond)
	}
	stopReaders(conns, &wg) // a reader left running would steal the next query's answers
	mu.Lock()
	defer mu.Unlock()
	// the PTR targets are the instances; SRV/TXT/A records fill in the details
	byInstance := map[string]*DisplayInfo{}
	var order []string
	srvTarget := map[string]string{}
	ips := map[string]net.IP{}
	for _, m := range msgs {
		for _, r := range parseRecords(m) {
			lname := strings.ToLower(r.name)
			switch r.typ {
			case 12: // PTR
				for i, svc := range services {
					if lname != strings.ToLower(svc) {
						continue
					}
					inst, _, err := readName(m, r.off)
					suffix := "." + strings.ToLower(svc)
					if err != nil || !strings.HasSuffix(strings.ToLower(inst), suffix) {
						continue
					}
					short := inst[:len(inst)-len(suffix)]
					// the later services are generic (_ws._tcp): only accept our own names
					if i > 0 && !strings.HasPrefix(strings.ToLower(short), "tty2oled-") {
						continue
					}
					key := strings.ToLower(inst)
					if _, ok := byInstance[key]; !ok {
						byInstance[key] = &DisplayInfo{Name: short, Port: 81}
						order = append(order, key)
					}
				}
			case 33: // SRV
				if len(r.data) > 6 {
					if tgt, _, err := readName(m, r.off+6); err == nil {
						srvTarget[lname] = strings.ToLower(tgt)
						if d, ok := byInstance[lname]; ok {
							d.Port = int(binary.BigEndian.Uint16(r.data[4:]))
						}
					}
				}
			case 16: // TXT
				if d, ok := byInstance[lname]; ok {
					for i := 0; i < len(r.data); {
						l := int(r.data[i])
						if i+1+l > len(r.data) {
							break
						}
						if k, v, ok := strings.Cut(string(r.data[i+1:i+1+l]), "="); ok && k == "ver" {
							d.Ver = v
						}
						i += 1 + l
					}
				}
			case 1: // A
				if len(r.data) == 4 {
					ips[lname] = net.IP(append([]byte(nil), r.data...))
				}
			}
		}
	}
	var out []DisplayInfo
	seen := map[string]int{} // one device can show up under several services
	for _, key := range order {
		d := byInstance[key]
		if ip, ok := ips[srvTarget[key]]; ok {
			d.IP = ip
		} else if ip, ok := ips[strings.ToLower(d.Name)+".local"]; ok {
			d.IP = ip
		}
		if i, dup := seen[strings.ToLower(d.Name)]; dup {
			if out[i].Ver == "" {
				out[i].Ver = d.Ver
			}
			if out[i].IP == nil {
				out[i].IP = d.IP
			}
			continue
		}
		seen[strings.ToLower(d.Name)] = len(out)
		out = append(out, *d)
	}
	return out
}

// findDisplays lists the displays on the network. Firmware that predates the dedicated
// "_tty2oled._tcp" service is found through its "_ws._tcp" one (instances named tty2oled-*).
func findDisplays() ([]DisplayInfo, error) {
	conns := openMDNSConns()
	if len(conns) == 0 {
		return nil, errors.New("no interface can do mDNS (port 5353 busy or no multicast)")
	}
	defer closeConns(conns)
	// "_ws._tcp" finds firmware from before the dedicated service existed
	list := browse([]string{"_tty2oled._tcp.local", "_ws._tcp.local"}, conns, 3200*time.Millisecond)
	for i := range list {
		if list[i].IP == nil {
			if ip, err := mdnsQuery(list[i].Host(), mdnsGroup, conns, 1500*time.Millisecond); err == nil {
				list[i].IP = ip
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}
