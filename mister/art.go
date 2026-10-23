// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ArtFinder looks up a picture for a core name. Search order:
//  1. <base>/art/<core>.jpg - the user's own full-colour JPEGs (exported from the web app)
//  2. <base>/pics_pri/<core>.gsc|xbm - the stock "priority" pictures
//  3. <base>/art-pd/<core>.jpg - the downloaded public-domain colour pack (never edited by
//     hand: the Downloader owns that folder, so user art stays in art/)
//  4. <base>/pics/<GSC_US|XBM_US|GSC|XBM|XBM_TEXT>/ per the USE_* switches
//
// Like the stock daemon, a name that has no match is shortened one character at a time
// ("jackalfixed" -> "jackalfixe" -> ... -> "jackal"), and _altN variants are diced.
type ArtFinder struct {
	base  string // the daemon's folder: art/
	stock string // the stock tty2oled folder: pics/, pics_pri/ (empty: same as base)
	cfg   Config
}

type Art struct {
	JPEG []byte
	Path string
}

// lookup returns the path of <dir>/<name>.<ext> for any of exts, case-insensitively.
func lookup(dir, name string, exts ...string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	want := map[string]string{}
	for _, e := range exts {
		want[strings.ToLower(name+"."+e)] = e
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if _, ok := want[strings.ToLower(e.Name())]; ok {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// altPick returns one of path or its <stem>_altN siblings at random (stock USE_RANDOM_ALT).
func altPick(path string) string {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	matches, _ := filepath.Glob(stem + "_alt*")
	var alts []string
	for _, m := range matches {
		if strings.EqualFold(filepath.Ext(m), ext) {
			alts = append(alts, m)
		}
	}
	if len(alts) == 0 {
		return path
	}
	if n := rand.Intn(len(alts) + 1); n > 0 {
		return alts[n-1]
	}
	return path
}

func (f *ArtFinder) folders() []string {
	var list []string
	if f.cfg.yes("USE_GSC_PICTURE", true) {
		if f.cfg.yes("USE_US_PICTURE", false) {
			list = append(list, "gsc_us")
		}
		list = append(list, "gsc")
	}
	if f.cfg.yes("USE_US_PICTURE", false) {
		list = append(list, "xbm_us")
	}
	list = append(list, "xbm")
	if f.cfg.yes("USE_TEXT_PICTURE", false) {
		list = append(list, "xbm_text")
	}
	return list
}

func (f *ArtFinder) findFile(core string) string {
	stock := f.stock
	if stock == "" {
		stock = f.base
	}
	artDir := filepath.Join(f.base, "art")
	packDir := filepath.Join(f.base, "art-pd") // the downloaded public-domain pack
	priDir := filepath.Join(stock, "pics_pri")
	for n := len(core); n >= 1; n-- {
		name := core[:n]
		if p := lookup(artDir, name, "jpg", "jpeg"); p != "" {
			return p
		}
		if n == len(core) { // the stock _pri folder only matches the exact name
			if p := lookup(priDir, name, "gsc", "xbm"); p != "" {
				return p
			}
		}
		// The pack comes after the user's own art at the same name length, so a user's
		// file always wins, but a longer pack match still beats a shorter user match.
		if p := lookup(packDir, name, "jpg", "jpeg"); p != "" {
			return p
		}
	}
	for _, folder := range f.folders() {
		dir := filepath.Join(stock, "pics", strings.ToUpper(folder))
		ext := folder[:3]
		for n := len(core); n >= 1; n-- {
			if p := lookup(dir, core[:n], ext); p != "" {
				return p
			}
		}
	}
	return ""
}

// Find returns the picture for core, or nil when there is none (the caller then sends
// just the name and the device draws it as text).
func (f *ArtFinder) Find(core string) (*Art, error) {
	p := f.findFile(core)
	if p == "" {
		return nil, nil
	}
	if f.cfg.yes("USE_RANDOM_ALT", true) {
		p = altPick(p)
	}
	var data []byte
	var err error
	switch strings.ToLower(filepath.Ext(p)) {
	case ".jpg", ".jpeg":
		if data, err = os.ReadFile(p); err == nil {
			data, err = fitJPEG(data)
		}
	default:
		data, err = legacyToJPEG(p)
	}
	if err != nil {
		return nil, err
	}
	return &Art{JPEG: data, Path: p}, nil
}

// artCache lets several displays share one lookup (and one JPEG conversion) of the same
// core change, and the same random _alt pick.
type artCache struct {
	mu   sync.Mutex
	core string
	art  *Art
	err  error
	at   time.Time
}

var sharedArt artCache

// FindShared is Find with a 5 s single-entry cache.
func (f *ArtFinder) FindShared(core string) (*Art, error) {
	sharedArt.mu.Lock()
	defer sharedArt.mu.Unlock()
	if sharedArt.core == core && time.Since(sharedArt.at) < 5*time.Second && !sharedArt.at.IsZero() {
		return sharedArt.art, sharedArt.err
	}
	art, err := f.Find(core)
	sharedArt.core, sharedArt.art, sharedArt.err, sharedArt.at = core, art, err, time.Now()
	return art, err
}

// Names returns every core name that has a picture somewhere the finder looks (art/,
// art-pd/, then the stock pics folders), without extensions or _altN variants, de-duplicated
// case-insensitively and sorted. It feeds the menu's "pick a core" list.
func (f *ArtFinder) Names() []string {
	stock := f.stock
	if stock == "" {
		stock = f.base
	}
	type src struct {
		dir  string
		exts []string
	}
	srcs := []src{
		{filepath.Join(f.base, "art"), []string{"jpg", "jpeg"}},
		{filepath.Join(f.base, "art-pd"), []string{"jpg", "jpeg"}},
	}
	for _, folder := range f.folders() {
		srcs = append(srcs, src{filepath.Join(stock, "pics", strings.ToUpper(folder)), []string{folder[:3]}})
	}
	seen := map[string]string{}
	for _, s := range srcs {
		ents, err := os.ReadDir(s.dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(e.Name())), ".")
			ok := false
			for _, x := range s.exts {
				ok = ok || ext == x
			}
			if !ok {
				continue
			}
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if i := strings.LastIndex(strings.ToLower(name), "_alt"); i > 0 {
				name = name[:i]
			}
			if _, dup := seen[strings.ToLower(name)]; !dup {
				seen[strings.ToLower(name)] = name
			}
		}
	}
	names := make([]string, 0, len(seen))
	for _, n := range seen {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names
}
