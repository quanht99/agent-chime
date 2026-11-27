//go:build updatetest

package update

import (
	"crypto/ed25519"
	"encoding/hex"
	"net/url"
	"os"
)

// testPublicKeyHex matches testdata/TEST-ONLY-public-key.hex. TEST ONLY.
const testPublicKeyHex = "5d4b0ea694e2958ee299813abf5eecc6de5c41a32580efa54358f391fcd49441"

// TestBuild is true only in -tags updatetest builds.
const TestBuild = true

// Key returns the test public key.
func Key() ed25519.PublicKey {
	b, err := hex.DecodeString(testPublicKeyHex)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}

// Base is AGENTCHIME_TEST_UPDATE_URL when set, else the releases URL.
func Base() string {
	if v := os.Getenv("AGENTCHIME_TEST_UPDATE_URL"); v != "" {
		return v
	}
	return ReleaseBase
}

// Allowed also accepts plain http to 127.0.0.1 in test builds.
func Allowed(u *url.URL) bool {
	if AllowedURL(u) {
		return true
	}
	return u != nil && u.Scheme == "http" && u.User == nil && u.Hostname() == "127.0.0.1"
}
