// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

// Self-update (`t2o-wifi -update`, and the menu's "Check for updates"). It reads the
// same MiSTer Downloader database that `update_all` uses - published at a stable URL on
// the project's GitHub Pages site - so the two can never disagree about which build is
// current. "Is there an update?" is simply "does the running binary's MD5 differ from the
// one the database lists?", which also works for dev builds with no clean version string.

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// The stable entry point. Its contents change each release (new MD5s, a new tag in
	// each url); the URL itself never does. tools/build_mister_db.py writes it, and
	// tools/build_site.sh publishes it here. This is the compiled DEFAULT; DB_URL in
	// t2o-wifi.ini overrides it (see dbURL), so a repo rename - which moves the Pages URL -
	// can be fixed with one ini line instead of needing a reflash to change the baked-in value.
	updateDBURL   = "https://doccaz.github.io/mister-tty2oled-gc9a01/t2o-wifi-db.json"
	updateFileKey = "t2o-wifi/t2o-wifi" // the daemon binary's entry in the database
	pidFile       = "/run/t2o-wifi.pid"
	// The MiSTer's CA bundle (the stock tty2oled scripts pass this exact path to wget).
	caBundle = "/etc/ssl/certs/cacert.pem"
)

type dbFile struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type updateDB struct {
	Files map[string]dbFile `json:"files"`
}

// fetchURL downloads a URL by shelling out to wget - the same tool the stock tty2oled
// scripts use on the MiSTer, so it is known to be present and to handle the device's
// certificate store (rather than linking Go's net/http + crypto/tls, which would nearly
// double this always-running binary for a feature used a few times a year, and whose TLS
// path can't be tested off-device anyway). It tries, in order: wget; wget with the MiSTer's
// CA bundle (for a wget build that needs it named); and curl as a last resort. Each gets a
// hard timeout, and a failing command's stderr is surfaced so a cert/clock error is legible
// on the menu's notice line instead of a bare "exit status 1".
func fetchURL(url string) ([]byte, error) {
	var cmds [][]string
	if _, err := exec.LookPath("wget"); err == nil {
		cmds = append(cmds, []string{"wget", "-q", "-O", "-", url})
		if _, err := os.Stat(caBundle); err == nil {
			cmds = append(cmds, []string{"wget", "-q", "-O", "-", "--ca-certificate=" + caBundle, url})
		}
	}
	if _, err := exec.LookPath("curl"); err == nil {
		cmds = append(cmds, []string{"curl", "-fsSL", url})
	}
	if len(cmds) == 0 {
		return nil, fmt.Errorf("cannot download %s: neither wget nor curl is installed", url)
	}
	var lastErr error
	for _, argv := range cmds {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
		cancel()
		if err == nil && len(out) > 0 {
			return out, nil
		}
		switch {
		case err == nil:
			lastErr = fmt.Errorf("%s: empty response from %s", argv[0], url)
		case ctx.Err() != nil:
			lastErr = fmt.Errorf("%s: timed out fetching %s", argv[0], url)
		default:
			msg := err.Error()
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				msg = strings.TrimSpace(string(ee.Stderr))
			}
			lastErr = fmt.Errorf("%s: %s", argv[0], msg)
		}
	}
	return nil, lastErr
}

func md5hex(b []byte) string {
	s := md5.Sum(b)
	return hex.EncodeToString(s[:])
}

func daemonRunning() bool {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	_, err = os.Stat("/proc/" + strings.TrimSpace(string(b)))
	return err == nil
}

// pickUpdate parses a database and returns the binary entry it lists for this platform,
// and whether it differs from the local build (by MD5). Split out so it is testable
// without the network or touching the running executable.
func pickUpdate(dbJSON []byte, localHash string) (entry dbFile, differs bool, err error) {
	var db updateDB
	if err = json.Unmarshal(dbJSON, &db); err != nil {
		return dbFile{}, false, fmt.Errorf("the update database is not valid JSON: %v", err)
	}
	e, ok := db.Files[updateFileKey]
	if !ok || e.URL == "" || e.Hash == "" || e.Size <= 0 {
		return dbFile{}, false, fmt.Errorf("the update database has no usable %q entry", updateFileKey)
	}
	return e, localHash != e.Hash, nil
}

