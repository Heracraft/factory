//go:build !darwin

package cli

// No OS keychain on this platform (docs/interfaces/cli-config.md: "on
// macOS the refresh token goes to the keychain ... else credentials.json").
// Everywhere else the refresh token stays in credentials.json mode 0600.

func keychainAvailable() bool           { return false }
func keychainGet(string) (string, bool) { return "", false }
func keychainSet(string, string) error  { return nil }
func keychainDelete(string) error       { return nil }
