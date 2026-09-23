//go:build windows

package cli

// lockFile is a no-op on Windows, which the CLI does not support.
func lockFile(string) (func(), error) { return func() {}, nil }
