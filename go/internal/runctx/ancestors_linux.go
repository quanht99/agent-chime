//go:build linux

package runctx

import "os"

func ancestors() [][]string { return ancestorsFrom("/proc", os.Getppid(), MaxDepth) }