// dbURL is the update database URL: DB_URL from t2o-wifi.ini if set, else the compiled
// default. One editable line survives a repo/Pages rename without a reflash.
func dbURL(base string) string {
	return loadConfig(base).str("DB_URL", updateDBURL)
}

// checkUpdate fetches the database and returns the binary entry it lists, the running
// executable's MD5, and whether they differ.
func checkUpdate(base string) (entry dbFile, localHash string, differs bool, err error) {
	data, err := fetchURL(dbURL(base))
	if err != nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cur, err := os.ReadFile(exe)
	if err != nil {
		return
	}
	localHash = md5hex(cur)
	entry, differs, err = pickUpdate(data, localHash)
	return
}

// updateTag pulls the vX.Y.Z out of a release-asset URL (.../download/vX.Y.Z/t2o-wifi) for
// a friendly message. It is cosmetic only; the MD5 is what decides and verifies an update.
func updateTag(url string) string {
	if _, after, ok := strings.Cut(url, "/download/"); ok {
		if tag, _, ok := strings.Cut(after, "/"); ok {
			return tag
		}
	}
	return "the latest build"
}

// doUpdate downloads the listed binary, verifies its size and MD5, then swaps it in.
// /media/fat is exFAT, so it does not rely on replacing a file that is being executed:
// it stops a running daemon first, renames the verified download over the binary, and
// restarts the daemon if it had been running. A truncated or corrupt download is never
// installed (it would brick both the daemon and this menu, which is how users recover).
func doUpdate(base string, entry dbFile) (restarted bool, err error) {
	bin, err := fetchURL(entry.URL)
	if err != nil {
		return false, err
	}
	if int64(len(bin)) != entry.Size {
		return false, fmt.Errorf("downloaded %d bytes, the database says %d - not installing", len(bin), entry.Size)
	}
	if got := md5hex(bin); got != entry.Hash {
		return false, fmt.Errorf("checksum mismatch (got %s, want %s) - not installing", got, entry.Hash)
	}
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	newPath := exe + ".new"
	if err = os.WriteFile(newPath, bin, 0o755); err != nil {
		return false, fmt.Errorf("writing %s: %v", newPath, err)
	}
	os.Chmod(newPath, 0o755) // best effort - exFAT has no permission bits

	wasRunning := daemonRunning()
	initScript := filepath.Join(base, "S60t2o-wifi")
	if wasRunning {
		exec.Command(initScript, "stop").Run()
		time.Sleep(500 * time.Millisecond)
	}
	if err = os.Rename(newPath, exe); err != nil {
		os.Remove(newPath)
		if wasRunning { // bring the old build back so the display isn't left dark
			exec.Command(initScript, "start").Run()
		}
		return false, fmt.Errorf("could not replace %s: %v", exe, err)
	}
	if wasRunning {
		exec.Command(initScript, "start").Run()
		return true, nil
	}
	return false, nil
}

// runUpdateCLI is `t2o-wifi -update`.
func runUpdateCLI(base string) {
	entry, local, differs, err := checkUpdate(base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "update:", err)
		os.Exit(1)
	}
	if !differs {
		fmt.Printf("t2o-wifi is up to date (%s, build %s).\n", version, local[:8])
		return
	}
	tag := updateTag(entry.URL)
	fmt.Printf("Updating to %s...\n", tag)
	restarted, err := doUpdate(base, entry)
	if err != nil {
		fmt.Fprintln(os.Stderr, "update:", err)
		os.Exit(1)
	}
	if restarted {
		fmt.Printf("Updated to %s and restarted the daemon.\n", tag)
	} else {
		fmt.Printf("Updated to %s. Start the daemon to run it: %s/S60t2o-wifi start\n", tag, base)
	}
}
