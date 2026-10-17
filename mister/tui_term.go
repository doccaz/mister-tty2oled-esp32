// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

//go:build linux

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

// rawMode puts the terminal in raw mode (keeping Ctrl-C) and returns a restore function.
func rawMode() (func(), error) {
	fd := os.Stdin.Fd()
	var old syscall.Termios
	if err := ioctl(fd, syscall.TCGETS, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.ICRNL | syscall.IXON | syscall.INLCR
	raw.Lflag &^= syscall.ICANON | syscall.ECHO
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, syscall.TCSETS, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return func() { ioctl(fd, syscall.TCSETS, unsafe.Pointer(&old)) }, nil
}

func termSize() (w, h int) {
	var ws struct{ Row, Col, X, Y uint16 }
	if err := ioctl(os.Stdout.Fd(), syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil || ws.Col == 0 {
		return 80, 25
	}
	return int(ws.Col), int(ws.Row)
}
