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
}

// opPollInterval is how often an op (and the project, for the phase
// label) is read while waiting: two cheap GETs (I-154).
const opPollInterval = 500 * time.Millisecond
const opPollTimeout = 20 * time.Minute

// pollDelay is the pause before the next poll of a wait that began at
// started: opPollInterval for the first 10 s, where a create or start
// usually ends, then 1 s, then 2 s after a minute, so a long build does not
// spend the account's api budget that a dashboard tab shares (I-187).
func pollDelay(started time.Time) time.Duration {
	switch el := time.Since(started); {
	case el < 10*time.Second:
		return opPollInterval
	case el < time.Minute:
		return time.Second
	default:
		return 2 * time.Second
	}
}

const sshWaitTimeout = 60 * time.Second
const sshRetryInterval = time.Second

// runRun implements the whole `repose run` sequence, 07-cli.md §5.5.
// attachOnly runs only steps 1 (resolve, no create), 3, 4, 8 — what
// `repose attach` is (§5.5's last paragraph).
func runRun(ctx context.Context, e *Env, opts RunOptions, attachOnly bool) error {
	pr := e.newProgress()
	defer pr.Fail() // clears a spinner line left by an early return

	res, err := resolveProject(ctx, e.Client, e.Dir, e.Cwd, e.resolveArg(opts.ProjectArg), &e.Cache, defaultResolveDeps())
	if err != nil {
		return err
	}

	if !attachOnly && opts.Agent == "" && opts.Prompt != "" && !strings.ContainsAny(strings.TrimSpace(opts.Prompt), " \t\n") {
		if err := refusePromptThatIsASlug(ctx, e, opts.Prompt); err != nil {
			return err
		}
	}

	project := res.Project
	if project == nil {
		if attachOnly {
			return errNoProjectFoundFor(res.Remote, e.Command)
		}
		if res.Remote == "" && opts.Name == "" {
			return errNoRemoteNoName()
		}
		project, err = createProjectForRun(ctx, e, res.Remote, opts, pr)
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
			return notRunningError(project)
		}
	} else if err := ensureRunning(ctx, e, project, pr); err != nil {
		return err
	}

	tz := laptopTZ()
	tzSaved := saveProjectTZ(ctx, e, project, tz)

	pr.Phase("Connecting to "+project.Slug, "")
	target, err := connect(ctx, e, project)
	if err != nil {
		return err
	}
	pr.End()
	_, _ = fmt.Fprintf(e.Out, "Connected to %s (%s)\n", project.Slug, project.Class)

	helper := sessionOptions{Slug: project.Slug, Target: target.Args, TZ: tz, HomeDir: e.HomeDir}
	if root := gitRepoRoot(e.Cwd); root != "" && res.Remote != "" && res.Remote == project.RemoteURL {
		// The git carry needs the project's own checkout: its includeIf
		// rules and identity are what the guest should get, and a run
		// from anywhere else would carry some other repository's.
		helper.RepoDir = root
	}
	if attachOnly {
		// The carry runs beside the attach, never before it (I-195).
		helper.Carry = true
		startSessionHelper(e, helper)
		tzSaved()
		return attachTmux(target, project.Slug, "", tz)
	}

	if !opts.NoSync {
		repoRoot := gitRepoRoot(e.Cwd)
		if repoRoot == "" {
			repoRoot = e.Cwd
		}
		pr.Phase("Syncing", "")
		// Tool logins, the git identity and the carry go between the
		// sync's probe and its apply, in one ssh, so the checkout lands in
		// a guest whose git already knows the user and how to reach the
		// remote (I-150), and the carry costs no round trip (I-195..I-198).
		var copied []string
		var carried *carryOutcome
		envs, err := buildEnvCarry(repoRoot)
		if err != nil {
			e.warn("Could not list your .env files (%s); none were sent.", oneLine(err.Error()))
		}
		summary, err := syncGuest(ctx, target, repoRoot, project.Slug, SyncOptions{
			StashRemote: opts.StashRemote, DiscardRemote: opts.DiscardRemote,
			Exclude: e.Cfg.SyncExclude, NoRemote: project.RemoteURL == "", RemoteURL: project.RemoteURL,
			Env: envs,
			BeforeApply: func(markers map[string]string) error {
				gc, err := buildGitCarry(repoRoot, e.HomeDir)
				if err != nil {
					e.warn("Could not read your git config (%s); the guest keeps its own.", oneLine(err.Error()))
				}
				cc, _ := buildClaudeCarry(e.HomeDir)
				if cc != nil {
					for _, n := range cc.Notes {
						e.warn("%s", n)
					}
				}
				copied, carried, err = syncCredentialsAndCarry(ctx, target, e.HomeDir, repoRoot, credSyncOptions{
					RemoteURL: project.RemoteURL,
					Kept: func(label string) {
						e.warn("Kept the guest's %s login: it is newer than the laptop's.", label)
					},
				}, carryOptions{TZ: tz, Git: gc, Claude: cc, Markers: markers})
				return err
			},
		})
		if err != nil {
			return err
		}
		pr.End()
		_, _ = fmt.Fprintln(e.Out, summary.String())
		for _, w := range summary.Warnings() {
			_, _ = fmt.Fprintln(e.ErrOut, w)
		}
		if len(copied) > 0 {
			_, _ = fmt.Fprintf(e.Out, "Credentials: %s\n", strings.Join(copied, ", "))
		}
		if carried != nil {
			for _, l := range carried.Lines() {
				_, _ = fmt.Fprintln(e.ErrOut, l)
			}
		}
	} else {
		helper.Carry = true
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
		pr.Phase("Starting "+agent, "")
		name, existed, err := windowNameFor(ctx, target, project.Slug, agent)
		if err != nil {
			return stepFailed("list the guest's tmux windows", err, "")
		}
		window = name
		if existed {
			pr.Fail()
			_, _ = fmt.Fprintf(e.ErrOut, "Another %s window is open; two agents share one working tree.\n", agent)
			pr.Phase("Starting "+agent, "")
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
			return stepFailed("start "+agent+" in the guest", err, "")
		}
		pr.End()
		if attachInstead {
			_, _ = fmt.Fprintln(e.Out, "Claude Code is not logged in on this guest yet. Finish the login in the window that opens, then re-run with your prompt.")
		}
	}

	_, _ = fmt.Fprintf(e.ErrOut, "Ready in %s.\n", fmtElapsed(pr.Total()))
	tzSaved()
	if opts.NoAttach {
		return nil
	}
	startSessionHelper(e, helper)
	return attachTmux(target, project.Slug, window, tz)
}

