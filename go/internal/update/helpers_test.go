package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	raw, err := os.ReadFile("testdata/TEST-ONLY-signing-seed.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		seed, err := base64.StdEncoding.DecodeString(line)
		if err != nil {
			t.Fatal(err)
		}
		return ed25519.NewKeyFromSeed(seed)
	}
	t.Fatal("no seed")
	return nil
}

func signWith(priv ed25519.PrivateKey, body []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, body)))
}

func sign(t *testing.T, body []byte) []byte {
	t.Helper()
	return signWith(testKey(t), body)
}

func goodManifest(version string) Manifest {
	p := "AgentChime-" + version + "-"
	sum := strings.Repeat("ab", 32)
	return Manifest{
		Format:      1,
		Version:     version,
		ReleasePage: ReleaseBase + "tag/v" + version,
		Files: []File{
			{OS: "macos", Arch: "universal", Name: p + "macos.zip", Size: 1000, SHA256: sum},
			{OS: "windows", Arch: "amd64", Name: p + "windows-amd64.zip", Size: 1000, SHA256: sum},
			{OS: "windows", Arch: "arm64", Name: p + "windows-arm64.zip", Size: 1000, SHA256: sum},
			{OS: "linux", Arch: "amd64", Name: p + "linux-amd64.tar.gz", Size: 1000, SHA256: sum},
			{OS: "linux", Arch: "arm64", Name: p + "linux-arm64.tar.gz", Size: 1000, SHA256: sum},
		},
	}
}

func mustJSON(t *testing.T, m Manifest) []byte {
	t.Helper()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}
