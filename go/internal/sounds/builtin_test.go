package sounds

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every embedded WAV must be byte-identical to the macOS resource of the same name, and the two
// folders must hold the same number of WAVs.
func TestBuiltinMatchesSwiftResources(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "AgentChime", "Resources", "Sounds")
	es, err := fs.ReadDir(BuiltinFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range es {
		if !strings.HasSuffix(e.Name(), ".wav") {
			continue
		}
		n++
		got, err := fs.ReadFile(BuiltinFS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from Swift resource", e.Name())
		}
	}
	m := 0
	ds, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ds {
		if strings.HasSuffix(e.Name(), ".wav") {
			m++
		}
	}
	if n != m {
		t.Errorf("%d embedded WAVs, %d Swift resources", n, m)
	}
}
