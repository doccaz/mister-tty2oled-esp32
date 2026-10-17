// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Erico Mendonca

package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"regexp"
	"strconv"
)

// The display only takes JPEGs over WiFi (the legacy XBM/GSC transfer is serial-only),
// so the daemon renders the community's 256x64 packs itself: the same way the firmware's
// drawLegacyScaled() does (nearest-neighbour to 240x60, centred on a black 240x240).

const (
	dispSize   = 240
	legacySrcW = 256
	legacySrcH = 64
	legacyDstH = 60
	// The device refuses a CMDCORC picture above 20,000 bytes (COLOR_BUF_MAX).
	maxJPEG = 19000
)

var (
	commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	hexRe     = regexp.MustCompile(`0[xX]([0-9a-fA-F]{2})`)
)

func atoiSafe(s string) (int, error) { return strconv.Atoi(s) }

// parseLegacy reads a tty2oled .gsc/.xbm C-array text file and returns the raw bytes:
// 8192 (4bpp grayscale) or 2048 (1bpp). Some files carry 6 extra header bytes.
func parseLegacy(data []byte) ([]byte, error) {
	data = commentRe.ReplaceAll(data, nil)
	var out []byte
	for _, m := range hexRe.FindAllSubmatch(data, -1) {
		v, _ := strconv.ParseUint(string(m[1]), 16, 8)
		out = append(out, byte(v))
	}
	if len(out) == 8198 || len(out) == 2054 {
		out = out[6:]
	}
	if len(out) != 8192 && len(out) != 2048 {
		return nil, errors.New("unexpected picture size " + strconv.Itoa(len(out)))
	}
	return out, nil
}

// renderLegacy converts raw GSC/XBM bytes to the 240x240 canvas.
func renderLegacy(buf []byte) *image.RGBA {
	isGsc := len(buf) == 8192
	img := image.NewRGBA(image.Rect(0, 0, dispSize, dispSize))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255 // opaque black
	}
	y0 := dispSize/2 - legacyDstH/2
	for y := 0; y < legacyDstH; y++ {
		srcY := y * legacySrcH / legacyDstH
		for x := 0; x < dispSize; x++ {
			srcX := x * legacySrcW / dispSize
			var g uint8
			if isGsc {
				b := buf[srcX/2+srcY*(legacySrcW/2)]
				n := b >> 4
				if srcX%2 == 1 {
					n = b & 0x0F
				}
				g = n * 17
			} else if buf[srcX/8+srcY*(legacySrcW/8)]>>(uint(srcX)%8)&1 == 1 {
				g = 255
			}
			img.SetRGBA(x, y0+y, color.RGBA{g, g, g, 255})
		}
	}
	return img
}

// encodeFit JPEG-encodes img, lowering the quality until it fits the device's buffer.
func encodeFit(img image.Image) ([]byte, error) {
	for q := 90; q >= 30; q -= 10 {
		var b bytes.Buffer
		if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if b.Len() <= maxJPEG {
			return b.Bytes(), nil
		}
	}
	return nil, errors.New("picture too large for the device even at low quality")
}

func legacyToJPEG(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := parseLegacy(data)
	if err != nil {
		return nil, err
	}
	return encodeFit(renderLegacy(raw))
}

// fitJPEG returns data unchanged when it already fits the device, otherwise
// re-encodes it with lower quality.
func fitJPEG(data []byte) ([]byte, error) {
	if len(data) <= maxJPEG {
		return data, nil
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return encodeFit(img)
}
