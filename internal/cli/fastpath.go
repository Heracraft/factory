package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The connect fast path (DECISIONS I-223). connect's slow path reads the
// account and every project from the api, reuses or re-issues the
// certificate, rewrites ~/.ssh/repose and proves the connection with
// `ssh <slug>.repose true`: two api round trips and one ssh round trip
// before the command's first real ssh, on every run and attach, although
// on most of them nothing changed since the last. When the files on disk
// already cover the project, none of the api calls can change anything,
// and when the multiplexed connection of an earlier command is still up
// (ControlPersist), it is the proof: the gateway ends a client connection
// as soon as its guest connection ends, so a live master means a guest
// that answered on it.

// noFastPath turns the fast path off (REPOSE_NO_FASTPATH=1), for
// measuring it against the slow one and as a way out if it misjudges.
func noFastPath() bool { return os.Getenv("REPOSE_NO_FASTPATH") == "1" }

// sshFilesCover reports whether ~/.ssh/repose (sd) already lets the CLI
// reach p: a certificate for the CLI's key with p's id among its
// principals and certReuseMargin of validity left, the known_hosts file,
// and a Host block for p's slug in the generated config. handle is the
// account handle the block names.
func sshFilesCover(sd string, p *Project, now time.Time) (handle string, ok bool) {
	if p == nil || p.ID == "" || p.Slug == "" {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(sd, "known_hosts")); err != nil {
		return "", false
	}
	pub, err := ensureReposeKey()
	if err != nil {
		return "", false
	}
	cert := parseCertFile(filepath.Join(sd, reposeKeyName+"-cert.pub"))
	if !certIsFor(cert, pub) || !certUsableFor(cert, []string{p.ID}, now, certReuseMargin) {
		return "", false
	}
	cfg, err := os.ReadFile(filepath.Join(sd, "config"))
	if err != nil {
		return "", false
	}
	return hostBlockHandle(string(cfg), p.Slug)
}

// hostBlockHandle finds `Host <slug>.repose` in a config renderSSHConfig
// wrote and returns the handle from its `User <slug>.<handle>` line.
func hostBlockHandle(cfg, slug string) (string, bool) {
	in := false
	for _, l := range strings.Split(cfg, "\n") {
		t := strings.TrimSpace(l)
		if k, v, ok := strings.Cut(t, " "); ok && strings.EqualFold(k, "Host") {
			in = v == slug+".repose"
			continue
		}
		if !in {
			continue
		}
		if k, v, ok := strings.Cut(t, " "); ok && strings.EqualFold(k, "User") {
			if h, ok := strings.CutPrefix(v, slug+"."); ok && h != "" {
				return h, true
			}
			return "", false
		}
	}
	return "", false
}

