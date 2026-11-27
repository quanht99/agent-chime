//go:build !linux && !windows

package runctx

func ancestors() [][]string { return nil }
