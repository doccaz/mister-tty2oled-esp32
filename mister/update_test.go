// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func haveDownloader() bool {
	for _, c := range []string{"wget", "curl"} {
		if _, err := exec.LookPath(c); err == nil {
			return true
		}
	}
	return false
}

const sampleDB = `{
  "v": 1,
  "db_id": "doccaz_mister_tty2oled_gc9a01",
  "timestamp": 1700000000,
  "files": {
    "t2o-wifi/t2o-wifi": {
      "hash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "size": 3408056,
      "url": "https://github.com/doccaz/mister-tty2oled-gc9a01/releases/download/v0.5.4/t2o-wifi",
      "reboot": true
    },
    "t2o-wifi/t2o-wifi.ini": { "hash": "b", "size": 948, "url": "u", "overwrite": false }
  },
  "folders": { "t2o-wifi/": {}, "Scripts/": {} }
}`

func TestPickUpdate(t *testing.T) {
	// Different local hash -> an update is offered.
	entry, differs, err := pickUpdate([]byte(sampleDB), "ffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatalf("pickUpdate: %v", err)
	}
	if !differs {
		t.Fatal("expected differs=true for a non-matching local hash")
	}
	if entry.Size != 3408056 || entry.Hash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("wrong entry: %+v", entry)
	}
	if updateTag(entry.URL) != "v0.5.4" {
		t.Fatalf("updateTag = %q, want v0.5.4", updateTag(entry.URL))
	}

	// Matching hash -> no update.
	if _, differs, _ := pickUpdate([]byte(sampleDB), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); differs {
		t.Fatal("expected differs=false when the local hash matches")
	}

	// A database missing the binary entry is an error, not a silent no-op.
	if _, _, err := pickUpdate([]byte(`{"files":{}}`), "x"); err == nil {
		t.Fatal("expected an error for a database without the binary entry")
	}
	if _, _, err := pickUpdate([]byte(`not json`), "x"); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestFetchURL(t *testing.T) {
	if !haveDownloader() {
		t.Skip("neither wget nor curl is installed")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/db.json" {
			w.Write([]byte(sampleDB))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	body, err := fetchURL(srv.URL + "/db.json")
	if err != nil {
		t.Fatalf("fetchURL: %v", err)
	}
	if _, differs, err := pickUpdate(body, "ffffffffffffffffffffffffffffffff"); err != nil || !differs {
		t.Fatalf("round trip: differs=%v err=%v", differs, err)
	}

	// A 404 must surface as an error (not an empty body fed to the parser). curl, if
	// present, also fails on 404 with -f, so the fallback stays an error too.
	if _, err := fetchURL(srv.URL + "/missing"); err == nil {
		t.Fatal("expected an error for a 404")
	}
}

func TestMd5hex(t *testing.T) {
	if got := md5hex([]byte("")); got != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Fatalf("md5hex(empty) = %s", got)
	}
}