// saveProjectTZ moves the project's stored zone to the laptop's when they
// differ (I-198), so the next start's SetupProject writes the zone this
// command puts in the running guest. It runs beside the connect; the
// returned wait is called before the attach and gives it at most two
// seconds, since a missed update only means the next start writes the
// old zone until the next run fixes it again.
func saveProjectTZ(ctx context.Context, e *Env, project *Project, tz string) (wait func()) {
	if tz == "" || (project.TZ != nil && *project.TZ == tz) {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		z := tz
		_, _ = e.Client.PatchProject(ctx, project.ID, PatchProjectRequest{TZ: &z})
	}()
	return func() {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

// refusePromptThatIsASlug catches `repose run izma`: every other command
// takes the project as its argument (I-155), but `run`'s argument is the
// prompt, and a one-word prompt that is exactly a project's name is far
// more likely a slip than an instruction to an agent.
func refusePromptThatIsASlug(ctx context.Context, e *Env, prompt string) error {
	projects, err := e.Client.ListProjects(ctx)
	if err != nil {
		return nil // the check is a courtesy; the run itself reports a real api failure
	}
	for _, p := range projects {
		if p.Slug == prompt {
			return exitf(ExitUsage, "%q is one of your projects, but `repose run`'s argument is a prompt for the agent. To work on it: `repose run --project %s` (or `repose attach %s`). To send the word itself as a prompt, name the agent: `repose run --agent claude %s`.", prompt, prompt, prompt, prompt)
		}
	}
	return nil
}

// connect is steps 3 and 4: the certificate and config, then the first
// connection (which, with multiplexing, is the one every later ssh in
// this command rides). It warns when the alias does not work from a
// plain terminal and uses the generated config directly in that case.
func connect(ctx context.Context, e *Env, project *Project) (sshTarget, error) {
	me, err := e.Client.GetMe(ctx)
	if err != nil {
		return sshTarget{}, err
	}
	allProjects, err := e.Client.ListProjects(ctx)
	if err != nil {
		return sshTarget{}, err
	}
	params := certParams{Handle: me.Handle, Projects: allProjects, CheckAlias: e.TargetFor == nil}
	cr, err := ensureCert(ctx, e.Client, params, nil)
	if err != nil {
		return sshTarget{}, err
	}
	target := e.target(project.Slug)
	if cr.AliasProblem != "" {
		e.warn("warning: %s\n(repose itself uses ~/.ssh/repose/config directly until then.)", cr.AliasProblem)
		if e.TargetFor == nil {
			if sd, err := sshDir(); err == nil {
				target = sshTarget{Args: []string{"-F", filepath.Join(sd, "config"), project.Slug + ".repose"}}
			}
		}
	}
	err = waitForSSH(ctx, target, certRefusalHandler(func() error {
		params.Force = true
		_, err := ensureCert(ctx, e.Client, params, nil)
		return err
	}))
	if err != nil {
		return sshTarget{}, err
	}
	return target, nil
}

// The gateway's refusals that are about the certificate itself
// (docs/interfaces/ssh-gateway.md, internal/gateway/server.go): a fresh
// certificate can fix these. The others (busy, rate limited, the control
// plane, a guest not accepting yet, a stopped or unknown project) cannot,
// and must not spend the one re-issue.
var (
	certRefusals = []string{
		"certificate revoked", "certificate expired", "certificate not yet valid",
		"certificate not signed by the repose ca", "certificate required", "certificate not valid for this project",
	}
	otherRefusals = []string{
		"gateway busy", "too many authentication attempts", "cannot reach control plane",
		"not accepting connections yet", "is stopped", "no such project", "login name must be",
	}
)

// isCertRefusal reports whether ssh's failure is the gateway refusing the
// certificate: one of its certificate banners, or a bare `Permission
// denied` with none of its other banners (a gateway too old to say).
func isCertRefusal(se *sshError) bool {
	if se.ExitCode != 255 {
		return false
	}
	s := strings.ToLower(se.Stderr)
	for _, m := range certRefusals {
		if strings.Contains(s, m) {
			return true
		}
	}
	for _, m := range otherRefusals {
		if strings.Contains(s, m) {
			return false
		}
	}
	return strings.Contains(s, "permission denied")
}

// certRefusalHandler is connect's answer to a refused connection
// (DECISIONS I-175): the first certificate refusal (revoked, expired, a
// CA rotation, a project the certificate predates) gets one forced
// re-issue and an immediate retry; a certificate refusal after that ends
// the wait at once with what the gateway said, instead of 60 s of retries
// ending in "SSH did not answer". Any other refusal keeps waiting.
func certRefusalHandler(reissue func() error) func(*sshError) (bool, error) {
	reissued := false
	return func(se *sshError) (bool, error) {
		if !isCertRefusal(se) {
			return false, nil
		}
		if !reissued {
			reissued = true
			if err := reissue(); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, exitf(ExitGeneric, "The gateway refused a certificate issued just now (%s). `repose login` (as the account that owns the project) and try again; if it still refuses, an operator may have revoked your certificates.", sshStderrDetail(se.Stderr))
	}
}

// warn prints one warning to stderr.
func (e *Env) warn(format string, args ...any) {
	if e.active != nil {
		_, _ = fmt.Fprintf(e.active, format+"\n", args...)
		return
	}
	_, _ = fmt.Fprintf(e.ErrOut, format+"\n", args...)
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
//
// tz, when known, travels as the session's TZ (sshd's AcceptEnv and the
// gateway pass it), so a base whose tmux takes TZ from the attaching
// client (update-environment) gets the laptop's zone rather than none.
func attachTmux(t sshTarget, slug, window, tz string) error {
	target := slug
	if window != "" {
		target = slug + ":" + window
	}
	extra := []string{"-t"}
	if tz != "" {
		if err := os.Setenv("TZ", tz); err == nil {
			extra = append(extra, "-o", "SendEnv=TZ")
		}
	}
	return execReplaceSSH(t, extra, fmt.Sprintf("tmux attach -t %s", shQuote(target)))
}

// waitForSSH is step 4: `ssh <target> true` until it answers, for up to
// sshWaitTimeout. onRefused is offered each ssh failure and returns true
// when it changed something worth an immediate retry (a re-issued
// certificate).
func waitForSSH(ctx context.Context, t sshTarget, onRefused func(*sshError) (bool, error)) error {
	deadline := time.Now().Add(sshWaitTimeout)
	for {
		err := runSSHOK(ctx, t, "true")
		if err == nil {
			return nil
		}
		var se *sshError
		if errors.As(err, &se) && se.ExitCode == -1 {
			return exitf(ExitGeneric, "Could not run ssh: %v. repose needs the OpenSSH client (`ssh`) on your PATH.", se.Err)
		}
		if se != nil && onRefused != nil {
			retry, err := onRefused(se)
			if err != nil {
				return err
			}
			if retry {
				continue
			}
		}
		if time.Now().After(deadline) {
			detail := ""
			if se != nil {
				detail = sshStderrDetail(se.Stderr)
			}
			msg := "Guest is running but SSH did not answer in 60s. `repose logs --kind console` may show why."
			if detail != "" {
				msg += "\nLast error from ssh: " + detail
			}
			return exitf(ExitGeneric, "%s", msg)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sshRetryInterval):
		}
	}
}

// ensureRunning is step 2: wait out a create or start in flight, start a
// stopped (or errored) guest, and wait for the op, streaming the build
// log when the op carries one. Every phase shows on pr.
func ensureRunning(ctx context.Context, e *Env, project *Project, pr *progress) error {
	p, err := e.Client.GetProject(ctx, project.ID)
	if err != nil {
		return err
	}
	*project = *p
	if project.State == "running" && !guestdDead(project) {
		return nil
	}
	// A project just created (or being started by someone else) has an op
	// in flight; starting it again is the conflict the first real run hit
	// ("recruiting is already starting", DECISIONS I-106). Wait on that op
	// when the api named it, otherwise on the state, then re-read.
	// "building" is the create op's first phase (05 §5.3): the project is
	// in it a moment after POST /projects answers "creating", which is what
	// the first M3 run hit (DECISIONS I-114).
	switch project.State {
	case "creating", "building", "starting", "stopping":
		if project.OpID != "" {
			op, err := waitOpPhased(ctx, e, project, project.OpID, pr, true)
			if err != nil {
				return err
			}
			if op.State == "error" {
				return failedStart(e, project, op, pr)
			}
		} else if err := waitState(ctx, e, project, pr); err != nil {
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
	case "restoring", "destroying", "destroyed":
		return notRunningError(project)
	}

	var sr *StartResult
	if project.State == "running" {
		// guestd stopped answering: the api restarts it (I-157), or, if its
		// newer sample says guestd is back, answers "already running".
		sr, err = e.Client.StartProject(ctx, project.ID)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == "conflict" {
			return nil
		}
	} else {
		err = retryOnOpConflict(ctx, func() error {
			var err error
			sr, err = e.Client.StartProject(ctx, project.ID)
			return err
		})
	}
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
	byState := true
	if sr.Restart {
		pr.Phase(fmt.Sprintf("Restarting %s (its agent stopped answering)", project.Slug), "Restarted "+project.Slug)
		byState = false
	} else {
		pr.Phase("Starting "+project.Slug, "Started "+project.Slug)
	}
	op, err := waitOpPhased(ctx, e, project, sr.OpID, pr, byState)
	if err != nil {
		return err
	}
	if op.State == "error" {
		return failedStart(e, project, op, pr)
	}
	pr.End()
	p, err = e.Client.GetProject(ctx, project.ID)
	if err != nil {
		return err
	}
	*project = *p
	return nil
}

// failedStart reports a create or start op that ended in error: a build's
// own failure is the Nix block and exit 10 (07-cli.md §5.8); anything else
// is one sentence and the next step.
func failedStart(e *Env, project *Project, op *Op, pr *progress) error {
	pr.Fail()
	switch op.Error.Code {
	case "build_failed", "eval_failed", "closure_too_large", "build_timeout":
		RenderBuildError(e.ErrOut, op.Error.Code, op.Error.Message, "repose.nix", nil)
		_, _ = fmt.Fprintf(e.ErrOut, "Fix it with `repose config edit --project %s`.\n", project.Slug)
		return silent(ExitBuildFailed)
	}
	return e.opFailed("start", project.Slug, op.Error, nextAfterFailedStart(project.Slug, op.Error.Code))
}

// opFailed is humanize.go's opFailed plus the host's own wording under -v.
func (e *Env) opFailed(verb, slug string, oe OpError, next string) error {
	err := opFailed(verb, slug, oe, next)
	if e.Verbose && oe.Detail != nil {
		if ee, ok := err.(*exitError); ok {
			ee.msg += fmt.Sprintf("\n(detail: %v)", oe.Detail)
		}
	}
	return err
}

// waitState polls the project until it leaves a transitional state,
// showing each state as a phase.
func waitState(ctx context.Context, e *Env, project *Project, pr *progress) error {
	started := time.Now()
	deadline := started.Add(opPollTimeout)
	last := ""
	for {
		p, err := e.Client.GetProject(ctx, project.ID)
		if err != nil {
			return err
		}
		*project = *p
		switch p.State {
		case "creating", "building", "starting", "stopping":
		default:
			return nil
		}
		if p.State != last {
			last = p.State
			if label, done := phaseForState(p.Slug, p.State); label != "" {
				pr.Phase(label, done)
			}
		}
		if time.Now().After(deadline) {
			return exitf(ExitGeneric, "%s has been %s for %s; `repose status %s` shows where it is.", p.Slug, p.State, opPollTimeout, p.Slug)
		}
		if err := sleepOrDone(ctx, pollDelay(started)); err != nil {
			return err
		}
	}
}

// waitOpPhased waits on an op like waitOp, streaming any build log
// through pr, and (when byState) relabels the phase from the project's
// state on every poll: "Building the environment", then "Booting".
func waitOpPhased(ctx context.Context, e *Env, project *Project, opID string, pr *progress, byState bool) (*Op, error) {
	last := ""
	tick := func() {
		if !byState {
			return
		}
		p, err := e.Client.GetProject(ctx, project.ID)
		if err != nil || p.State == last {
			return
		}
		last = p.State
		if label, done := phaseForState(p.Slug, p.State); label != "" {
			pr.Phase(label, done)
		}
	}
	var out io.Writer = pr
	if pr == nil {
		out = e.ErrOut
	}
	return waitOpWith(ctx, e.Client, project.ID, opID, out, tick)
}

// waitOp polls an op to completion, streaming its build log to out if one
// appears (07-cli.md §5.5 step 2, §5.8).
func waitOp(ctx context.Context, c *Client, projectID, opID string, out io.Writer) (*Op, error) {
	return waitOpWith(ctx, c, projectID, opID, out, nil)
}

func waitOpWith(ctx context.Context, c *Client, projectID, opID string, out io.Writer, tick func()) (*Op, error) {
	seq := 0
	streamDone := false
	streamTries := 0
	started := time.Now()
	deadline := started.Add(opPollTimeout)
	for {
		if tick != nil {
			tick()
		}
		op, err := c.GetOp(ctx, projectID, opID)
		if err != nil {
			return nil, err
		}
		if op.LogURL != "" && !streamDone && streamTries < 5 && op.State != "done" && op.State != "error" {
			streamTries++
			state, lastSeq, err := StreamBuildLog(ctx, c, projectID, opID, out, seq)
			seq = lastSeq
			if err == nil && (state == "done" || state == "error") {
				streamDone = true
				// The op read before the stream has no result yet; the
				// error (code, message, fragment line) is on the op the
				// api wrote when the stream ended, so read it again
				// rather than returning the stale one with its state
				// flipped, which rendered every build failure as
				// "error:" and nothing (I-127).
				if fresh, err := c.GetOp(ctx, projectID, opID); err == nil {
					op = fresh
				}
				if state == "error" && op.State != "error" {
					op.State = "error"
				}
			}
			// A stream cut short (a proxy's idle timeout, a network blip)
			// resumes from the last line it printed on the next poll.
		}
		if op.State == "done" || op.State == "error" {
			return op, nil
		}
		if time.Now().After(deadline) {
			return nil, exitf(ExitGeneric, "The operation is still running after %s; `repose status` shows where it is.", opPollTimeout)
		}
		if err := sleepOrDone(ctx, pollDelay(started)); err != nil {
			return nil, err
		}
	}
}

func createProjectForRun(ctx context.Context, e *Env, remote string, opts RunOptions, pr *progress) (*Project, error) {
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
			// The phase starts once the api has answered, so it names the
			// slug every later line and command uses, not the name as
			// typed ("teksafari.org" is created as teksafari-org); the
			// POST itself takes well under a second (I-191).
			pr.Phase("Creating "+p.Slug, fmt.Sprintf("Created %s (%s)", p.Slug, class))
			dir := ""
			if remote == "" {
				// A --name project with no remote has nothing else to be
				// found by; one with a remote is found by it (I-152).
				dir = dirKey(e.Cwd, defaultResolveDeps())
			}
			rememberProject(&e.Cache, remote, dir, *p)
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
			pr.Fail()
			_, _ = fmt.Fprintf(e.ErrOut, "%q is taken; trying %q\n", name, req.Name)
			continue
		}
		return nil, err
	}
	return nil, exitf(ExitGeneric, "Could not find a free project name after 10 attempts; pass --name NAME.")
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
