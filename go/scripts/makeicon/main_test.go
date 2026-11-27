package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"
)

func TestICOHasFourSizes(t *testing.T) {
	data := icoFile([]*image.RGBA{bellAt(16), bellAt(32), bellAt(48), bellAt(256)})
	le := binary.LittleEndian
	if le.Uint16(data[0:]) != 0 || le.Uint16(data[2:]) != 1 || le.Uint16(data[4:]) != 4 {
		t.Fatalf("bad ICONDIR: % x", data[:6])
	}
	want := []int{16, 32, 48, 256}
	for i, w := range want {
		e := data[6+16*i:]
		b := int(e[0])
		if w == 256 {
			if e[0] != 0 || e[1] != 0 {
				t.Fatalf("entry %d: want 0x0 bytes, got %d,%d", i, e[0], e[1])
			}
			b = 256
		} else if int(e[0]) != w || int(e[1]) != w {
			t.Fatalf("entry %d: size bytes %d,%d want %d", i, e[0], e[1], w)
		}
		n, off := le.Uint32(e[8:]), le.Uint32(e[12:])
		img, err := png.Decode(bytes.NewReader(data[off : off+n]))
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if got := img.Bounds().Dx(); got != b || img.Bounds().Dy() != b {
			t.Fatalf("entry %d: png %dx%d want %d", i, got, img.Bounds().Dy(), b)
		}
	}
}
