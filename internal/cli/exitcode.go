package cli

import "fmt"

// Exit codes, docs/interfaces/cli-config.md "Exit codes".
const (
	ExitOK              = 0
	ExitGeneric         = 1
	ExitUsage           = 2
	ExitNotLoggedIn     = 3
	ExitProjectNotFound = 4
	ExitGuestNotRunning = 5
	ExitDirtyRemoteTree = 6
	ExitPaymentRequired = 7
	ExitCapacity        = 8
	ExitBuildFailed     = 10
)

// exitError carries a message already printed (or to be printed) and the
// exit code main should return. Commands return this instead of calling
// os.Exit directly so tests can assert on it.
type exitError struct {
	code int
	msg  string // empty when the command already printed its own message
}

func (e *exitError) Error() string {
	if e.msg == "" {
		return "exit"
	}
	return e.msg
}

func exitf(code int, format string, args ...any) *exitError {
	return &exitError{code: code, msg: fmt.Sprintf(format, args...)}
}

// silent exits with code and no further message (the command already
// printed one, e.g. streamed build log output).
func silent(code int) *exitError { return &exitError{code: code} }
