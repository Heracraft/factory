// Package testguest is an in-process SSH server standing in for a repose
// guest, for the CLI's own tests. docs/workstreams/07-cli.md §7 asks for a
// "Docker test fixture test/guest-sshd/ with tmux and git, trusting the
// test CA"; this package covers the same ground (a real sshd endpoint
// running real git and tmux binaries against a scratch home directory)
// without a container, the same trade the gateway-edge workstream made
// ("an in-process SSH server standing in for a guest", 06-gateway-edge.md
// §7, DECISIONS I-49). It authenticates with a plain authorized key rather
// than the CA certificate chain: certificate verification is the
// gateway's contract (docs/interfaces/ssh-gateway.md) and is covered by
// internal/ca/testca's own tests and internal/cli's cert_test.go; what
// this package exercises is what the CLI does once it is talking to a
// guest's shell: git, tar and tmux over a real SSH session.
package testguest

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

// Guest is a running fake guest sshd. Call Close when done.
type Guest struct {
	Addr string // "127.0.0.1:port"
	Home string // the fake guest's $HOME

	listener net.Listener
	config   *ssh.ServerConfig
	sockDir  string
	wg       sync.WaitGroup
	conns    atomic.Int64
}

// New starts a fake guest that accepts authorizedKey and runs every exec
// request as `bash -c <cmd>` with HOME=home (so `~` expansion, git and
// tmux all behave as they do on a real guest) and TMUX_TMPDIR pinned so a
// tmux server started by one exec is found by the next, as it is on a
// real, long-lived guest.
func New(home string, authorizedKey ssh.PublicKey) (*Guest, error) {
	hostKey, err := generateHostKey()
	if err != nil {
		return nil, fmt.Errorf("testguest: host key: %w", err)
	}
	sockDir := filepath.Join(home, ".tmux-sockets")
	if err := os.MkdirAll(sockDir, 0o700); err != nil {
		return nil, err
	}

	g := &Guest{Home: home, sockDir: sockDir}
	g.config = &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if string(key.Marshal()) != string(authorizedKey.Marshal()) {
				return nil, fmt.Errorf("unrecognised key")
			}
			return nil, nil
		},
	}
	g.config.AddHostKey(hostKey)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("testguest: listen: %w", err)
	}
	g.listener = l
	g.Addr = l.Addr().String()

	g.wg.Add(1)
	go g.serve()
	return g, nil
}

// Close stops accepting connections and kills any tmux server the fake
// guest started.
func (g *Guest) Close() {
	_ = g.listener.Close()
	g.wg.Wait()
	killTmux(g.sockDir)
}

// Connections is how many SSH connections have authenticated so far, for
// tests that prove a client multiplexes its sessions over one.
func (g *Guest) Connections() int { return int(g.conns.Load()) }

func (g *Guest) serve() {
	defer g.wg.Done()
	for {
		nc, err := g.listener.Accept()
		if err != nil {
			return // listener closed: the test is over
		}
		go g.handleConn(nc)
	}
}

func (g *Guest) handleConn(nc net.Conn) {
	sc, chans, reqs, err := ssh.NewServerConn(nc, g.config)
	if err != nil {
		return // auth failure or reset; nothing to report from a background goroutine
	}
	g.conns.Add(1)
	defer func() { _ = sc.Close() }()
	go ssh.DiscardRequests(reqs)
	for ch := range chans {
		if ch.ChannelType() != "session" {
			_ = ch.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		channel, requests, err := ch.Accept()
		if err != nil {
			continue
		}
		go g.handleSession(channel, requests)
	}
}

func (g *Guest) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer func() { _ = channel.Close() }()
	for req := range requests {
		switch req.Type {
		case "exec":
			var payload struct{ Command string }
			_ = ssh.Unmarshal(req.Payload, &payload)
			_ = req.Reply(true, nil)
			code := g.run(payload.Command, channel)
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{uint32(code)}))
			return
		case "pty-req", "shell", "env", "window-change":
			_ = req.Reply(true, nil)
		default:
			_ = req.Reply(false, nil)
		}
	}
}

func (g *Guest) run(command string, channel ssh.Channel) int {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = g.Home
	// TMUX (set when the test runner itself runs inside a tmux session)
	// must not leak in: a client that inherits it talks to the outer
	// server instead of the one this fake guest's TMUX_TMPDIR points at.
	// XDG_CONFIG_HOME is the test process's (the "laptop's"); a real
	// guest has none, and git would read the laptop's config through it.
	cmd.Env = append(filterEnv(os.Environ(), "TMUX", "XDG_CONFIG_HOME"),
		"HOME="+g.Home,
		"TMUX_TMPDIR="+g.sockDir,
	)
	cmd.Stdin = channel
	cmd.Stdout = channel
	cmd.Stderr = channel.Stderr()
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		_, _ = fmt.Fprintln(channel.Stderr(), err)
		return 1
	}
	return 0
}

// GenerateClientKey makes an ed25519 keypair for a test to use as the
// authorized identity, writing the private key into dir and returning its
// path and the public key.
func GenerateClientKey(dir string) (privPath string, pub ssh.PublicKey, err error) {
	priv, pubKey, err := generateEd25519()
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(path, priv, 0o600); err != nil {
		return "", nil, err
	}
	return path, pubKey, nil
}

// filterEnv drops every "KEY=..." entry whose key is in drop.
func filterEnv(env []string, drop ...string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		skip := false
		for _, d := range drop {
			if key == d {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, kv)
		}
	}
	return out
}

func killTmux(sockDir string) {
	// tmux places its socket at $TMUX_TMPDIR/tmux-<uid>/default.
	socket := filepath.Join(sockDir, fmt.Sprintf("tmux-%d", os.Getuid()), "default")
	cmd := exec.Command("tmux", "-S", socket, "kill-server")
	_ = cmd.Run() // best effort; nothing to clean up if no server ever started
}
