package cli

import (
	"context"
	"fmt"
	"os"
)

// ConfigShowCmd implements `repose config show [--revisions]`.
func ConfigShowCmd(ctx context.Context, e *Env, projectArg string, revisions bool) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	if revisions {
		revs, err := e.Client.ListRevisions(ctx, project.ID)
		if err != nil {
			return err
		}
		if e.JSON {
			return writeJSONOut(e.Out, revs)
		}
		for _, r := range revs {
			line := fmt.Sprintf("%s\t%s\t%s", r.ID, r.Status, r.CreatedAt.Format("2006-01-02 15:04"))
			if r.Error != "" {
				line += "\t" + r.Error
			}
			_, _ = fmt.Fprintln(e.Out, line)
		}
		return nil
	}
	cfg, err := e.Client.GetConfig(ctx, project.ID)
	if err != nil {
		return err
	}
	if e.JSON {
		return writeJSONOut(e.Out, cfg)
	}
	_, _ = fmt.Fprint(e.Out, cfg.Fragment)
	return nil
}

// applyFragmentAndRender PUTs a fragment, streams the build log, and
// prints the error block on failure (07-cli.md §5.10, §5.8).
func applyFragmentAndRender(ctx context.Context, e *Env, project *Project, fragment, localFragmentPath string) error {
	revisionID, opID, err := e.Client.PutConfigFragment(ctx, project.ID, fragment)
	if err != nil {
		var apiErr *APIError
		if ok := asAPIError(err, &apiErr); ok && apiErr.Code == "invalid" {
			RenderBuildError(e.Out, apiErr.Code, apiErr.Message, localFragmentPath, mustReadFragment(localFragmentPath))
			return silent(ExitBuildFailed)
		}
		return err
	}
	if opID == "" {
		_, _ = fmt.Fprintf(e.Out, "configuration unchanged; %s is still active\n", revisionID)
		return nil
	}
	op, err := waitOp(ctx, e.Client, project.ID, opID, e.Out)
	if err != nil {
		return err
	}
	if op.State == "error" {
		RenderBuildError(e.Out, op.Error.Code, op.Error.Message, localFragmentPath, mustReadFragment(localFragmentPath))
		return silent(ExitBuildFailed)
	}
	_, _ = fmt.Fprintf(e.Out, "Applied revision %s\n", revisionID)
	if revs, err := e.Client.ListRevisions(ctx, project.ID); err == nil {
		for _, r := range revs {
			if r.ID == revisionID && r.RebootRequired {
				_, _ = fmt.Fprintln(e.Out, "This change needs a reboot; run `repose stop && repose start` when the agent is idle.")
			}
		}
	}
	return nil
}

func mustReadFragment(path string) []byte {
	b, _ := os.ReadFile(path)
	return b
}

func asAPIError(err error, target **APIError) bool {
	if ae, ok := err.(*APIError); ok {
		*target = ae
		return true
	}
	return false
}

// ConfigApplyCmd implements `repose config apply [PATH]`.
func ConfigApplyCmd(ctx context.Context, e *Env, projectArg, path string) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	if path == "" {
		path = "./repose.nix"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return exitf(ExitUsage, "reading %s: %v", path, err)
	}
	return applyFragmentAndRender(ctx, e, project, string(b), path)
}

// ConfigEditCmd implements `repose config edit`: fetch, open $EDITOR on a
// temp file, PUT on save. editor is injected for testing.
func ConfigEditCmd(ctx context.Context, e *Env, projectArg string, editor func(path string) error) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	cfg, err := e.Client.GetConfig(ctx, project.ID)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "repose-config-*.nix")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(cfg.Fragment); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := editor(tmp.Name()); err != nil {
		return err
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	return applyFragmentAndRender(ctx, e, project, string(b), tmp.Name())
}
