// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatch(t *testing.T) {
	dir := t.TempDir()
	ch, err := watchDir(dir, "CORENAME")
	if err != nil {
		t.Fatal(err)
	}
	wait := func(what string) {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("no event for " + what)
		}
	}
	os.WriteFile(filepath.Join(dir, "CORENAME"), []byte("NES"), 0o644)
	wait("write")
	os.WriteFile(filepath.Join(dir, "other"), []byte("x"), 0o644)
	os.Rename(filepath.Join(dir, "other"), filepath.Join(dir, "CORENAME"))
	wait("replace")
}
