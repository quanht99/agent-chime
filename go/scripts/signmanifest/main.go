// Command signmanifest writes and signs latest.json for a release. CI runs it
// with the private seed in the environment variable UPDATE_SIGNING_KEY.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/quanht99/agent-chime/go/internal/update"
)

var plainVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

const defaultRequire = "macos,windows-amd64,windows-arm64,linux-amd64,linux-arm64"

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "signmanifest:", err)
		os.Exit(1)
	}
}

func run(args []string, env func(string) string, stdout io.Writer) error {
	fs := flag.NewFlagSet("signmanifest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	version := fs.String("version", "", "release version X.Y.Z")
	dir := fs.String("dir", "", "folder with the archives")
	out := fs.String("out", "", "folder to write latest.json and latest.json.sig to")
	require := fs.String("require", defaultRequire, "comma-separated targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" || *dir == "" || *out == "" {
		return errors.New("-version, -dir and -out are required")
	}
	if !plainVersion.MatchString(*version) {
		return errors.New("-version must be plain X.Y.Z, without a v prefix")
	}

	// Never include the secret, or any error that may quote it, in messages.
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env("UPDATE_SIGNING_KEY")))
	if err != nil {
		return errors.New("UPDATE_SIGNING_KEY is not valid base64")
	}
	if len(seed) != ed25519.SeedSize {
		return errors.New("UPDATE_SIGNING_KEY must be a 32-byte base64 seed")
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := update.Key()
	if pub == nil {
		return errors.New("release public key not set")
	}
	if !bytes.Equal(priv.Public().(ed25519.PublicKey), pub) {
		return errors.New("signing key does not match the compiled-in public key")
	}

	m := update.Manifest{
		Format:      1,
		Version:     *version,
		ReleasePage: update.ReleaseBase + "tag/v" + *version,
	}
	for _, target := range strings.Split(*require, ",") {
		target = strings.TrimSpace(target)
		f, err := fileFor(*version, target)
		if err != nil {
			return err
		}
		path := filepath.Join(*dir, f.Name)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("missing file for %s: %s", target, f.Name)
		}
		sum := sha256.Sum256(data)
		f.Size = int64(len(data))
		f.SHA256 = hex.EncodeToString(sum[:])
		m.Files = append(m.Files, f)
	}

	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, body)) + "\n")
	if _, err := update.ParseVerified(body, sig, pub); err != nil {
		return fmt.Errorf("own output failed verification: %w", err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "latest.json"), body, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "latest.json.sig"), sig, 0o644); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "latest.json\nlatest.json.sig")
	return err
}

func fileFor(version, target string) (update.File, error) {
	p := "AgentChime-" + version + "-"
	if target == "macos" {
		return update.File{OS: "macos", Arch: "universal", Name: p + "macos.zip"}, nil
	}
	osName, arch, ok := strings.Cut(target, "-")
	if ok && (arch == "amd64" || arch == "arm64") {
		switch osName {
		case "windows":
			return update.File{OS: "windows", Arch: arch, Name: p + target + ".zip"}, nil
		case "linux":
			return update.File{OS: "linux", Arch: arch, Name: p + target + ".tar.gz"}, nil
		}
	}
	return update.File{}, fmt.Errorf("unknown target %q", target)
}
