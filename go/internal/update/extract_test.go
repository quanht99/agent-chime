package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type entry struct {
	name string
	body string
	typ  byte // tar type; 0 = regular
	link string
}

func writeZip(t *testing.T, entries []entry) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body))
	}
	zw.Close()
	f.Close()
	return p
}

func writeTar(t *testing.T, entries []entry) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0)}
		if e.typ != 0 {
			h.Typeflag, h.Linkname, h.Size = e.typ, e.link, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	gz.Close()
	f.Close()
	return p
}

var winWant = []string{"AgentChime.exe", "AgentChime-Settings.exe"}
var linWant = []string{"agentchime", "agentchime-settings"}

func TestExtractZipRoot(t *testing.T) {
	a := writeZip(t, []entry{{name: "AgentChime.exe", body: "A"}, {name: "AgentChime-Settings.exe", body: "S"}, {name: "README.txt", body: "r"}})
	dir := t.TempDir()
	got, err := Extract(a, dir, winWant)
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if b, _ := os.ReadFile(got["AgentChime.exe"]); string(b) != "A" {
		t.Fatal("content")
	}
	if _, err := os.Stat(filepath.Join(dir, "README.txt")); err == nil {
		t.Fatal("README written")
	}
	// Windows has no POSIX modes: a writable file always reads back as 0666.
	if st, _ := os.Stat(got["AgentChime.exe"]); runtime.GOOS != "windows" && st.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", st.Mode())
	}
}

func TestExtractTarTopFolder(t *testing.T) {
	a := writeTar(t, []entry{
		{name: "AgentChime-0.9.1-linux-amd64/agentchime", body: "A"},
		{name: "AgentChime-0.9.1-linux-amd64/agentchime-settings", body: "S"},
		{name: "AgentChime-0.9.1-linux-amd64/notes", body: "n"},
	})
	dir := t.TempDir()
	got, err := Extract(a, dir, linWant)
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if b, _ := os.ReadFile(got["agentchime-settings"]); string(b) != "S" {
		t.Fatal("content")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes")); err == nil {
		t.Fatal("notes written")
	}
}

func TestExtractRejectsEscape(t *testing.T) {
	for _, bad := range []string{"../x", "/abs", "a/../../x", `a\..\x`} {
		for _, kind := range []string{"zip", "tar"} {
			t.Run(kind+"/"+bad, func(t *testing.T) {
				root := t.TempDir()
				dir := filepath.Join(root, "out")
				os.Mkdir(dir, 0o755)
				es := []entry{{name: "agentchime", body: "A"}, {name: bad, body: "evil"}, {name: "agentchime-settings", body: "S"}}
				var a string
				if kind == "zip" {
					es[0].name, es[2].name = "AgentChime.exe", "AgentChime-Settings.exe"
					a = writeZip(t, es)
				} else {
					a = writeTar(t, es)
				}
				want := linWant
				if kind == "zip" {
					want = winWant
				}
				if _, err := Extract(a, dir, want); err == nil {
					t.Fatal("expected error")
				}
				var found []string
				filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
					if err == nil && !info.IsDir() {
						found = append(found, p)
					}
					return nil
				})
				if len(found) != 0 {
					t.Fatalf("files exist: %v", found)
				}
			})
		}
	}
}

func TestExtractRejectsSymlink(t *testing.T) {
	a := writeTar(t, []entry{{name: "agentchime", typ: tar.TypeSymlink, link: "/etc/passwd"}, {name: "agentchime-settings", body: "S"}})
	dir := t.TempDir()
	if _, err := Extract(a, dir, linWant); err == nil {
		t.Fatal("expected error")
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Fatalf("left: %v", left)
	}
}

func TestExtractRejectsDuplicate(t *testing.T) {
	a := writeTar(t, []entry{{name: "agentchime", body: "A"}, {name: "X/agentchime", body: "B"}, {name: "agentchime-settings", body: "S"}})
	if _, err := Extract(a, t.TempDir(), linWant); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractMissingWanted(t *testing.T) {
	a := writeTar(t, []entry{{name: "agentchime", body: "A"}})
	_, err := Extract(a, t.TempDir(), linWant)
	if err == nil || !strings.Contains(err.Error(), "agentchime-settings") {
		t.Fatalf("err = %v", err)
	}
}

func TestExtractIgnoresDeepAndSimilarNames(t *testing.T) {
	a := writeTar(t, []entry{{name: "A/B/agentchime", body: "x"}, {name: "agentchime", body: "A"}, {name: "agentchime-settings", body: "S"}})
	got, err := Extract(a, t.TempDir(), linWant)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(got["agentchime"]); string(b) != "A" {
		t.Fatal("wrong entry")
	}
}

func TestCanInstall(t *testing.T) {
	for _, p := range []Phase{Available, CheckFailed, Failed, BadBuild} {
		if !(Status{Phase: p, Current: "0.3.0", Latest: "0.3.1"}).CanInstall() {
			t.Errorf("%s should allow", p)
		}
	}
	for _, p := range []Phase{Downloading, RestartNeeded, Updated} {
		if (Status{Phase: p, Current: "0.3.0", Latest: "0.3.1"}).CanInstall() {
			t.Errorf("%s should block", p)
		}
	}
	if (Status{Phase: Available, Current: "0.3.0", Latest: "0.3.0"}).CanInstall() {
		t.Error("same version")
	}
	if (Status{Phase: Available, Current: "0.3.0"}).CanInstall() {
		t.Error("empty latest")
	}
}
