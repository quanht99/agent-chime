package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fixture struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	paths  []string
	agents []string
	cookie bool
	// handler overrides; return true when handled.
	hook   func(w http.ResponseWriter, r *http.Request, n int) bool
	counts map[string]int
	files  map[string][]byte
	sleeps []time.Duration
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, counts: map[string]int{}, files: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.agents = append(f.agents, r.Header.Get("User-Agent"))
		if r.Header.Get("Cookie") != "" {
			f.cookie = true
		}
		f.counts[r.URL.Path]++
		n := f.counts[r.URL.Path]
		hook := f.hook
		body, ok := f.files[r.URL.Path]
		f.mu.Unlock()
		if hook != nil && hook(w, r, n) {
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) client() *Client {
	c := NewClient()
	c.Base = f.srv.URL + "/"
	c.Key = testKey(f.t).Public().(ed25519.PublicKey)
	c.Allow = func(u *url.URL) bool { return u.Scheme == "http" && u.Hostname() == "127.0.0.1" }
	c.Sleep = func(d time.Duration) { f.sleeps = append(f.sleeps, d) }
	return c
}

func (f *fixture) publish(m Manifest) {
	b := mustJSON(f.t, m)
	f.files["/latest/download/latest.json"] = b
	f.files["/latest/download/latest.json.sig"] = sign(f.t, b)
}

func (f *fixture) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.paths)
}

// manifestFor builds a manifest whose linux/amd64 file is data.
func manifestFor(version string, data []byte) (Manifest, File) {
	m := goodManifest(version)
	sum := sha256.Sum256(data)
	for i := range m.Files {
		if m.Files[i].OS == "linux" && m.Files[i].Arch == "amd64" {
			m.Files[i].Size = int64(len(data))
			m.Files[i].SHA256 = hex.EncodeToString(sum[:])
			return m, m.Files[i]
		}
	}
	panic("no linux file")
}

func TestLatestOK(t *testing.T) {
	f := newFixture(t)
	f.publish(goodManifest("0.3.1"))
	m, err := f.client().Latest(context.Background())
	if err != nil || m.Version != "0.3.1" {
		t.Fatalf("got %+v, %v", m, err)
	}
	if f.requests() != 2 || len(f.sleeps) != 0 {
		t.Fatalf("requests=%d sleeps=%v", f.requests(), f.sleeps)
	}
	for _, a := range f.agents {
		if a != "AgentChime" {
			t.Fatalf("user agent %q", a)
		}
	}
	if f.cookie {
		t.Fatal("cookie sent")
	}
}

func TestLatestRetriesOnceThenUnverified(t *testing.T) {
	f := newFixture(t)
	f.publish(goodManifest("0.3.1"))
	f.files["/latest/download/latest.json.sig"] = signWith(testKeyOther(t), f.files["/latest/download/latest.json"])
	_, err := f.client().Latest(context.Background())
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
	if len(f.sleeps) != 1 || f.sleeps[0] != 5*time.Second || f.requests() != 4 {
		t.Fatalf("sleeps=%v requests=%d", f.sleeps, f.requests())
	}
}

func testKeyOther(t *testing.T) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))
}

func TestLatestRecoversOnSecondTry(t *testing.T) {
	f := newFixture(t)
	f.publish(goodManifest("0.3.1"))
	good := f.files["/latest/download/latest.json.sig"]
	f.hook = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if strings.HasSuffix(r.URL.Path, ".sig") && n == 1 {
			w.Write(signWith(testKeyOther(t), f.files["/latest/download/latest.json"]))
			return true
		}
		return false
	}
	_ = good
	m, err := f.client().Latest(context.Background())
	if err != nil || m.Version != "0.3.1" {
		t.Fatalf("got %+v, %v", m, err)
	}
	if len(f.sleeps) != 1 {
		t.Fatalf("sleeps=%v", f.sleeps)
	}
}

func TestLatest404(t *testing.T) {
	f := newFixture(t)
	_, err := f.client().Latest(context.Background())
	if err == nil || errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
	if len(f.sleeps) != 1 || f.requests() != 2 {
		t.Fatalf("sleeps=%v requests=%d", f.sleeps, f.requests())
	}
}

type recorder struct {
	mu    sync.Mutex
	hosts []string
	next  http.RoundTripper
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.hosts = append(r.hosts, req.URL.Host)
	r.mu.Unlock()
	return r.next.RoundTrip(req)
}

