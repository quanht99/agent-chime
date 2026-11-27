// Package audio parses and scales WAV data, decides whether a sound file is
// playable, picks a Linux player and plays sounds on each OS.
package audio

import (
	"encoding/binary"
	"errors"
	"math"
)

// ErrNoPlayer means no usable sound player was found.
var ErrNoPlayer = errors.New("no sound player found")

const (
	formatPCM        = 1
	formatFloat      = 3
	formatExtensible = 0xFFFE
)

// WAV describes where the samples of a WAV file live.
type WAV struct {
	Format   uint16 // 1 PCM, 3 float; 0xFFFE is resolved to its sub-format
	Channels uint16
	Rate     uint32
	Bits     uint16
	DataOff  int
	DataLen  int
}

// ParseWAV walks the RIFF chunks and returns the format and data location.
// It fails on a missing fmt or data chunk and on a truncated data chunk.
func ParseWAV(b []byte) (WAV, error) {
	var w WAV
	le := binary.LittleEndian
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return w, errors.New("not a RIFF/WAVE file")
	}
	haveFmt := false
	pos := 12
	for pos+8 <= len(b) {
		id := string(b[pos : pos+4])
		size := int(le.Uint32(b[pos+4:]))
		body := pos + 8
		switch id {
		case "fmt ":
			if size < 16 || body+size > len(b) {
				return w, errors.New("bad fmt chunk")
			}
			w.Format = le.Uint16(b[body:])
			w.Channels = le.Uint16(b[body+2:])
			w.Rate = le.Uint32(b[body+4:])
			w.Bits = le.Uint16(b[body+14:])
			if w.Format == formatExtensible {
				// WAVEFORMATEXTENSIBLE: cbSize at 16, validBits 18, mask 20, GUID 24.
				if size < 40 || le.Uint16(b[body+16:]) < 22 {
					return w, errors.New("bad extensible fmt chunk")
				}
				w.Format = le.Uint16(b[body+24:])
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return w, errors.New("data chunk before fmt chunk")
			}
			if size > len(b)-body {
				return w, errors.New("truncated data chunk")
			}
			w.DataOff = body
			w.DataLen = size
			return w, nil
		}
		next := body + size + size&1
		if next <= pos || next > len(b) {
			break
		}
		pos = next
	}
	return w, errors.New("no data chunk")
}

// ScaleWAV returns a scaled copy of b. It reports false when the format is not
// one it can scale (PCM 8/16/24/32 bit or 32-bit float).
func ScaleWAV(b []byte, volume float64) ([]byte, bool) {
	w, err := ParseWAV(b)
	if err != nil {
		return nil, false
	}
	switch {
	case w.Format == formatPCM && (w.Bits == 8 || w.Bits == 16 || w.Bits == 24 || w.Bits == 32):
	case w.Format == formatFloat && w.Bits == 32:
	default:
		return nil, false
	}
	if math.IsNaN(volume) {
		volume = 1
	}
	volume = math.Min(1, math.Max(0, volume))
	out := make([]byte, len(b))
	copy(out, b)
	if volume == 1 {
		return out, true
	}
	d := out[w.DataOff : w.DataOff+w.DataLen]
	le := binary.LittleEndian
	switch {
	case w.Format == formatFloat:
		for i := 0; i+4 <= len(d); i += 4 {
			s := math.Float32frombits(le.Uint32(d[i:]))
			le.PutUint32(d[i:], math.Float32bits(float32(float64(s)*volume)))
		}
	case w.Bits == 8:
		for i := range d {
			d[i] = byte(clamp(128+math.Round((float64(d[i])-128)*volume), 0, 255))
		}
	case w.Bits == 16:
		for i := 0; i+2 <= len(d); i += 2 {
			s := float64(int16(le.Uint16(d[i:])))
			le.PutUint16(d[i:], uint16(int16(clamp(math.Round(s*volume), math.MinInt16, math.MaxInt16))))
		}
	case w.Bits == 24:
		for i := 0; i+3 <= len(d); i += 3 {
			s := int32(uint32(d[i])<<8|uint32(d[i+1])<<16|uint32(d[i+2])<<24) >> 8
			v := int32(clamp(math.Round(float64(s)*volume), -1<<23, 1<<23-1))
			d[i], d[i+1], d[i+2] = byte(v), byte(v>>8), byte(v>>16)
		}
	case w.Bits == 32:
		for i := 0; i+4 <= len(d); i += 4 {
			s := float64(int32(le.Uint32(d[i:])))
			le.PutUint32(d[i:], uint32(int32(clamp(math.Round(s*volume), math.MinInt32, math.MaxInt32))))
		}
	}
	return out, true
}

func clamp(v, lo, hi float64) float64 { return math.Min(hi, math.Max(lo, v)) }