// masterAlive reports whether a ControlPersist master for t is up
// (`ssh -O check`, local only: it asks the master's socket, not the
// network).
func masterAlive(ctx context.Context, t sshTarget) bool {
	if goos() == "windows" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	args := append([]string{"-O", "check"}, t.Args...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.WaitDelay = time.Second
	return cmd.Run() == nil
}

// connectFast is connect when ~/.ssh/repose already covers the project:
// no api call, and no ssh at all when a master is up. ok is false when
// the slow path must run (the files do not cover the project, the alias
// does not resolve and the slow path's warning is due, tests, Windows).
func connectFast(ctx context.Context, e *Env, project *Project) (target sshTarget, ok bool, err error) {
	if e.TargetFor != nil || noFastPath() {
		return sshTarget{}, false, nil
	}
	if t, ok := e.early.connected(ctx, e, project); ok {
		return t, true, nil
	}
	sd, err := sshDir()
	if err != nil {
		return sshTarget{}, false, nil
	}
	handle, covered := sshFilesCover(sd, project, time.Now())
	if !covered {
		return sshTarget{}, false, nil
	}
	if resolves, _ := aliasResolves(project.Slug, handle); !resolves {
		return sshTarget{}, false, nil
	}
	target = e.target(project.Slug)
	if masterAlive(ctx, target) {
		timingf("connect: files cover the project, ssh master up")
		return target, true, nil
	}
	timingf("connect: files cover the project, no ssh master")
	err = waitForSSH(ctx, target, certRefusalHandler(func() error {
		// A refusal of the certificate on disk (revoked, a rotated CA):
		// the slow path's forced re-issue, which needs the account.
		me, err := e.Client.GetMe(ctx)
		if err != nil {
			return err
		}
		projects, err := e.Client.ListProjects(ctx)
		if err != nil {
			return err
		}
		_, err = ensureCert(ctx, e.Client, certParams{Handle: me.Handle, Projects: projects, Force: true, CheckAlias: true}, nil)
		return err
	}))
	if err != nil {
		return sshTarget{}, true, err
	}
	return target, true, nil
}

// cachedGuess is the project this command will most likely resolve to,
// from the projects cache alone (no api call): the explicit PROJECT when
// the cache knows its id, else the cached project for this checkout's
// remote, unless the directory is pinned to another one. nil when the
// cache cannot say.
func cachedGuess(e *Env, explicit string, deps resolveDeps) *Project {
	if explicit != "" {
		for remote, c := range e.Cache.ByRemote {
			if c.Slug == explicit || c.ProjectID == explicit {
				return &Project{ID: c.ProjectID, Slug: c.Slug, RemoteURL: remote}
			}
		}
		return nil
	}
	remote := deps.RemoteFor(e.Cwd)
	if remote == "" {
		return nil
	}
	c, ok := e.Cache.ByRemote[remote]
	if !ok || c.ProjectID == "" || c.Slug == "" {
		return nil
	}
	for _, k := range uniqueStrings(dirKey(e.Cwd, deps), e.Cwd) {
		if id, ok := e.Cache.ByDir[k]; ok && id != c.ProjectID {
			return nil
		}
	}
	return &Project{ID: c.ProjectID, Slug: c.Slug, RemoteURL: remote}
}

// warmTarget returns the ssh target for guess when the files on disk
// cover it (covered), and whether a master for it is up (master): the
// guest answered on a connection that is still open.
func warmTarget(ctx context.Context, e *Env, guess *Project) (t sshTarget, covered, master bool) {
	if guess == nil || e.TargetFor != nil || noFastPath() {
		return sshTarget{}, false, false
	}
	sd, err := sshDir()
	if err != nil {
		return sshTarget{}, false, false
	}
	handle, covered := sshFilesCover(sd, guess, time.Now())
	if !covered {
		return sshTarget{}, false, false
	}
	if resolves, _ := aliasResolves(guess.Slug, handle); !resolves {
		return sshTarget{}, false, false
	}
	t = e.target(guess.Slug)
	return t, true, masterAlive(ctx, t)
}

// attachFast is `repose attach` with no api call (I-223): the cache names
// the project and a live master proves its guest is running, which is
// everything the attach needs. done is false when the full path must
// run; it then has done nothing.
func attachFast(ctx context.Context, e *Env, explicit string) (done bool, err error) {
	deps := defaultResolveDeps()
	guess := cachedGuess(e, explicit, deps)
	target, _, master := warmTarget(ctx, e, guess)
	if !master {
		return false, nil
	}
	timingf("attach: cached project, ssh master up; no api call")
	_, _ = fmt.Fprintf(e.Out, "Connected to %s\n", guess.Slug)
	tz := laptopTZ()
	helper := sessionOptions{Slug: guess.Slug, Target: target.Args, TZ: tz, HomeDir: e.HomeDir, Forward: os.Getenv(forwardEnvOff) != "1", Carry: true}
	if root := gitRepoRoot(e.Cwd); root != "" && explicit == "" {
		// Guessed from this checkout's remote: the checkout is the
		// project's own, whose git config the carry takes.
		helper.RepoDir = root
	}
	startSessionHelper(e, helper)
	return true, attachTmux(target, guess.Slug, "", tz)
}

// earlyProbe is the sync's probe, started before the api has answered
// (I-223): when the cache names the project and ~/.ssh/repose covers it,
// the probe (a read of the guest's checkout, and the creation of an
// empty one, which the sync would do anyway) runs beside resolve's read
// of the project instead of after it. With no master up (cold), the
// probe's own connection becomes the master, so the ssh handshake also
// overlaps the api call. The result is used only when the project
// resolves to the guessed one and was running all along; settle decides.
type earlyProbe struct {
	id, slug string
	target   sshTarget
	cold     bool // no master was up: the probe made the connection
	done     chan struct{}
	out      []byte
	err      error
	usable   bool // set by settle
}

func startEarlyProbe(ctx context.Context, e *Env, opts RunOptions) *earlyProbe {
	if opts.NoSync {
		return nil
	}
	guess := cachedGuess(e, e.resolveArg(opts.ProjectArg), defaultResolveDeps())
	target, covered, master := warmTarget(ctx, e, guess)
	if !covered {
		return nil
	}
	if master {
		timingf("run: cached project, ssh master up; probe started beside the api")
	} else {
		timingf("run: cached project, no ssh master; probe (and master) started beside the api")
	}
	ep := &earlyProbe{id: guess.ID, slug: guess.Slug, target: target, cold: !master, done: make(chan struct{})}
	go func() {
		defer close(ep.done)
		ep.out, ep.err = runSSH(ctx, target, probeScript(guess.Slug), nil)
	}()
	return ep
}

// settle decides, once the api has answered, whether the probe stands in
// for the sync's own: the project is the guessed one and its guest was
// running before this command (a start makes a new guest). A cold probe
// that will not be used is waited for and its master closed: against a
// stopped guest, the gateway's refusal would otherwise keep a master
// that leads nowhere for the next ssh of this command to ride.
func (ep *earlyProbe) settle(ctx context.Context, e *Env, p *Project, wasRunning bool) {
	if ep == nil {
		return
	}
	ep.usable = p != nil && p.ID == ep.id && wasRunning
	if !ep.usable && ep.cold {
		<-ep.done
		closeMaster(ctx, e, ep.slug)
	}
}

// forProject is the probe's result as SyncOptions.Probe, or nil when
// settle found it cannot stand in for the sync's own.
func (ep *earlyProbe) forProject() func() ([]byte, error) {
	if ep == nil || !ep.usable {
		return nil
	}
	return func() ([]byte, error) {
		<-ep.done
		return ep.out, ep.err
	}
}

// connected is connectFast's use of a cold probe for project: once it
// has answered, its connection is the command's master and no `ssh true`
// is needed. ok is false when there was no such probe or it failed; a
// failed one's master is closed, and connect proves the connection the
// usual way.
func (ep *earlyProbe) connected(ctx context.Context, e *Env, project *Project) (sshTarget, bool) {
	if ep == nil || !ep.usable || !ep.cold || project == nil || project.ID != ep.id {
		return sshTarget{}, false
	}
	<-ep.done
	if ep.err != nil {
		closeMaster(ctx, e, ep.slug)
		return sshTarget{}, false
	}
	timingf("connect: the early probe's connection is the master")
	return ep.target, true
}
