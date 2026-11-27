package update

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vectorDir = "../../../AgentChime/Tests/Fixtures/update"

func TestSwiftVectors(t *testing.T) {
	priv := testKey(t)
	pub := priv.Public().(ed25519.PublicKey)

	if os.Getenv("AGENTCHIME_WRITE_VECTORS") == "1" {
		// A fixed, throwaway key so the vector is reproducible. Not a secret.
		seed := make([]byte, ed25519.SeedSize)
		for i := range seed {
			seed[i] = byte(i + 1)
		}
		other := ed25519.NewKeyFromSeed(seed)

		good := mustJSON(t, goodManifest("0.9.1"))
		tampered := append([]byte(nil), good...)
		tampered[10] ^= 1
		f2 := goodManifest("0.9.1")
		f2.Format = 2
		f2b := mustJSON(t, f2)
		nm := goodManifest("0.9.1")
		nm.Files = nm.Files[1:]
		nmb := mustJSON(t, nm)

		files := map[string][]byte{
			"test-public-key.hex": []byte(hex.EncodeToString(pub) + "\n"),
			"good.json":           good,
			"good.json.sig":       sign(t, good),
			"tampered.json":       tampered,
			"wrongkey.json.sig":   signWith(other, good),
			"format2.json":        f2b,
			"format2.json.sig":    sign(t, f2b),
			"nomac.json":          nmb,
			"nomac.json.sig":      sign(t, nmb),
		}
		if err := os.MkdirAll(vectorDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for n, b := range files {
			if err := os.WriteFile(filepath.Join(vectorDir, n), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	rd := func(n string) []byte {
		b, err := os.ReadFile(filepath.Join(vectorDir, n))
		if err != nil {
			t.Fatalf("%v (run with AGENTCHIME_WRITE_VECTORS=1)", err)
		}
		return b
	}
	hx, err := hex.DecodeString(strings.TrimSpace(string(rd("test-public-key.hex"))))
	if err != nil || !ed25519.PublicKey(hx).Equal(pub) {
		t.Fatal("public key vector mismatch")
	}
	k := ed25519.PublicKey(hx)
	if m, err := ParseVerified(rd("good.json"), rd("good.json.sig"), k); err != nil || m.Version != "0.9.1" || len(m.Files) != 5 {
		t.Fatalf("good: %v %+v", err, m)
	}
	if _, err := ParseVerified(rd("tampered.json"), rd("good.json.sig"), k); !errors.Is(err, ErrUnverified) {
		t.Fatalf("tampered: %v", err)
	}
	if _, err := ParseVerified(rd("good.json"), rd("wrongkey.json.sig"), k); !errors.Is(err, ErrUnverified) {
		t.Fatalf("wrongkey: %v", err)
	}
	if _, err := ParseVerified(rd("format2.json"), rd("format2.json.sig"), k); !errors.Is(err, ErrBadManifest) {
		t.Fatalf("format2: %v", err)
	}
	m, err := ParseVerified(rd("nomac.json"), rd("nomac.json.sig"), k)
	if err != nil {
		t.Fatalf("nomac: %v", err)
	}
	if _, err := m.FileFor("darwin", "arm64"); !errors.Is(err, ErrNoFile) {
		t.Fatalf("nomac FileFor: %v", err)
	}
}
