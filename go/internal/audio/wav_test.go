package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/testenv"
)

// buildWAV makes a WAV with the given fmt chunk body and data.
func buildWAV(fmtBody, data []byte) []byte {
	le := binary.LittleEndian
	b := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	b = le.AppendUint32(b, uint32(len(fmtBody)))
	b = append(b, fmtBody...)
	b = append(b, "data"...)
	b = le.AppendUint32(b, uint32(len(data)))
	b = append(b, data...)
	le.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

func fmtBody(format, bits uint16) []byte {
	le := binary.LittleEndian
	f := make([]byte, 16)
	le.PutUint16(f[0:], format)
	le.PutUint16(f[2:], 1)
	le.PutUint32(f[4:], 8000)
	le.PutUint32(f[8:], 8000*uint32(bits/8))
	le.PutUint16(f[12:], bits/8)
	le.PutUint16(f[14:], bits)
	return f
}

func TestScale16(t *testing.T) {
	in := testenv.WAV16([]int16{1000, -1000, -32768, 32767}, 8000)
	orig := append([]byte(nil), in...)
	out, ok := ScaleWAV(in, 0.5)
	if !ok {
		t.Fatal("not scaled")
	}
	if !bytes.Equal(in, orig) {
		t.Fatal("input mutated")
	}
	w, _ := ParseWAV(out)
	got := func(i int) int16 { return int16(binary.LittleEndian.Uint16(out[w.DataOff+i*2:])) }
	if got(0) != 500 || got(1) != -500 || got(2) != -16384 || got(3) != 16384 {
		t.Fatalf("got %d %d %d %d", got(0), got(1), got(2), got(3))
	}
	same, ok := ScaleWAV(in, 1.0)
	if !ok || !bytes.Equal(same, in) || &same[0] == &in[0] {
		t.Fatal("volume 1.0 must be an identical copy")
	}
	if got := int16(binary.LittleEndian.Uint16(same[w.DataOff+4:])); got != -32768 {
		t.Fatalf("got %d", got)
	}
}

func TestScale8Unsigned(t *testing.T) {
	in := buildWAV(fmtBody(1, 8), []byte{228, 128, 0, 255})
	out, ok := ScaleWAV(in, 0.5)
	if !ok {
		t.Fatal("not scaled")
	}
	w, _ := ParseWAV(out)
	d := out[w.DataOff:]
	if d[0] != 178 || d[1] != 128 || d[2] != 64 || d[3] != 192 {
		t.Fatalf("got %v", d[:4])
	}
}

func TestScale24(t *testing.T) {
	// 1000000 and -1000000 as 24-bit LE.
	enc := func(v int32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16)} }
	data := append(enc(1000000), enc(-1000000)...)
	data = append(data, enc(-8388608)...)
	out, ok := ScaleWAV(buildWAV(fmtBody(1, 24), data), 0.5)
	if !ok {
		t.Fatal("not scaled")
	}
	d := out[len(out)-9:]
	dec := func(b []byte) int32 { return int32(uint32(b[0])<<8|uint32(b[1])<<16|uint32(b[2])<<24) >> 8 }
	if dec(d) != 500000 || dec(d[3:]) != -500000 || dec(d[6:]) != -4194304 {
		t.Fatalf("got %d %d %d", dec(d), dec(d[3:]), dec(d[6:]))
	}
}

func TestScale32Int(t *testing.T) {
	data := binary.LittleEndian.AppendUint32(nil, uint32(1<<30))
	out, ok := ScaleWAV(buildWAV(fmtBody(1, 32), data), 0.5)
	if !ok || int32(binary.LittleEndian.Uint32(out[len(out)-4:])) != 1<<29 {
		t.Fatal("32-bit int not scaled")
	}
}

func TestScaleFloat32(t *testing.T) {
	data := binary.LittleEndian.AppendUint32(nil, math.Float32bits(0.5))
	out, ok := ScaleWAV(buildWAV(fmtBody(3, 32), data), 0.5)
	if !ok {
		t.Fatal("not scaled")
	}
	if f := math.Float32frombits(binary.LittleEndian.Uint32(out[len(out)-4:])); f != 0.25 {
		t.Fatalf("got %v", f)
	}
}

func TestScaleExtensiblePCM(t *testing.T) {
	f := make([]byte, 40)
	le := binary.LittleEndian
	le.PutUint16(f[0:], 0xFFFE)
	le.PutUint16(f[2:], 1)
	le.PutUint32(f[4:], 8000)
	le.PutUint32(f[8:], 16000)
	le.PutUint16(f[12:], 2)
	le.PutUint16(f[14:], 16)
	le.PutUint16(f[16:], 22)
	le.PutUint16(f[18:], 16)
	// GUID 00000001-0000-0010-8000-00aa00389b71
	copy(f[24:], []byte{1, 0, 0, 0, 0, 0, 0x10, 0, 0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71})
	in := buildWAV(f, le.AppendUint16(nil, 1000))
	w, err := ParseWAV(in)
	if err != nil || w.Format != 1 || w.Bits != 16 {
		t.Fatalf("parse: %+v %v", w, err)
	}
	out, ok := ScaleWAV(in, 0.5)
	if !ok || int16(le.Uint16(out[len(out)-2:])) != 500 {
		t.Fatal("extensible PCM not scaled")
	}
}

func TestScaleRejectsADPCM(t *testing.T) {
	if _, ok := ScaleWAV(buildWAV(fmtBody(2, 4), []byte{1, 2, 3, 4}), 0.5); ok {
		t.Fatal("ADPCM must not scale")
	}
	if _, ok := ScaleWAV([]byte("garbage"), 0.5); ok {
		t.Fatal("garbage must not scale")
	}
}

func TestParseWAVTruncated(t *testing.T) {
	in := testenv.WAV16([]int16{1, 2, 3, 4}, 8000)
	if _, err := ParseWAV(in[:len(in)-2]); err == nil {
		t.Fatal("expected truncated error")
	}
}
