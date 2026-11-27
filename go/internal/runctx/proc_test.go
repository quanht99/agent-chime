package runctx

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func writeProc(t *testing.T, root string, pid, ppid int, argv ...string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	status := "Name:\tx\nPPid:\t" + strconv.Itoa(ppid) + "\nUid:\t1000\n"
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := ""
	if len(argv) > 0 {
		cmd = strings.Join(argv, "\x00") + "\x00"
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmd), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAncestorsFromFakeProc(t *testing.T) {
	root := t.TempDir()
	writeProc(t, root, 30, 20, "/bin/zsh", "-lc", "x")
	writeProc(t, root, 20, 10, "/home/example/.local/bin/codex", "exec", "hi")
	writeProc(t, root, 10, 1, "node", "/a/b.js")
	writeProc(t, root, 1, 0, "/sbin/init")

	got := ancestorsFrom(root, 30, MaxDepth)
	want := [][]string{{"/bin/zsh", "-lc", "x"}, {"/home/example/.local/bin/codex", "exec", "hi"}, {"node", "/a/b.js"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got := ancestorsFrom(root, 30, 2); len(got) != 2 {
		t.Errorf("maxDepth not honoured: %v", got)
	}
}

func TestAncestorsFromSkipsEmptyAndHandlesMissingAndLoops(t *testing.T) {
	root := t.TempDir()
	writeProc(t, root, 5, 4, "a")
	writeProc(t, root, 4, 3) // kernel-thread style: empty cmdline, skipped
	writeProc(t, root, 3, 3, "c")
	got := ancestorsFrom(root, 5, MaxDepth)
	want := [][]string{{"a"}, {"c"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got := ancestorsFrom(root, 99, MaxDepth); len(got) != 0 {
		t.Errorf("missing pid: %v", got)
	}
}
