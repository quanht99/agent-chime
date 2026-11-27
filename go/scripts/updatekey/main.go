// Command updatekey makes the release signing key. The owner runs it once,
// outside the repository, and keeps the seed file private.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "updatekey:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("updatekey", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "", "file to write the private seed to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("-out is required")
	}
	abs, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err == nil {
		return errors.New("output file already exists")
	}
	// Walk the real folder: a symlink can lead into a git tree the lexical path doesn't show.
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return err
	}
	for dir := parent; ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return errors.New("output folder is inside a git repository")
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(base64.StdEncoding.EncodeToString(priv.Seed()) + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(abs)
		return werr
	}
	_, err = fmt.Fprintf(stdout, "public key: %s\n", hex.EncodeToString(pub))
	return err
}
