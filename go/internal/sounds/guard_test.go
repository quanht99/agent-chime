package sounds

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var builtinWavNames = []string{
	"Aurora", "Bloom", "Breeze", "Chime", "Crystal", "Ding", "Drift",
	"Ember", "Knock", "Lumen", "Pebble", "Ripple", "Rise", "Tide",
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Skipf("no working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("no .git found above the test directory")
		}
		dir = parent
	}
}

// TestOnlyBuiltinWavsInRepo fails if any WAV other than the 14 built-in sounds
// is tracked or sits untracked-but-not-ignored, or if the macOS and Go copies differ.
func TestOnlyBuiltinWavsInRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := findRepoRoot(t)
	cmd := exec.Command("git", "ls-files", "-co", "--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var want []string
	for _, n := range builtinWavNames {
		want = append(want, "AgentChime/Resources/Sounds/"+n+".wav", "go/internal/sounds/builtin/"+n+".wav")
	}
	sort.Strings(want)
	var got []string
	for _, p := range strings.Split(string(out), "\n") {
		p = strings.TrimSpace(p)
		if strings.HasSuffix(strings.ToLower(p), ".wav") {
			got = append(got, p)
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WAV files in the repo:\n got  %v\n want %v", got, want)
	}
	for _, n := range builtinWavNames {
		a, errA := os.ReadFile(filepath.Join(root, "AgentChime", "Resources", "Sounds", n+".wav"))
		b, errB := os.ReadFile(filepath.Join(root, "go", "internal", "sounds", "builtin", n+".wav"))
		if errA != nil || errB != nil {
			t.Errorf("%s: %v %v", n, errA, errB)
			continue
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s.wav differs between AgentChime/Resources/Sounds and go/internal/sounds/builtin", n)
		}
	}
}
