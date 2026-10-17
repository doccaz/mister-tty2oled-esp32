// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

//go:build linux

package main

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

// watchDir reports (on the returned channel) every write, create, move-in or delete of
// the given file names inside dir. /tmp/CORENAME is sometimes rewritten in place and
// sometimes replaced, so the directory is watched rather than the file.
func watchDir(dir string, names ...string) (<-chan string, error) {
	fd, err := syscall.InotifyInit()
	if err != nil {
		return nil, err
	}
	mask := uint32(syscall.IN_MODIFY | syscall.IN_CLOSE_WRITE | syscall.IN_CREATE | syscall.IN_MOVED_TO | syscall.IN_DELETE)
	if _, err := syscall.InotifyAddWatch(fd, filepath.Clean(dir), mask); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	out := make(chan string, 8)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := syscall.Read(fd, buf)
			if err != nil || n < syscall.SizeofInotifyEvent {
				continue
			}
			for off := 0; off+syscall.SizeofInotifyEvent <= n; {
				ev := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[off]))
				nameBytes := buf[off+syscall.SizeofInotifyEvent : off+syscall.SizeofInotifyEvent+int(ev.Len)]
				name := string(nameBytes)
				for i, c := range nameBytes {
					if c == 0 {
						name = string(nameBytes[:i])
						break
					}
				}
				if want[name] {
					select {
					case out <- name:
					default:
					}
				}
				off += syscall.SizeofInotifyEvent + int(ev.Len)
			}
		}
	}()
	return out, nil
}
