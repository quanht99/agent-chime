package audio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func write(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlayableRejectsBadFiles(t *testing.T) {
	pl := Players{PwPlay: "pw-play"}
	good := testenv.WAV16([]int16{1, 2, 3, 4}, 8000)
	for _, goos := range []string{"windows", "linux"} {
		if !IsPlayable(write(t, "ok.wav", good), goos, pl) {
			t.Errorf("%s: valid wav rejected", goos)
		}
		if IsPlayable(write(t, "empty.wav", nil), goos, pl) {
			t.Errorf("%s: empty accepted", goos)
		}
		if IsPlayable(write(t, "text.wav", []byte("this is not audio at all")), goos, pl) {
			t.Errorf("%s: text accepted", goos)
		}
		if IsPlayable(write(t, "trunc.wav", good[:len(good)-3]), goos, pl) {
			t.Errorf("%s: truncated accepted", goos)
		}
		if IsPlayable(filepath.Join(t.TempDir(), "missing.wav"), goos, pl) {
			t.Errorf("%s: missing accepted", goos)
		}
	}
}

func TestHeaderMatches(t *testing.T) {
	cases := map[string][]byte{
		"a.ogg":  []byte("OggS\x00\x02"),
		"a.oga":  []byte("OggS\x00\x02"),
		"a.flac": []byte("fLaC\x00"),
		"a.mp3":  []byte("ID3\x04\x00"),
		"b.mp3":  {0xFF, 0xFB, 0x90, 0x00},
		"a.aiff": []byte("FORM\x00\x00\x00\x00AIFF"),
		"a.aif":  []byte("FORM\x00\x00\x00\x00AIFC"),
	}
	for name, b := range cases {
		if !HeaderMatches(write(t, name, b)) {
			t.Errorf("%s should match", name)
		}
		if HeaderMatches(write(t, name, []byte("xxxxxxxxxxxxxxxx"))) {
			t.Errorf("%s should not match junk", name)
		}
	}
	if HeaderMatches(write(t, "a.txt", []byte("RIFF....WAVE"))) {
		t.Error("unknown extension must not match")
	}
}

func TestAllowedExtensions(t *testing.T) {
	has := func(l []string, e string) bool {
		for _, x := range l {
			if x == e {
				return true
			}
		}
		return false
	}
	if has(AllowedExtensions("linux", Players{Aplay: "aplay"}), ".ogg") {
		t.Error("aplay only must not allow .ogg")
	}
	if !has(AllowedExtensions("linux", Players{Paplay: "paplay"}), ".ogg") {
		t.Error("paplay should allow .ogg")
	}
	if got := AllowedExtensions("windows", Players{}); len(got) != 1 || got[0] != ".wav" {
		t.Errorf("windows: %v", got)
	}
	if !has(AllowedExtensions("darwin", Players{}), ".aiff") {
		t.Error("darwin should allow .aiff")
	}
}
