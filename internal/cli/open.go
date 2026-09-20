package cli

import (
	"context"
	"fmt"
	"net"
	"strconv"
)

// OpenPortCmd implements `repose open PORT [--local-port N] [--no-browser]`
// (07-cli.md §5.9). It blocks in the foreground running the SSH forward,
// like plain `ssh -N -L`; callers that want it backgrounded run it in a
// goroutine.
func OpenPortCmd(ctx context.Context, e *Env, projectArg string, port int, localPort int, noBrowser bool) error {
	project, err := requireRunningProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	target := e.target(project.Slug)
	if err := waitForSSH(ctx, target); err != nil {
		return err
	}
	if localPort == 0 {
		localPort = port
	}
	if !portFree(localPort) {
		freed, err := freePort()
		if err != nil {
			return err
		}
		fmt.Fprintf(e.ErrOut, "port %d is taken; forwarding to %d instead\n", localPort, freed)
		localPort = freed
	}
	url := fmt.Sprintf("http://localhost:%d", localPort)
	fmt.Fprintf(e.Out, "%s → %s:%d (Ctrl-C to stop)\n", url, project.Slug, port)
	if !noBrowser {
		_ = openBrowser(url)
	}
	forward := fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", localPort, port)
	return execReplaceSSH(target, []string{"-N", "-L", forward}, "")
}

// OpenDesktopCmd implements `repose open --desktop`.
func OpenDesktopCmd(ctx context.Context, e *Env, projectArg string, noBrowser bool) error {
	project, err := requireRunningProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	target := e.target(project.Slug)
	if err := waitForSSH(ctx, target); err != nil {
		return err
	}
	if _, err := runSSH(ctx, target, "systemctl --user start repose-desktop", nil); err != nil {
		return err
	}
	url := "http://localhost:6080/vnc.html?autoconnect=1"
	fmt.Fprintf(e.Out, "%s (Ctrl-C stops the forward; the desktop keeps running)\n", url)
	if !noBrowser {
		_ = openBrowser(url)
	}
	return execReplaceSSH(target, []string{"-N", "-L", "127.0.0.1:6080:127.0.0.1:6080"}, "")
}

func portFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portStr)
}
