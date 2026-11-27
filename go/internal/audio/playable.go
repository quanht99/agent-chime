package audio

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// HeaderMatches reports whether the file starts with the magic bytes that its
// extension promises.
func HeaderMatches(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := make([]byte, 12)
	n, _ := io.ReadFull(f, h)
	h = h[:n]
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		return n >= 12 && bytes.Equal(h[0:4], []byte("RIFF")) && bytes.Equal(h[8:12], []byte("WAVE"))
	case ".oga", ".ogg":
		return bytes.HasPrefix(h, []byte("OggS"))
	case ".flac":
		return bytes.HasPrefix(h, []byte("fLaC"))
	case ".mp3":
		return bytes.HasPrefix(h, []byte("ID3")) || (n >= 2 && h[0] == 0xFF && h[1]&0xE0 == 0xE0)
	case ".aiff", ".aif":
		return n >= 12 && bytes.Equal(h[0:4], []byte("FORM")) &&
			(bytes.Equal(h[8:12], []byte("AIFF")) || bytes.Equal(h[8:12], []byte("AIFC")))
	}
	return false
}

// IsPlayable reports whether path exists, is not empty, has an extension that
// goos/pl can play and a matching header. WAV files must also parse fully.
func IsPlayable(path, goos string, pl Players) bool {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	allowed := false
	for _, e := range AllowedExtensions(goos, pl) {
		if e == ext {
			allowed = true
			break
		}
	}
	if !allowed || !HeaderMatches(path) {
		return false
	}
	if ext == ".wav" {
		b, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		if _, err := ParseWAV(b); err != nil {
			return false
		}
	}
	return true
}
