package cli

import (
	"context"
	"fmt"
)

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
	fmt.Fprintf(e.Out, "Connected to %s (%s)\n", project.Slug, project.Class)
	return nil
}

// StopCmd implements `repose stop [--no-snapshot]`.
func StopCmd(ctx context.Context, e *Env, projectArg string, snapshot bool) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	opID, err := e.Client.StopProject(ctx, project.ID, snapshot)
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
		fmt.Fprintf(e.Out, "Stopped %s. Snapshot %s (%s). Disk is still billed; `repose destroy` to stop that.\n", p.Slug, snapID, humanBytes(snapBytes))
	} else {
		fmt.Fprintf(e.Out, "Stopped %s. Disk is still billed; `repose destroy` to stop that.\n", p.Slug)
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
	if err := e.Client.DestroyProject(ctx, project.ID); err != nil {
		return err
	}
	until := "30 days from now"
	if project.LastSnapshotAt != nil {
		until = project.LastSnapshotAt.AddDate(0, 0, 30).Format("2006-01-02")
	}
	fmt.Fprintf(e.Out, "Destroyed. Last snapshot kept until %s; `repose snapshots restore <id> --as-new NAME` brings it back.\n", until)
	return nil
}

// ResizeCmd implements the hidden `repose resize` alias for POST /resize
// (07-cli.md §5.6, documented only in features/config.md).
func ResizeCmd(ctx context.Context, e *Env, projectArg string, bytes int64) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	opID, err := e.Client.ResizeProject(ctx, project.ID, bytes)
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
	fmt.Fprintf(e.Out, "Resized %s to %s.\n", project.Slug, humanBytes(bytes))
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
