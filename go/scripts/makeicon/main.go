// Command makeicon draws the tray bell and writes icon.png and icon.ico into
// internal/tray, plus the Windows app.ico and the 256 px PNG under scripts/. Run from the go/ directory: go run ./scripts/makeicon
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

// inBell reports whether the point (x, y), in the 64 x 64 design grid, lies
// inside the bell shape. It mirrors bell() with continuous coordinates.
func inBell(x, y float64) bool {
	if y < 26 {
		dx, dy := x-32, y-26
		if dx*dx+dy*dy <= 16*16 {
			return true
		}
	} else if y < 46 {
		half := 16 + (y-26)*8/20
		if x >= 32-half && x < 32+half {
			return true
		}
	}
	if y >= 46 && y < 51 && x >= 8 && x < 56 {
		return true
	}
	if y >= 8 && y < 12 && x >= 29 && x < 35 {
		return true
	}
	if y >= 52 && y < 60 && x >= 25 && x < 39 {
		dx, dy := x-32, y-52
		return dx*dx+dy*dy*1.5 <= 36
	}
	return false
}

// bellAt draws the bell at n x n pixels with 4 x 4 supersampling.
func bellAt(n int) *image.RGBA {
	const ss = 4
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	scale := float64(size) / float64(n)
	for py := 0; py < n; py++ {
		for px := 0; px < n; px++ {
			hits := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) * scale
					y := (float64(py) + (float64(sy)+0.5)/ss) * scale
					if inBell(x, y) {
						hits++
					}
				}
			}
			if hits == 0 {
				continue
			}
			a := uint8(hits * 255 / (ss * ss))
			// Premultiplied colour of 0x1f6feb at coverage a.
			img.SetRGBA(px, py, color.RGBA{
				uint8(0x1f * int(a) / 255), uint8(0x6f * int(a) / 255), uint8(0xeb * int(a) / 255), a,
			})
		}
	}
	return img
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		log.Fatal(err)
	}
	return b.Bytes()
}

// icoFile packs the images as PNG-in-ICO entries. Sizes of 256 are stored as 0.
func icoFile(imgs []*image.RGBA) []byte {
	le := binary.LittleEndian
	pngs := make([][]byte, len(imgs))
	for i, im := range imgs {
		pngs[i] = pngBytes(im)
	}
	var ico bytes.Buffer
	_ = binary.Write(&ico, le, [3]uint16{0, 1, uint16(len(imgs))})
	offset := 6 + 16*len(imgs)
	for i, im := range imgs {
		w, h := im.Bounds().Dx(), im.Bounds().Dy()
		ico.Write([]byte{byte(w % 256), byte(h % 256), 0, 0})
		_ = binary.Write(&ico, le, [2]uint16{1, 32})
		_ = binary.Write(&ico, le, uint32(len(pngs[i])))
		_ = binary.Write(&ico, le, uint32(offset))
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		ico.Write(p)
	}
	return ico.Bytes()
}

func main() {
	pngBuf := pngBytes(bell())
	var ico bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&ico, le, [3]uint16{0, 1, 1}) // ICONDIR: reserved, type icon, count
	ico.Write([]byte{size, size, 0, 0})            // width, height, palette, reserved
	_ = binary.Write(&ico, le, [2]uint16{1, 32})   // planes, bit depth
	_ = binary.Write(&ico, le, uint32(len(pngBuf)))
	_ = binary.Write(&ico, le, uint32(22)) // offset: 6 + 16
	ico.Write(pngBuf)

	write := func(path string, b []byte) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	dir := filepath.Join("internal", "tray")
	write(filepath.Join(dir, "icon.png"), pngBuf)
	write(filepath.Join(dir, "icon.ico"), ico.Bytes())
	big := bellAt(256)
	write(filepath.Join("scripts", "winres", "app.ico"),
		icoFile([]*image.RGBA{bellAt(16), bellAt(32), bellAt(48), big}))
	write(filepath.Join("scripts", "icons", "agentchime-256.png"), pngBytes(big))
}
