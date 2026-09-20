package cli

import (
	"context"
	"fmt"
)

// SnapshotsListCmd implements `repose snapshots list`.
func SnapshotsListCmd(ctx context.Context, e *Env, projectArg string) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	snaps, err := e.Client.ListSnapshots(ctx, project.ID)
	if err != nil {
		return err
	}
	if e.JSON {
		return writeJSONOut(e.Out, snaps)
	}
	for _, s := range snaps {
		fmt.Fprintf(e.Out, "%s\t%s\t%s\t%s\n", s.ID, s.CreatedAt.Format("2006-01-02 15:04"), humanBytes(s.Bytes), s.Reason)
	}
	return nil
}

// SnapshotsCreateCmd implements `repose snapshots create`.
func SnapshotsCreateCmd(ctx context.Context, e *Env, projectArg string) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	opID, err := e.Client.CreateSnapshot(ctx, project.ID)
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
	fmt.Fprintln(e.Out, "Snapshot created.")
	return nil
}

// SnapshotsRestoreCmd implements `repose snapshots restore SNAPSHOT_ID
// [--as-new NAME]`. Without --as-new it requires the project stopped and
// asks for confirmation (07-cli.md §5.10).
func SnapshotsRestoreCmd(ctx context.Context, e *Env, projectArg, snapshotID, asNew string, confirm func() (bool, error)) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	if asNew == "" {
		if project.State != "stopped" {
			return exitf(ExitGuestNotRunning, "%s must be stopped before restoring in place; `repose stop` first, or use --as-new NAME.", project.Slug)
		}
		if confirm != nil {
			ok, err := confirm()
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(e.Out, "Not restored.")
				return nil
			}
		}
	}
	opID, err := e.Client.RestoreSnapshot(ctx, project.ID, snapshotID, asNew)
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
	fmt.Fprintln(e.Out, "Restored.")
	return nil
}
