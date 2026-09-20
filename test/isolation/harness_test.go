package isolation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const envPrefix = "REPOSE_ISOLATION_"

func env(name string) string { return strings.TrimSpace(os.Getenv(envPrefix + name)) }

// need skips the test unless every named variable is set.
func need(t *testing.T, names ...string) {
	t.Helper()
	var missing []string
	for _, n := range names {
		if env(n) == "" {
			missing = append(missing, envPrefix+n)
		}
	}
	if len(missing) > 0 {
		t.Skipf("needs a real host: set %s", strings.Join(missing, ", "))
	}
	t.Logf("host=%s date=%s", env("HOST_ID"), time.Now().UTC().Format(time.RFC3339))
}

// shellQuote single-quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// result is what a remote script left behind.
type result struct {
	out  string
	err  string
	code int
}

func (r result) String() string {
	return fmt.Sprintf("exit %d\nstdout: %s\nstderr: %s", r.code, strings.TrimSpace(r.out), strings.TrimSpace(r.err))
}

// run executes script through the prefix in the named environment variable
// (EXEC_A, EXEC_B or HOST_EXEC). The script is passed as one quoted
// argument, so `sh -c` style prefixes and ssh prefixes both work.
func run(t *testing.T, where string, timeout time.Duration, script string) result {
	t.Helper()
	prefix := env(where)
	if prefix == "" {
		t.Skipf("set %s%s", envPrefix, where)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", prefix+" "+shellQuote(script))
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	r := result{out: out.String(), err: errb.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		r.code = ee.ExitCode()
	} else if err != nil {
		r.code = -1
		r.err += "\n" + err.Error()
	}
	return r
}

// inA, inB and onHost are the three places a script can run.
func inA(t *testing.T, script string) result    { return run(t, "EXEC_A", 2*time.Minute, script) }
func inB(t *testing.T, script string) result    { return run(t, "EXEC_B", 2*time.Minute, script) }
func onHost(t *testing.T, script string) result { return run(t, "HOST_EXEC", 2*time.Minute, script) }

func mustSucceed(t *testing.T, r result, what string) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("%s should succeed:\n%s", what, r)
	}
}

func mustFail(t *testing.T, r result, what string) {
	t.Helper()
	if r.code == 0 {
		t.Fatalf("%s should have been refused, but succeeded:\n%s", what, r)
	}
	t.Logf("%s refused as expected: exit %d", what, r.code)
}

// repoRoot walks up from this file to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above " + file)
		}
		dir = parent
	}
}
