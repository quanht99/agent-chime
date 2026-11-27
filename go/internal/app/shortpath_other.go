//go:build !windows

package app

var shortPath func(string) (string, error)
