package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	userAgent    = "AgentChime"
	maxRedirects = 10
	retryDelay   = 5 * time.Second
)

// Client fetches the signed manifest and the archive it describes.
type Client struct {
	Base  string // ends in "/"
	HTTP  *http.Client
	Key   ed25519.PublicKey
	Allow func(*url.URL) bool
	Sleep func(time.Duration) // time.Sleep; tests pass a no-op
}

// NewClient returns a client for this build's base URL and key.
func NewClient() *Client {
	c := &Client{Base: Base(), Key: Key(), Allow: Allowed, Sleep: time.Sleep}
	// No Client.Timeout: it would cap reading the whole archive body. Each request's context sets
	// the overall limit; the transport only bounds connecting and waiting for the first byte.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 30 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	c.HTTP = &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("update: too many redirects")
			}
			if !c.Allow(req.URL) {
				return fmt.Errorf("update: redirect to %s refused", req.URL.Redacted())
			}
			return nil
		},
	}
	return c
}

// get issues one GET with only the User-Agent header and returns the open body.
func (c *Client) get(ctx context.Context, raw string) (*http.Response, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if c.Allow == nil || !c.Allow(u) {
		return nil, fmt.Errorf("update: url %s not allowed", u.Redacted())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// getLimited reads at most max bytes; more than max is ErrUnverified.
func (c *Client) getLimited(ctx context.Context, raw string, max int64) ([]byte, error) {
	resp, err := c.get(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, ErrUnverified
	}
	return b, nil
}

func (c *Client) latestOnce(ctx context.Context) (Manifest, error) {
	body, err := c.getLimited(ctx, c.Base+"latest/download/latest.json", MaxManifest)
	if err != nil {
		return Manifest{}, err
	}
	sig, err := c.getLimited(ctx, c.Base+"latest/download/latest.json.sig", MaxSig)
	if err != nil {
		return Manifest{}, err
	}
	return ParseVerified(body, sig, c.Key)
}

// Latest fetches and verifies the manifest, retrying once after 5 s.
func (c *Client) Latest(ctx context.Context) (Manifest, error) {
	m, err := c.latestOnce(ctx)
	if err == nil {
		return m, nil
	}
	if ctx.Err() != nil {
		return Manifest{}, fmt.Errorf("check: %w", ctx.Err())
	}
	c.Sleep(retryDelay)
	m, err = c.latestOnce(ctx)
	if err == nil {
		return m, nil
	}
	if errors.Is(err, ErrUnverified) || errors.Is(err, ErrBadManifest) {
		return Manifest{}, ErrUnverified
	}
	return Manifest{}, fmt.Errorf("check: %w", err)
}

// Download fetches the archive for goos/goarch into dir and checks size and
// SHA-256. On a mismatch it re-fetches the manifest once and tries again. It
// returns the manifest whose file it kept. No partial file is left on error.
func (c *Client) Download(ctx context.Context, m Manifest, goos, goarch, dir string) (string, Manifest, error) {
	path, err := c.downloadOne(ctx, m, goos, goarch, dir)
	if err == nil {
		return path, m, nil
	}
	if !errors.Is(err, ErrUnverified) {
		return "", Manifest{}, err
	}
	c.Sleep(retryDelay)
	m2, err := c.Latest(ctx)
	if err != nil {
		return "", Manifest{}, err
	}
	path, err = c.downloadOne(ctx, m2, goos, goarch, dir)
	if err != nil {
		return "", Manifest{}, err
	}
	return path, m2, nil
}

func (c *Client) downloadOne(ctx context.Context, m Manifest, goos, goarch, dir string) (_ string, err error) {
	f, err := m.FileFor(goos, goarch)
	if err != nil {
		return "", err
	}
	if f.Name != filepath.Base(f.Name) || strings.ContainsAny(f.Name, `/\`) || f.Name == "." || f.Name == ".." {
		return "", ErrUnverified
	}
	if f.Size <= 0 || f.Size > MaxArchive {
		return "", ErrUnverified
	}
	resp, err := c.get(ctx, c.Base+"download/v"+m.Version+"/"+f.Name)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, f.Name)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, f.Size+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	if n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return "", ErrUnverified
	}
	return path, nil
}
