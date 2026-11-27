// Command e2eserver is the local update server for the end-to-end tests. It serves a folder the
// way GitHub serves a release and listens on 127.0.0.1 only.
//
//	GET /latest/download/latest.json(.sig)   -> <dir>/latest.json(.sig)
//	GET /download/v<version>/<name>          -> <dir>/<name>
//
// Every request adds one line to <dir>/requests. The file <dir>/mode is read on every request:
//
//	ok             serve the files (also when the file is missing or empty)
//	down           503 everything
//	badsig         serve a .sig of other bytes
//	badsum         flip the last byte of archives
//	redirect-evil  302 the manifest to https://evil.example/latest.json
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

func main() {
	dir := flag.String("dir", "", "folder to serve")
	portfile := flag.String("portfile", "", "file to write the port to")
	flag.Parse()
	if *dir == "" || *portfile == "" {
		log.Fatal("e2eserver: -dir and -portfile are required")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	// Write the port to a temp name first, so a reader never sees half of it.
	tmp := *portfile + ".tmp"
	port := fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	if err := os.WriteFile(tmp, []byte(port+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.Rename(tmp, *portfile); err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.Serve(ln, &server{dir: *dir}))
}

type server struct {
	dir string
	mu  sync.Mutex
}

func (s *server) mode() string {
	b, err := os.ReadFile(filepath.Join(s.dir, "mode"))
	if err != nil {
		return "ok"
	}
	if m := strings.TrimSpace(string(b)); m != "" {
		return m
	}
	return "ok"
}

func (s *server) count(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.dir, "requests"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.count(r.Method + " " + r.URL.Path)
	mode := s.mode()
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if mode == "down" {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}

	var name string
	manifest := false
	switch p := r.URL.Path; {
	case p == "/latest/download/latest.json" || p == "/latest/download/latest.json.sig":
		name = path.Base(p)
		manifest = true
	case strings.HasPrefix(p, "/download/v"):
		rest := strings.TrimPrefix(p, "/download/")
		_, file, ok := strings.Cut(rest, "/")
		if !ok || file == "" || file != path.Base(file) {
			http.NotFound(w, r)
			return
		}
		name = file
	default:
		http.NotFound(w, r)
		return
	}
	if manifest && mode == "redirect-evil" {
		http.Redirect(w, r, "https://evil.example/latest.json", http.StatusFound)
		return
	}
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case mode == "badsig" && name == "latest.json.sig":
		data = otherSig(data)
	case mode == "badsum" && !manifest && len(data) > 0:
		data[len(data)-1] ^= 0xff
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

// otherSig returns a well-formed signature of different bytes: the real one with a bit flipped.
func otherSig(sig []byte) []byte {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) == 0 {
		return []byte("AAAA\n")
	}
	raw[0] ^= 1
	return []byte(base64.StdEncoding.EncodeToString(raw) + "\n")
}
