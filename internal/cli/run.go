package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunOptions is `repose run`'s flags (07-cli.md §5.1).
type RunOptions struct {
	Prompt        string
	Agent         string
	Size          string
	Name          string
	StashRemote   bool
	DiscardRemote bool
	NoSync        bool
	NoAttach      bool
	ProjectArg    string

	// AskPush is 5.5c's "Commit H is not on origin. Push <branch> now?
	// [Y/n]"; nil means always push (the real CLI wires this to stdin).
	AskPush func(commit, branch string) (bool, error)
}

const opPollInterval = 2 * time.Second
const opPollTimeout = 10 * time.Minute
const sshWaitTimeout = 60 * time.Second

// runRun implements the whole `repose run` sequence, 07-cli.md §5.5.
// attachOnly runs only steps 1 (resolve, no create), 3, 4, 8 — what
// `repose attach` is (§5.5's last paragraph).
func runRun(ctx context.Context, e *Env, opts RunOptions, attachOnly bool) error {
	res, err := resolveProject(ctx, e.Client, e.Dir, e.Cwd, e.resolveArg(opts.ProjectArg), &e.Cache, defaultResolveDeps())
	if err != nil {
		return err
	}

	project := res.Project
	if project == nil {
		if attachOnly {
			return errNoProjectFound(res.Remote)
		}
		if res.Remote == "" && opts.Name == "" {
			return errNoRemoteNoName()
		}
		project, err = createProjectForRun(ctx, e, res.Remote, opts)
		if err != nil {
			return err
		}
	}

	if attachOnly {
		p, err := e.Client.GetProject(ctx, project.ID)
		if err != nil {
			return err
		}
		*project = *p
		if project.State != "running" {
			return exitf(ExitGuestNotRunning, "%s is stopped. Run `repose start`.", project.Slug)
		}
	} else if err := ensureRunning(ctx, e, project); err != nil {
		return err
	}

	me, err := e.Client.GetMe(ctx)
	if err != nil {
		return err
	}
	allProjects, err := e.Client.ListProjects(ctx)
	if err != nil {
		return err
	}
	if _, err := ensureCert(ctx, e.Client, certParams{Handle: me.Handle, Projects: allProjects}, nil); err != nil {
		return err
	}

	target := e.target(project.Slug)
	if err := waitForSSH(ctx, target); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Out, "Connected to %s (%s)\n", project.Slug, project.Class)

	if attachOnly {
		return attachTmux(target, project.Slug, "")
	}

	if !opts.NoSync {
		repoRoot := gitRepoRoot(e.Cwd)
		if repoRoot == "" {
			repoRoot = e.Cwd
		}
		summary, err := syncGuest(ctx, target, repoRoot, project.Slug, SyncOptions{
			StashRemote: opts.StashRemote, DiscardRemote: opts.DiscardRemote,
			Exclude: e.Cfg.SyncExclude, AskPush: opts.AskPush,
		})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(e.Out, summary.String())

		copied, err := syncCredentials(ctx, target, e.HomeDir, repoRoot)
		if err != nil {
			return err
		}
		if len(copied) > 0 {
			_, _ = fmt.Fprintf(e.Out, "Credentials: %s\n", strings.Join(copied, ", "))
		}
	}

	window := ""
	if opts.Prompt != "" {
		agent := opts.Agent
		if agent == "" {
			agent = project.AgentDefault
		}
		if agent == "" {
			agent = e.Cfg.DefaultAgent
		}
		name, existed, err := windowNameFor(ctx, target, project.Slug, agent)
		if err != nil {
			return err
		}
		window = name
		if existed {
			_, _ = fmt.Fprintf(e.ErrOut, "Another %s window is open; two agents share one working tree.\n", agent)
		}

		attachInstead := false
		if agent == "claude" {
			hasSecret, err := hasOAuthSecret(ctx, e.Client, project.ID)
			if err != nil {
				return err
			}
			attachInstead, err = needsClaudeLogin(ctx, target, hasSecret)
			if err != nil {
				return err
			}
		}
		if err := startAgentWindow(ctx, target, project.Slug, name, agent, opts.Prompt, attachInstead); err != nil {
			return err
		}
		if attachInstead {
			_, _ = fmt.Fprintln(e.Out, "Claude Code is not logged in on this guest yet. Finish the login in the window that opens, then re-run with your prompt.")
		}
	}

	if opts.NoAttach {
		return nil
	}
	return attachTmux(target, project.Slug, window)
}

