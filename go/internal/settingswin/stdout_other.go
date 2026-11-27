//go:build !unix

package settingswin

// DetachStdout is a no-op where a closed pipe cannot raise SIGPIPE.
func DetachStdout() error { return nil }
