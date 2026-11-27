package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"reflect"
	"testing"
)

func TestParseVerifiedAccepts(t *testing.T) {
	want := goodManifest("0.3.1")
	body := mustJSON(t, want)
	got, err := ParseVerified(body, sign(t, body), testKey(t).Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestParseVerifiedWrongKey(t *testing.T) {
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	body := mustJSON(t, goodManifest("0.3.1"))
	_, err := ParseVerified(body, signWith(other, body), testKey(t).Public().(ed25519.PublicKey))
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseVerifiedChangedByte(t *testing.T) {
	body := mustJSON(t, goodManifest("0.3.1"))
	sig := sign(t, body)
	body[10] ^= 1
	_, err := ParseVerified(body, sig, testKey(t).Public().(ed25519.PublicKey))
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseVerifiedMissingSig(t *testing.T) {
	body := mustJSON(t, goodManifest("0.3.1"))
	pub := testKey(t).Public().(ed25519.PublicKey)
	for _, sig := range [][]byte{nil, []byte("not base64 !!!")} {
		if _, err := ParseVerified(body, sig, pub); !errors.Is(err, ErrUnverified) {
			t.Fatalf("sig %q: err = %v", sig, err)
		}
	}
}

func TestParseVerifiedTooBig(t *testing.T) {
	body := make([]byte, MaxManifest+1)
	for i := range body {
		body[i] = ' '
	}
	_, err := ParseVerified(body, sign(t, body), testKey(t).Public().(ed25519.PublicKey))
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseVerifiedNilOrShortKey(t *testing.T) {
	body := mustJSON(t, goodManifest("0.3.1"))
	for _, k := range []ed25519.PublicKey{nil, make(ed25519.PublicKey, 5)} {
		if _, err := ParseVerified(body, sign(t, body), k); !errors.Is(err, ErrUnverified) {
			t.Fatalf("err = %v", err)
		}
	}
}

func TestParseVerifiedBadContent(t *testing.T) {
	cases := map[string]func(*Manifest){
		"format 2":        func(m *Manifest) { m.Format = 2 },
		"version dev":     func(m *Manifest) { m.Version = "dev" },
		"release page":    func(m *Manifest) { m.ReleasePage = "https://evil.example/" },
		"name traversal":  func(m *Manifest) { m.Files[0].Name = "../AgentChime-0.3.1-x" },
		"name backslash":  func(m *Manifest) { m.Files[0].Name = `AgentChime-0.3.1-a\b` },
		"name wrong ver":  func(m *Manifest) { m.Files[3].Name = "AgentChime-0.3.0-linux-amd64.tar.gz" },
		"size zero":       func(m *Manifest) { m.Files[0].Size = 0 },
		"size too big":    func(m *Manifest) { m.Files[0].Size = MaxArchive + 1 },
		"sha upper short": func(m *Manifest) { m.Files[0].SHA256 = "ABC" },
	}
	pub := testKey(t).Public().(ed25519.PublicKey)
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			m := goodManifest("0.3.1")
			mut(&m)
			body := mustJSON(t, m)
			if _, err := ParseVerified(body, sign(t, body), pub); !errors.Is(err, ErrBadManifest) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestFileForEachPair(t *testing.T) {
	m := goodManifest("0.3.1")
	ok := map[[2]string]string{
		{"windows", "amd64"}: "AgentChime-0.3.1-windows-amd64.zip",
		{"windows", "arm64"}: "AgentChime-0.3.1-windows-arm64.zip",
		{"linux", "amd64"}:   "AgentChime-0.3.1-linux-amd64.tar.gz",
		{"linux", "arm64"}:   "AgentChime-0.3.1-linux-arm64.tar.gz",
		{"darwin", "arm64"}:  "AgentChime-0.3.1-macos.zip",
	}
	for k, name := range ok {
		f, err := m.FileFor(k[0], k[1])
		if err != nil || f.Name != name {
			t.Fatalf("%v: %v %v", k, f.Name, err)
		}
	}
	for _, k := range [][2]string{{"linux", "386"}, {"freebsd", "amd64"}} {
		if _, err := m.FileFor(k[0], k[1]); !errors.Is(err, ErrNoFile) {
			t.Fatalf("%v: err = %v", k, err)
		}
	}
	m.Files = append(m.Files[:4:4], m.Files[5:]...)
	if _, err := m.FileFor("linux", "arm64"); !errors.Is(err, ErrNoFile) {
		t.Fatalf("missing entry: err = %v", err)
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		r    int
		ok   bool
	}{
		{"0.3.0", "0.3.1", -1, true},
		{"0.3.1", "0.3.1", 0, true},
		{"v0.3.1", "0.3.1", 0, true},
		{"0.10.0", "0.9.0", 1, true},
		{"1.0.0", "0.99.99", 1, true},
		{"dev", "0.3.0", 0, false},
		{"0.3", "0.3.0", 0, false},
		{"0.3.0-rc1", "0.3.0", 0, false},
		{"e2e-new", "0.3.0", 0, false},
	}
	for _, c := range cases {
		r, ok := Compare(c.a, c.b)
		if r != c.r || ok != c.ok {
			t.Errorf("Compare(%q,%q) = %d,%v want %d,%v", c.a, c.b, r, ok, c.r, c.ok)
		}
	}
}
