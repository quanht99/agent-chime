package runctx

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ancestorsFrom walks the parent chain under procRoot starting at pid itself
// (callers pass their parent pid, as the Swift version does), nearest first,
// stopping before pid 1, on a loop, a missing entry, or at maxDepth entries.
// Entries with an empty cmdline are skipped but the walk continues.
func ancestorsFrom(procRoot string, pid, maxDepth int) [][]string {
	var result [][]string
	for steps := 0; pid > 1 && len(result) < maxDepth && steps < 4*maxDepth+64; steps++ {
		dir := filepath.Join(procRoot, strconv.Itoa(pid))
		if args := readCmdline(dir); len(args) > 0 {
			result = append(result, args)
		}
		next, ok := readPPid(dir)
		if !ok || next == pid {
			break
		}
		pid = next
	}
	return result
}

func readCmdline(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "cmdline"))
	if err != nil || len(b) == 0 {
		return nil
	}
	b = bytes.TrimSuffix(b, []byte{0})
	if len(b) == 0 {
		return nil
	}
	return strings.Split(string(b), "\x00")
}

func readPPid(dir string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "PPid:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			return n, err == nil
		}
	}
	return 0, false
}
