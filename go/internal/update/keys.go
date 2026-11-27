//go:build !updatetest

package update

import (
	"crypto/ed25519"
	"encoding/hex"
	"net/url"
)

// releasePublicKeyHex is the Ed25519 public key (hex) that signs releases.
// The private half lives only in the UPDATE_SIGNING_KEY Actions secret. Changing this key means
// copies built with the old one can't verify new releases, so their users update by hand once.
const releasePublicKeyHex = "6555c7a7ff8beca7d3286b7b8dd04ff3e5fcd880281ffcf2f54a03457d4929fe"

// TestBuild is false in release builds.
const TestBuild = false

// Key returns the compiled-in release key, or nil when none is set.
func Key() ed25519.PublicKey {
	b, err := hex.DecodeString(releasePublicKeyHex)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}

// Base is the releases URL prefix.
func Base() string { return ReleaseBase }

// Allowed reports whether u may be fetched.
func Allowed(u *url.URL) bool { return AllowedURL(u) }
