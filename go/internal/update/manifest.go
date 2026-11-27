// Package update holds the cgo-free core of AgentChime's self-update: the
// signed manifest, version comparison and the allowed-URL rules.
package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// ReleaseBase is the only place releases are fetched from.
const ReleaseBase = "https://github.com/quanht99/agent-chime/releases/"

const (
	MaxManifest = 64 << 10
	MaxSig      = 200
	MaxArchive  = 100 << 20
)

var (
	// ErrUnverified: the signature or the checksum failed.
	ErrUnverified = errors.New("update: signature or checksum failed")
	// ErrBadManifest: the signature is fine but the content is invalid.
	ErrBadManifest = errors.New("update: invalid manifest")
	// ErrNoFile: the manifest has no archive for this platform.
	ErrNoFile = errors.New("update: no file for this platform")
)

type File struct {
	OS     string `json:"os"`   // "macos" | "windows" | "linux"
	Arch   string `json:"arch"` // "universal" | "amd64" | "arm64"
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"` // lowercase hex
}

type Manifest struct {
	Format      int    `json:"format"` // 1
	Version     string `json:"version"`
	ReleasePage string `json:"releasePage"`
	Files       []File `json:"files"`
}

// ParseVerified verifies sig (std base64 of an Ed25519 signature) over body
// before parsing it, then validates the content.
func ParseVerified(body, sig []byte, key ed25519.PublicKey) (Manifest, error) {
	if len(body) > MaxManifest || len(sig) > MaxSig || len(key) != ed25519.PublicKeySize {
		return Manifest{}, ErrUnverified
	}
	raw, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return Manifest{}, ErrUnverified
	}
	if !ed25519.Verify(key, body, raw) {
		return Manifest{}, ErrUnverified
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return Manifest{}, ErrBadManifest
	}
	if m.Format != 1 {
		return Manifest{}, ErrBadManifest
	}
	if _, ok := parseVersion(m.Version); !ok {
		return Manifest{}, ErrBadManifest
	}
	if !strings.HasPrefix(m.ReleasePage, ReleaseBase) {
		return Manifest{}, ErrBadManifest
	}
	prefix := "AgentChime-" + m.Version + "-"
	for _, f := range m.Files {
		if !strings.HasPrefix(f.Name, prefix) || strings.ContainsAny(f.Name, `/\`) || strings.Contains(f.Name, "..") {
			return Manifest{}, ErrBadManifest
		}
		if f.Size < 1 || f.Size > MaxArchive || !isLowerHex64(f.SHA256) {
			return Manifest{}, ErrBadManifest
		}
	}
	return m, nil
}

func isLowerHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// FileFor picks the archive for a Go GOOS/GOARCH pair.
func (m Manifest) FileFor(goos, goarch string) (File, error) {
	var osName, arch string
	switch goos {
	case "windows", "linux":
		if goarch != "amd64" && goarch != "arm64" {
			return File{}, ErrNoFile
		}
		osName, arch = goos, goarch
	case "darwin":
		osName, arch = "macos", "universal"
	default:
		return File{}, ErrNoFile
	}
	for _, f := range m.Files {
		if f.OS == osName && f.Arch == arch {
			return f, nil
		}
	}
	return File{}, ErrNoFile
}

// parseVersion accepts strict X.Y.Z with an optional leading "v".
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		if p == "" || len(p) > 9 {
			return v, false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return v, false
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// Compare returns -1/0/1 and true when both versions parse, else 0, false.
func Compare(a, b string) (int, bool) {
	va, ok1 := parseVersion(a)
	vb, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return 0, false
	}
	for i := range va {
		if va[i] < vb[i] {
			return -1, true
		}
		if va[i] > vb[i] {
			return 1, true
		}
	}
	return 0, true
}