func hasOAuthSecret(ctx context.Context, c *Client, projectID string) (bool, error) {
	secrets, err := c.ListSecrets(ctx, projectID)
	if err != nil {
		return false, err
	}
	for _, s := range secrets {
		if s.Name == "CLAUDE_CODE_OAUTH_TOKEN" {
			return true, nil
		}
	}
	return false, nil
}

// attachTmux is step 8: exec ssh -t <slug>.repose tmux attach [-t
// <slug>:<window>], replacing the CLI process.
func attachTmux(t sshTarget, slug, window string) error {
	target := slug
	if window != "" {
		target = slug + ":" + window
	}
	return execReplaceSSH(t, []string{"-t"}, fmt.Sprintf("tmux attach -t %s", target))
}

func waitForSSH(ctx context.Context, t sshTarget) error {
	deadline := time.Now().Add(sshWaitTimeout)
	for {
		if err := runSSHOK(ctx, t, "true"); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return exitf(ExitGeneric, "Guest is running but SSH did not answer in 60s. `repose logs --kind console` may show why.")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// ensureRunning is step 2: start a stopped/creating guest and wait for the
// op, streaming the build log when the op carries one.
func ensureRunning(ctx context.Context, e *Env, project *Project) error {
	p, err := e.Client.GetProject(ctx, project.ID)
	if err != nil {
		return err
	}
	*project = *p
	if project.State == "running" {
		return nil
	}
	// A project just created (or being started by someone else) has an op
	// in flight; starting it again is the conflict the first real run hit
	// ("recruiting is already starting", DECISIONS I-106). Wait on that op
	// when the api named it, otherwise on the state, then re-read.
	// "building" is the create op's first phase (05 §5.3): the project is
	// in it a moment after POST /projects answers "creating", which is what
	// the first M3 run hit (DECISIONS I-114).
	if project.State == "creating" || project.State == "building" || project.State == "starting" {
		if project.OpID != "" {
			op, err := waitOp(ctx, e.Client, project.ID, project.OpID, e.Out)
			if err != nil {
				return err
			}
			if op.State == "error" {
				return exitf(ExitGeneric, "%s", op.Error)
			}
		} else if err := waitState(ctx, e.Client, project); err != nil {
			return err
		}
		p, err = e.Client.GetProject(ctx, project.ID)
		if err != nil {
			return err
		}
		*project = *p
		if project.State == "running" {
			return nil
		}
	}
	var opID string
	err = retryOnOpConflict(ctx, func() error {
		var err error
		opID, err = e.Client.StartProject(ctx, project.ID)
		return err
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == "payment_required" {
			return exitf(ExitPaymentRequired, "Add a card at https://repose.herakraft.co/billing first.")
		}
		if errors.As(err, &apiErr) && apiErr.Code == "capacity" {
			return exitf(ExitCapacity, "No capacity right now; try again in a few minutes. (We have been alerted.)")
		}
		return err
	}
	op, err := waitOp(ctx, e.Client, project.ID, opID, e.Out)
	if err != nil {
		return err
	}
	if op.State == "error" {
		return exitf(ExitGeneric, "%s", op.Error)
	}
	p, err = e.Client.GetProject(ctx, project.ID)
	if err != nil {
		return err
	}
	*project = *p
	return nil
}

// waitState polls the project until it leaves a transitional state.
func waitState(ctx context.Context, c *Client, project *Project) error {
	for {
		p, err := c.GetProject(ctx, project.ID)
		if err != nil {
			return err
		}
		*project = *p
		if p.State != "creating" && p.State != "building" && p.State != "starting" && p.State != "stopping" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// waitOp polls an op to completion, streaming its build log if one
// appears (07-cli.md §5.5 step 2, §5.8).
func waitOp(ctx context.Context, c *Client, projectID, opID string, out io.Writer) (*Op, error) {
	seq := 0
	streamed := false
	deadline := time.Now().Add(opPollTimeout)
	for {
		op, err := c.GetOp(ctx, projectID, opID)
		if err != nil {
			return nil, err
		}
		if op.LogURL != "" && !streamed {
			streamed = true
			state, lastSeq, err := StreamBuildLog(ctx, c, projectID, opID, out, seq)
			if err == nil {
				seq = lastSeq
				if state == "error" {
					op.State = "error"
				}
			}
		}
		if op.State == "done" || op.State == "error" {
			return op, nil
		}
		if time.Now().After(deadline) {
			return nil, exitf(ExitGeneric, "timed out waiting for the operation to finish")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(opPollInterval):
		}
	}
}

func createProjectForRun(ctx context.Context, e *Env, remote string, opts RunOptions) (*Project, error) {
	name := opts.Name
	if name == "" {
		name = basenameFromRemote(remote, e.Cwd)
	}
	class := opts.Size
	if class == "" {
		class = e.Cfg.DefaultClass
	}
	req := CreateProjectRequest{Name: name, RemoteURL: remote, Class: class, TZ: localTZ()}

	for attempt := 1; attempt <= 10; attempt++ {
		p, err := e.Client.CreateProject(ctx, req)
		if err == nil {
			rememberProject(&e.Cache, remote, e.Cwd, *p)
			if err := e.saveCache(); err != nil {
				return nil, err
			}
			return p, nil
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == "payment_required" {
			return nil, exitf(ExitPaymentRequired, "Add a card at https://repose.herakraft.co/billing first.")
		}
		if errors.As(err, &apiErr) && apiErr.Code == "conflict" {
			req.Name = fmt.Sprintf("%s-%d", name, attempt+1)
			_, _ = fmt.Fprintf(e.ErrOut, "%q is taken; trying %q\n", name, req.Name)
			continue
		}
		return nil, err
	}
	return nil, exitf(ExitGeneric, "could not find a free project name after 10 attempts")
}

func basenameFromRemote(remote, cwd string) string {
	if remote != "" {
		return filepath.Base(remote)
	}
	return filepath.Base(cwd)
}

// localTZ returns the laptop's IANA zone name, or "" when it cannot be
// known, in which case the request omits tz and the api applies its
// default. Go names time.Local "Local" unless TZ is set, and the previous
// fallback sent the abbreviation ("EAT"), which the api rightly refuses as
// not an IANA name (M2 gate, DECISIONS I-104).
func localTZ() string {
	if z := os.Getenv("TZ"); z != "" && z != "Local" {
		if _, err := time.LoadLocation(strings.TrimPrefix(z, ":")); err == nil {
			return strings.TrimPrefix(z, ":")
		}
	}
	if z := time.Local.String(); z != "" && z != "Local" {
		if _, err := time.LoadLocation(z); err == nil {
			return z
		}
	}
	if z := tzFromLocaltime("/etc/localtime"); z != "" {
		return z
	}
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		if z := strings.TrimSpace(string(b)); z != "" {
			if _, err := time.LoadLocation(z); err == nil {
				return z
			}
		}
	}
	return ""
}

// tzFromLocaltime resolves a /etc/localtime symlink to the zone name after
// the zoneinfo directory ("/usr/share/zoneinfo/Europe/Paris" and macOS's
// "/var/db/timezone/zoneinfo/Europe/Paris" both give Europe/Paris).
func tzFromLocaltime(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	target = filepath.Clean(target)
	const marker = "/zoneinfo/"
	i := strings.LastIndex(target, marker)
	if i < 0 {
		return ""
	}
	z := target[i+len(marker):]
	if _, err := time.LoadLocation(z); err != nil {
		return ""
	}
	return z
}
