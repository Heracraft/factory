package cli

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const opConflictRetryWindow = 10 * time.Second
const opConflictRetryInterval = 250 * time.Millisecond

// retryOnOpConflict retries fn while it fails with a 409 conflict:
// DECISIONS I-70 says a secret or principal push to a running guest
// queues an update_secrets op the caller gets no id for, and
// stop/start/resize/destroy answer 409 conflict "an operation is in
// progress" while it runs, usually well under a second — the only
// conflict these four routes ever produce. Any other error returns
// immediately.
func retryOnOpConflict(ctx context.Context, fn func() error) error {
	deadline := time.Now().Add(opConflictRetryWindow)
	for {
		err := fn()
		var apiErr *APIError
		if err == nil || !errors.As(err, &apiErr) || apiErr.Code != "conflict" {
			return err
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opConflictRetryInterval):
		}
	}
}

// StartCmd implements `repose start` (07-cli.md §5.6): start, wait, print
// the connected line. Does not sync.
func StartCmd(ctx context.Context, e *Env, projectArg string) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	if err := ensureRunning(ctx, e, project); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Out, "Connected to %s (%s)\n", project.Slug, project.Class)
	return nil
}

// StopCmd implements `repose stop [--no-snapshot]`.
func StopCmd(ctx context.Context, e *Env, projectArg string, snapshot bool) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	var opID string
	err = retryOnOpConflict(ctx, func() error {
		var err error
		opID, err = e.Client.StopProject(ctx, project.ID, snapshot)
		return err
	})
	if err != nil {
		return err
	}
	op, err := waitOp(ctx, e.Client, project.ID, opID, e.Out)
	if err != nil {
		return err
	}
	if op.State == "error" {
		return exitf(ExitGeneric, "%s", op.Error)
	}
	p, err := e.Client.GetProject(ctx, project.ID)
	if err != nil {
		return err
	}
	snapID, snapBytes := "", int64(0)
	if snaps, err := e.Client.ListSnapshots(ctx, project.ID); err == nil && len(snaps) > 0 {
		latest := snaps[len(snaps)-1]
		snapID, snapBytes = latest.ID, latest.Bytes
	}
	if snapshot && snapID != "" {
		_, _ = fmt.Fprintf(e.Out, "Stopped %s. Snapshot %s (%s). Disk is still billed; `repose destroy` to stop that.\n", p.Slug, snapID, humanBytes(snapBytes))
	} else {
		_, _ = fmt.Fprintf(e.Out, "Stopped %s. Disk is still billed; `repose destroy` to stop that.\n", p.Slug)
	}
	return nil
}

// DestroyCmd implements `repose destroy [--yes]`.
func DestroyCmd(ctx context.Context, e *Env, projectArg string, yes bool, confirmSlug func(slug string) (string, error)) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	if !yes {
		if confirmSlug == nil {
			return exitf(ExitUsage, "destroying %s needs --yes or a typed confirmation", project.Slug)
		}
		typed, err := confirmSlug(project.Slug)
		if err != nil {
			return err
		}
		if typed != project.Slug {
			return exitf(ExitUsage, "typed name did not match %q; nothing destroyed", project.Slug)
		}
	}
	if err := retryOnOpConflict(ctx, func() error { return e.Client.DestroyProject(ctx, project.ID) }); err != nil {
		return err
	}
	until := "30 days from now"
	if project.LastSnapshotAt != nil {
		until = project.LastSnapshotAt.AddDate(0, 0, 30).Format("2006-01-02")
	}
	_, _ = fmt.Fprintf(e.Out, "Destroyed. Last snapshot kept until %s; `repose snapshots restore <id> --as-new NAME` brings it back.\n", until)
	return nil
}

// ResizeCmd implements the hidden `repose resize` alias for POST /resize
// (07-cli.md §5.6, documented only in features/config.md).
func ResizeCmd(ctx context.Context, e *Env, projectArg string, bytes int64) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	var opID string
	err = retryOnOpConflict(ctx, func() error {
		var err error
		opID, err = e.Client.ResizeProject(ctx, project.ID, bytes)
		return err
	})
	if err != nil {
		return err
	}
	op, err := waitOp(ctx, e.Client, project.ID, opID, e.Out)
	if err != nil {
		return err
	}
	if op.State == "error" {
		return exitf(ExitGeneric, "%s", op.Error)
	}
	_, _ = fmt.Fprintf(e.Out, "Resized %s to %s.\n", project.Slug, humanBytes(bytes))
	return nil
}

// requireProject resolves the current project and reports the exact
// not-found/no-remote errors of §5.3 for every command that is not `run`.
func requireProject(ctx context.Context, e *Env, projectArg string) (*Project, error) {
	res, err := resolveProject(ctx, e.Client, e.Dir, e.Cwd, e.resolveArg(projectArg), &e.Cache, defaultResolveDeps())
	if err != nil {
		return nil, err
	}
	if res.Project == nil {
		return nil, errNoProjectFound(res.Remote)
	}
	return res.Project, nil
}

// requireRunningProject is requireProject plus the "guest not running"
// check that attach-like commands need (07-cli.md §6: exit 5).
func requireRunningProject(ctx context.Context, e *Env, projectArg string) (*Project, error) {
	p, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return nil, err
	}
	if p.State != "running" {
		return nil, exitf(ExitGuestNotRunning, "%s is stopped. Run `repose start`.", p.Slug)
	}
	return p, nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n2 := n / unit; n2 >= unit; n2 /= unit {
		div *= unit
		exp++
	}
	units := "KMGTPE"
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), units[exp])
}
