// Package testenv isolates tests from the real home directory.
package testenv

import (
	"encoding/binary"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

// Home points AGENTCHIME_HOME at a temp dir and returns the resolved paths.
func Home(t testing.TB) paths.Paths {
	t.Helper()
	t.Setenv("AGENTCHIME_HOME", t.TempDir())
	return paths.Current()
}

// WAV16 returns mono 16-bit PCM WAV bytes.
func WAV16(samples []int16, rate int) []byte {
	dataLen := len(samples) * 2
	b := make([]byte, 44+dataLen)
	le := binary.LittleEndian
	copy(b[0:], "RIFF")
	le.PutUint32(b[4:], uint32(36+dataLen))
	copy(b[8:], "WAVEfmt ")
	le.PutUint32(b[16:], 16)
	le.PutUint16(b[20:], 1) // PCM
	le.PutUint16(b[22:], 1) // mono
	le.PutUint32(b[24:], uint32(rate))
	le.PutUint32(b[28:], uint32(rate*2))
	le.PutUint16(b[32:], 2)
	le.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	le.PutUint32(b[40:], uint32(dataLen))
	for i, s := range samples {
		le.PutUint16(b[44+i*2:], uint16(s))
	}
	return b
}