func TestClientRefusesBadRedirect(t *testing.T) {
	f := newFixture(t)
	f.hook = func(w http.ResponseWriter, r *http.Request, n int) bool {
		http.Redirect(w, r, "https://evil.example/latest.json", http.StatusFound)
		return true
	}
	c := f.client()
	srvHost := strings.TrimPrefix(f.srv.URL, "http://")
	c.Allow = func(u *url.URL) bool { return u.Host == srvHost || AllowedURL(u) }
	rec := &recorder{next: http.DefaultTransport}
	c.HTTP.Transport = rec
	_, err := c.Latest(context.Background())
	if err == nil || errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
	if len(rec.hosts) == 0 {
		t.Fatal("no requests recorded")
	}
	for _, h := range rec.hosts {
		if h != srvHost {
			t.Fatalf("request left test server: %v", rec.hosts)
		}
	}

	// A redirect to plain http on an allowed host name is refused too.
	f.hook = func(w http.ResponseWriter, r *http.Request, n int) bool {
		http.Redirect(w, r, "http://github.com/x", http.StatusFound)
		return true
	}
	rec.hosts = nil
	if _, err := c.Latest(context.Background()); err == nil || errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
	for _, h := range rec.hosts {
		if h != srvHost {
			t.Fatalf("request left test server: %v", rec.hosts)
		}
	}
}

func TestClientRefusesDisallowedFirstURL(t *testing.T) {
	c := NewClient()
	c.Base = "http://127.0.0.1:1/"
	c.Sleep = func(time.Duration) {}
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("expected refusal")
	}
}

func TestDownloadChecksSum(t *testing.T) {
	f := newFixture(t)
	data := []byte(strings.Repeat("archive", 50))
	m, file := manifestFor("0.3.1", data)
	f.publish(m)
	f.files["/download/v0.3.1/"+file.Name] = data
	dir := t.TempDir()
	path, used, err := f.client().Download(context.Background(), m, "linux", "amd64", dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(data) || used.Version != "0.3.1" || filepath.Dir(path) != dir {
		t.Fatalf("bad result %q %+v", path, used)
	}
}

func TestDownloadRefetchesManifestOnBadSum(t *testing.T) {
	f := newFixture(t)
	data := []byte(strings.Repeat("archive", 50))
	m, file := manifestFor("0.3.1", data)
	f.publish(m)
	f.files["/download/v0.3.1/"+file.Name] = data
	f.hook = func(w http.ResponseWriter, r *http.Request, n int) bool {
		if strings.HasPrefix(r.URL.Path, "/download/") && n == 1 {
			bad := append([]byte(nil), data...)
			bad[0] ^= 1
			w.Write(bad)
			return true
		}
		return false
	}
	dir := t.TempDir()
	path, _, err := f.client().Download(context.Background(), m, "linux", "amd64", dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(data) || len(f.sleeps) != 1 {
		t.Fatalf("sleeps=%v", f.sleeps)
	}
}

func TestDownloadUnverifiedTwice(t *testing.T) {
	f := newFixture(t)
	data := []byte(strings.Repeat("archive", 50))
	m, file := manifestFor("0.3.1", data)
	f.publish(m)
	bad := append([]byte(nil), data...)
	bad[0] ^= 1
	f.files["/download/v0.3.1/"+file.Name] = bad
	dir := t.TempDir()
	_, _, err := f.client().Download(context.Background(), m, "linux", "amd64", dir)
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

type countBody struct {
	io.ReadCloser
	n *atomic.Int64
}

func (c countBody) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.n.Add(int64(n))
	return n, err
}

type countTransport struct {
	next http.RoundTripper
	n    atomic.Int64
}

func (c *countTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := c.next.RoundTrip(req)
	if err == nil && strings.HasPrefix(req.URL.Path, "/download/") {
		resp.Body = countBody{resp.Body, &c.n}
	}
	return resp, err
}

func TestDownloadStopsAtSize(t *testing.T) {
	f := newFixture(t)
	data := []byte(strings.Repeat("archive", 50))
	m, file := manifestFor("0.3.1", data)
	f.publish(m)
	f.files["/download/v0.3.1/"+file.Name] = append(append([]byte(nil), data...), make([]byte, 10)...)
	c := f.client()
	ct := &countTransport{next: http.DefaultTransport}
	c.HTTP.Transport = ct
	dir := t.TempDir()
	_, _, err := c.Download(context.Background(), m, "linux", "amd64", dir)
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("err = %v", err)
	}
	// Two attempts (retry after bad size), each reads at most Size+1.
	if max := 2 * (file.Size + 1); ct.n.Load() > max {
		t.Fatalf("read %d bytes, limit %d", ct.n.Load(), max)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func TestNewClientHasNoWholeRequestTimeout(t *testing.T) {
	c := NewClient()
	if c.HTTP.Timeout != 0 {
		t.Fatalf("Client.Timeout = %v: it would cap the archive body", c.HTTP.Timeout)
	}
	tr, ok := c.HTTP.Transport.(*http.Transport)
	if !ok || tr.ResponseHeaderTimeout <= 0 || tr.TLSHandshakeTimeout <= 0 || tr.Proxy == nil {
		t.Fatalf("transport = %#v", c.HTTP.Transport)
	}
}
