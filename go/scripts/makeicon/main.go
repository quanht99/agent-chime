// Command makeicon draws the tray bell and writes icon.png and icon.ico into
// internal/tray. Run from the go/ directory: go run ./scripts/makeicon
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

const size = 64

func bell() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	fg := color.RGBA{0x1f, 0x6f, 0xeb, 0xff}
	fill := func(r image.Rectangle) { draw.Draw(img, r, &image.Uniform{fg}, image.Point{}, draw.Src) }
	// Dome: a disc clipped to the upper half, then a widening body.
	cx, cy, r := 32.0, 26.0, 16.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dy <= 0 && dx*dx+dy*dy <= r*r {
				img.Set(x, y, fg)
			}
		}
	}
	for y := 26; y < 46; y++ {
		half := 16 + (y-26)*8/20
		fill(image.Rect(32-half, y, 32+half, y+1))
	}
	fill(image.Rect(8, 46, 56, 51)) // rim
	fill(image.Rect(29, 8, 35, 12)) // top knob
	for y := 0; y < 8; y++ {        // clapper
		for x := 0; x < 14; x++ {
			dx, dy := float64(x)-7+0.5, float64(y)+0.5
			if dx*dx+dy*dy*1.5 <= 36 {
				img.Set(25+x, 52+y, fg)
			}
		}
	}
	return img
}

func main() {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, bell()); err != nil {
		log.Fatal(err)
	}
	var ico bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&ico, le, [3]uint16{0, 1, 1}) // ICONDIR: reserved, type icon, count
	ico.Write([]byte{size, size, 0, 0})            // width, height, palette, reserved
	_ = binary.Write(&ico, le, [2]uint16{1, 32})   // planes, bit depth
	_ = binary.Write(&ico, le, uint32(pngBuf.Len()))
	_ = binary.Write(&ico, le, uint32(22)) // offset: 6 + 16
	ico.Write(pngBuf.Bytes())

	dir := filepath.Join("internal", "tray")
	for name, b := range map[string][]byte{"icon.png": pngBuf.Bytes(), "icon.ico": ico.Bytes()} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
