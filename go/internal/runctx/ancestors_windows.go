//go:build windows

package runctx

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const processCommandLineInformation = 60

type procEntry struct {
	ppid uint32
	exe  string
}

func snapshot() map[uint32]procEntry {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	m := map[uint32]procEntry{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(h, &e); err == nil; err = windows.Process32Next(h, &e) {
		m[e.ProcessID] = procEntry{ppid: e.ParentProcessID, exe: windows.UTF16ToString(e.ExeFile[:])}
	}
	return m
}

// commandLine returns the argv of pid, or nil on any failure.
func commandLine(pid uint32) []string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	size := uint32(4096)
	for try := 0; try < 3; try++ {
		buf := make([]byte, size)
		var need uint32
		err := windows.NtQueryInformationProcess(h, processCommandLineInformation, unsafe.Pointer(&buf[0]), size, &need)
		if err != nil {
			if need > size && need < 1<<20 {
				size = need
				continue
			}
			return nil
		}
		us := (*windows.NTUnicodeString)(unsafe.Pointer(&buf[0]))
		if us.Length == 0 || us.Buffer == nil {
			return nil
		}
		// The string data lives inside buf, right after the header.
		line := windows.UTF16ToString(unsafe.Slice(us.Buffer, us.Length/2))
		args, err := windows.DecomposeCommandLine(line)
		if err != nil || len(args) == 0 {
			return nil
		}
		return args
	}
	return nil
}

func ancestors() (result [][]string) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	procs := snapshot()
	pid := uint32(os.Getppid())
	seen := map[uint32]bool{}
	for pid > 4 && len(result) < MaxDepth && !seen[pid] {
		seen[pid] = true
		p, ok := procs[pid]
		if !ok {
			break
		}
		args := commandLine(pid)
		if len(args) == 0 && p.exe != "" {
			args = []string{p.exe}
		}
		if len(args) > 0 {
			result = append(result, args)
		}
		pid = p.ppid
	}
	return result
}
