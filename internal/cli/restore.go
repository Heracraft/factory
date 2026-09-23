package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Restore by name (DECISIONS I-167). `repose destroy` used to end with
// `repose snapshots restore <snapshot id> --project <project id> --as-new
// NAME`; now it ends with `repose restore <slug>`, and the api resolves
// the name, the snapshot and the new project.

// DestroyedProject is one row of GET /projects/destroyed (api.md).
type DestroyedProject struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Slug            string     `json:"slug"`
	Class           string     `json:"class"`
	RemoteURL       string     `json:"remote_url,omitempty"`
	VolumeBytes     int64      `json:"volume_bytes"`
	DestroyedAt     time.Time  `json:"destroyed_at"`
	NameFree        bool       `json:"name_free"`
	RestorableUntil *time.Time `json:"restorable_until"`
	Snapshot        Snapshot   `json:"snapshot"`
}

// RestoreRequest is POST /projects/restore's body.
type RestoreRequest struct {
	Slug       string `json:"slug,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	SnapshotID string `json:"snapshot_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

// RestoreResult is POST /projects/restore's answer.
type RestoreResult struct {
	OpID              string    `json:"op_id"`
	ProjectID         string    `json:"project_id"`
	Name              string    `json:"name"`
	Slug              string    `json:"slug"`
	SnapshotID        string    `json:"snapshot_id"`
	SnapshotCreatedAt time.Time `json:"snapshot_created_at"`
	FromProjectID     string    `json:"from_project_id"`
}

func (c *Client) ListDestroyed(ctx context.Context) ([]DestroyedProject, error) {
	var out []DestroyedProject
	if err := c.get(ctx, "/projects/destroyed", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) RestoreByName(ctx context.Context, req RestoreRequest) (*RestoreResult, error) {
	var r RestoreResult
	if err := c.post(ctx, "/projects/restore", req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// restoreHint is the line a destroy ends with.
func restoreHint(slug string) string { return "repose restore " + slug }

// RestoreCmd implements `repose restore NAME [--as NEW] [--snapshot ID]`.
// NAME is the project's name (or id); it is restored from its newest
// snapshot (or --snapshot) as a new project called NAME, or NEW. When a
// live project holds the name, a terminal is asked for another one
// (askName) and anything else is told to pass --as.
func RestoreCmd(ctx context.Context, e *Env, name, as, snapshotID string, askName func(prompt string) (string, error)) error {
	name = strings.TrimSpace(name)
	if name == "" && snapshotID == "" {
		return exitf(ExitUsage, "Name the project to restore: `repose restore NAME`. `repose projects --destroyed` lists what can be restored.")
	}
	req := RestoreRequest{SnapshotID: snapshotID, Name: as}
	switch {
	case looksLikeUUID(name):
		req.ProjectID = name
	case name != "":
		req.Slug = name
	}
	var res *RestoreResult
	for {
		var err error
		res, err = e.Client.RestoreByName(ctx, req)
		if err == nil {
			break
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			return err
		}
		switch {
		case apiErr.Code == "conflict" && apiErr.Detail["reason"] == "name_taken":
			taken, _ := apiErr.Detail["name"].(string)
			if askName == nil {
				return exitf(ExitUsage, "A project called %s already exists. Restore under another name with `repose restore %s --as NEW-NAME`.", taken, name)
			}
			newName, err := askName(fmt.Sprintf("A project called %s already exists. Name for the restored one (empty to cancel): ", taken))
			if err != nil {
				return err
			}
			if strings.TrimSpace(newName) == "" {
				_, _ = fmt.Fprintln(e.Out, "Nothing restored.")
				return nil
			}
			req.Name = strings.TrimSpace(newName)
			continue
		case apiErr.Code == "not_found":
			return exitf(ExitProjectNotFound, "%s. `repose projects --destroyed` lists what can be restored.", strings.TrimSuffix(humaneMessage(apiErr.Message), "."))
		}
		return err
	}
	pr := e.newProgress()
	defer pr.Fail()
	pr.Phase(fmt.Sprintf("Restoring %s from its snapshot of %s", res.Slug, res.SnapshotCreatedAt.Local().Format("2006-01-02 15:04")), "")
	project := &Project{ID: res.ProjectID, Slug: res.Slug, Name: res.Name}
	op, err := waitOpPhased(ctx, e, project, res.OpID, pr, true)
	if err != nil {
		return err
	}
	pr.Fail()
	if op.State == "error" {
		return e.opFailed("restore", res.Slug, op.Error, fmt.Sprintf("`repose status %s` shows where it stopped; `repose destroy %s` removes it, and the snapshot stays restorable.", res.Slug, res.Slug))
	}
	p, err := e.Client.GetProject(ctx, res.ProjectID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Out, "Restored %s from its snapshot of %s in %s; it is %s (%s). `repose attach %s` to get in.\n",
		p.Slug, res.SnapshotCreatedAt.Local().Format("2006-01-02 15:04"), fmtElapsed(pr.Total()), stateWords(p.State), p.Class, p.Slug)
	return nil
}

// DestroyedCmd implements `repose projects --destroyed`: what can be
// restored, and until when.
func DestroyedCmd(ctx context.Context, e *Env) error {
	list, err := e.Client.ListDestroyed(ctx)
	if err != nil {
		return err
	}
	if e.JSON {
		if list == nil {
			list = []DestroyedProject{}
		}
		return writeJSONOut(e.Out, list)
	}
	if len(list) == 0 {
		_, _ = fmt.Fprintln(e.Out, "Nothing to restore: no project destroyed in the last 30 days still has a snapshot.")
		return nil
	}
	writeDestroyedTable(e.Out, list)
	return nil
}

func writeDestroyedTable(w io.Writer, list []DestroyedProject) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROJECT\tCLASS\tDESTROYED\tSNAPSHOT\tSIZE\tRESTORABLE UNTIL")
	for _, d := range list {
		until := "-"
		if d.RestorableUntil != nil {
			until = d.RestorableUntil.Local().Format("2006-01-02")
		}
		name := d.Slug
		if !d.NameFree {
			name += " (name in use)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", name, d.Class, d.DestroyedAt.Local().Format("2006-01-02 15:04"),
			d.Snapshot.CreatedAt.Local().Format("2006-01-02 15:04"), humanBytes(d.Snapshot.Bytes), until)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintln(w, "`repose restore NAME` brings one back (`--as NEW-NAME` when the name is in use).")
}

// destroyedSlugsForCompletion is what `repose restore <TAB>` offers.
func destroyedSlugsForCompletion(env func() (*Env, error)) []string {
	e, err := env()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	list, err := e.Client.ListDestroyed(ctx)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, d := range list {
		seen[d.Slug] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
