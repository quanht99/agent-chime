//go:build windows

package app

import "golang.org/x/sys/windows"

// shortPath returns the 8.3 form of path.
func shortPath(path string) (string, error) {
	in, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	n, err := windows.GetShortPathName(in, nil, 0)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, n)
	n, err = windows.GetShortPathName(in, &buf[0], uint32(len(buf)))
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}
