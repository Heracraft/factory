// Package project implements the SetupProject request of
// docs/interfaces/vsock-guestd.md: the project directory, project.json, the
// environment file, and the tmux session that every other feature attaches to.
package project

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/guestd/sysdep"
)

// TmuxUnit is the user unit that owns the project's tmux session. It belongs
// to the guest base (02); guestd starts it rather than running tmux itself, so
// the session survives a guestd restart and belongs to dev's tmux server.
const TmuxUnit = "repose-tmux-session.service"

// DefaultTimeout bounds the whole setup.
const DefaultTimeout = 60 * time.Second

// slugRe is the shape of a project slug. It becomes a directory name and a
// tmux session name, so anything that could escape either is rejected.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Info is what /home/dev/.repose/project.json holds
// (docs/interfaces/guest-conventions.md).
type Info struct {
	ProjectID  string `json:"project_id,omitempty"`
	Slug       string `json:"slug"`
	Name       string `json:"name,omitempty"`
	RemoteURL  string `json:"remote_url,omitempty"`
	UserHandle string `json:"user_handle,omitempty"`
	Class      string `json:"class,omitempty"`
	TZ         string `json:"tz,omitempty"`
}

// Handler sets projects up and remembers the current slug for the sampler.
type Handler struct {
	paths sysdep.Paths
	run   sysdep.Runner
	log   *slog.Logger
	uid   int
	gid   int

	mu   sync.RWMutex
	slug string
}

// New builds the handler and loads the slug from a project.json left by an
// earlier boot, so a guestd restart samples the right tmux session before
// hostd has sent SetupProject again.
func New(p sysdep.Paths, run sysdep.Runner, log *slog.Logger) *Handler {
	uid, gid := sysdep.DevIdentity()
	h := &Handler{paths: p, run: run, log: log, uid: uid, gid: gid}
	if info, err := h.load(); err == nil && info.Slug != "" {
		h.slug = info.Slug
	}
	return h
}

// Slug is the current project slug, or "" before the first SetupProject.
func (h *Handler) Slug() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.slug
}

func (h *Handler) load() (Info, error) {
	var info Info
	b, err := os.ReadFile(h.paths.ProjectJSON())
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return info, fmt.Errorf("parse project.json: %w", err)
	}
	return info, nil
}

// Setup is idempotent: every field it writes is rewritten, the directory is
// created if missing, git init runs only on a directory with no .git, and the
// tmux unit is started only when it is not already active.
func (h *Handler) Setup(ctx context.Context, req *guestdv1.SetupProject) error {
	slug := req.GetProjectSlug()
	if !slugRe.MatchString(slug) {
		return sysdep.Invalid("setup project: %q is not a valid project slug", slug)
	}
	if tz := req.GetTz(); strings.ContainsAny(tz, "\n\r") {
		return sysdep.Invalid("setup project: tz contains a newline")
	}
	if lang := req.GetLang(); strings.ContainsAny(lang, "\n\r") {
		return sysdep.Invalid("setup project: lang contains a newline")
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	if err := h.writeProjectJSON(req); err != nil {
		return err
	}
	if err := h.writeEnvFile(req); err != nil {
		return err
	}
	created, err := h.ensureProjectDir(slug)
	if err != nil {
		return err
	}
	initialised, err := h.ensureGitRepo(ctx, slug)
	if err != nil {
		return err
	}
	if err := h.ensureOrigin(ctx, slug, req.GetRemoteUrl()); err != nil {
		return err
	}
	started, err := h.ensureTmux(ctx)
	if err != nil {
		return err
	}

	h.mu.Lock()
	h.slug = slug
	h.mu.Unlock()

	// The slug is a name the user chose, so it stays out of the line; the
	// project id from project.json is the identifier that may be logged.
	id := ""
	if info, err := h.load(); err == nil {
		id = info.ProjectID
	}
	h.log.Info("project set up",
		"event", "setup_project", "project_id", id,
		"dir_created", created, "git_init", initialised, "tmux_started", started)
	return nil
}

// writeProjectJSON prefers the bytes hostd sent (the api's full record) and
// falls back to the fields on the request, so the guest always has a
// project.json even when only the slug is known.
func (h *Handler) writeProjectJSON(req *guestdv1.SetupProject) error {
	body := req.GetProjectJson()
	if len(body) > 0 {
		var probe map[string]any
		if err := json.Unmarshal(body, &probe); err != nil {
			return sysdep.Invalid("setup project: project_json is not JSON: %v", err)
		}
	} else {
		info := Info{
			Slug:      req.GetProjectSlug(),
			Name:      req.GetProjectSlug(),
			RemoteURL: req.GetRemoteUrl(),
			TZ:        req.GetTz(),
		}
		b, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return sysdep.Errf(sysdep.CodeInternal, "encode project.json: %w", err)
		}
		body = append(b, '\n')
	}

	dir := filepath.Dir(h.paths.ProjectJSON())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "create .repose directory: %w", err)
	}
	if err := os.Chown(dir, h.uid, h.gid); err != nil && !os.IsPermission(err) {
		return sysdep.Errf(sysdep.CodeInternal, "chown .repose directory: %w", err)
	}
	if err := sysdep.WriteFileAtomic(h.paths.ProjectJSON(), body, 0o644, h.uid, h.gid); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "write project.json: %w", err)
	}
	return nil
}

// writeEnvFile writes /etc/repose/env, sourced by /etc/profile.d/repose.sh (02).
func (h *Handler) writeEnvFile(req *guestdv1.SetupProject) error {
	var b strings.Builder
	b.WriteString("# Written by guestd at SetupProject; do not edit.\n")
	fmt.Fprintf(&b, "REPOSE_PROJECT=%s\n", req.GetProjectSlug())
	if tz := req.GetTz(); tz != "" {
		fmt.Fprintf(&b, "TZ=%s\n", tz)
	}
	if lang := req.GetLang(); lang != "" {
		fmt.Fprintf(&b, "LANG=%s\n", lang)
	}
	if err := os.MkdirAll(h.paths.EtcDir(), 0o755); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "create /etc/repose: %w", err)
	}
	if err := sysdep.WriteFileAtomic(h.paths.EtcEnv(), []byte(b.String()), 0o644, 0, 0); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "write environment file: %w", err)
	}
	return nil
}

func (h *Handler) ensureProjectDir(slug string) (bool, error) {
	dir := h.paths.ProjectDir(slug)
	if _, err := os.Stat(dir); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, sysdep.Errf(sysdep.CodeInternal, "stat project directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, sysdep.Errf(sysdep.CodeInternal, "create project directory: %w", err)
	}
	if err := os.Chown(dir, h.uid, h.gid); err != nil && !os.IsPermission(err) {
		return false, sysdep.Errf(sysdep.CodeInternal, "chown project directory: %w", err)
	}
	return true, nil
}

// ensureGitRepo runs git init as dev when the directory has no .git, so the
// CLI's fetch-and-checkout at run has somewhere to land.
func (h *Handler) ensureGitRepo(ctx context.Context, slug string) (bool, error) {
	dir := h.paths.ProjectDir(slug)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, sysdep.Errf(sysdep.CodeInternal, "stat git directory: %w", err)
	}
	res, err := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"git", "init", "--quiet"},
		User:      "dev",
		Dir:       dir,
		Env:       sysdep.DevEnv(h.paths, "dev"),
		MaxOutput: 8 << 10,
	})
	if err != nil {
		return false, sysdep.Errf(sysdep.CodeInternal, "git init in the project directory: %w", err)
	}
	if res.ExitCode != 0 {
		return false, sysdep.Errf(sysdep.CodeInternal, "git init in the project directory: exited %d", res.ExitCode)
	}
	return true, nil
}

// ensureOrigin points the repository's `origin` at the project's remote,
// which is what the CLI's sync fetches from at every run
// (docs/features/sync-at-launch.md: "a clone with an origin remote"). The
// api stores the remote normalised as host/path; the guest fetches it over
// SSH through the agent the CLI forwards, so origin is the SSH form. The
// first real run found the directory git-inited with no origin at all
// (DECISIONS I-107). Idempotent: set-url when origin exists.
func (h *Handler) ensureOrigin(ctx context.Context, slug, remote string) error {
	url := originURL(remote)
	if url == "" {
		return nil
	}
	dir := h.paths.ProjectDir(slug)
	run := func(argv ...string) (int, error) {
		res, err := h.run.Run(ctx, sysdep.RunSpec{
			Argv:      argv,
			User:      "dev",
			Dir:       dir,
			Env:       sysdep.DevEnv(h.paths, "dev"),
			MaxOutput: 8 << 10,
		})
		if err != nil {
			return -1, sysdep.Errf(sysdep.CodeInternal, "%s in the project directory: %w", strings.Join(argv[:2], " "), err)
		}
		return res.ExitCode, nil
	}
	if code, err := run("git", "remote", "add", "origin", url); err != nil {
		return err
	} else if code == 0 {
		return nil
	}
	// origin exists: keep it pointed at the project's remote.
	if code, err := run("git", "remote", "set-url", "origin", url); err != nil {
		return err
	} else if code != 0 {
		return sysdep.Errf(sysdep.CodeInternal, "git remote set-url origin: exited %d", code)
	}
	return nil
}

// originURL turns the api's normalised remote ("github.com/owner/repo")
// into the SSH clone URL ("git@github.com:owner/repo.git"). A remote that
// already carries a scheme or an scp-style prefix is used as is; an empty
// remote (a --name project with no git remote) gives "".
func originURL(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	if strings.Contains(remote, "://") || strings.HasPrefix(remote, "git@") {
		return remote
	}
	host, path, ok := strings.Cut(remote, "/")
	if !ok || host == "" || path == "" {
		return ""
	}
	path = strings.TrimSuffix(path, ".git")
	return "git@" + host + ":" + path + ".git"
}

// ensureTmux starts the user unit if it is not already running.
func (h *Handler) ensureTmux(ctx context.Context) (bool, error) {
	active, err := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"systemctl", "--user", "-M", "dev@", "is-active", "--quiet", TmuxUnit},
		MaxOutput: 4 << 10,
		Env:       sysdep.DevEnv(h.paths, "root"),
	})
	if err != nil {
		return false, sysdep.Errf(sysdep.CodeInternal, "query the tmux session unit: %w", err)
	}
	if active.ExitCode == 0 {
		return false, nil
	}
	res, err := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"systemctl", "--user", "-M", "dev@", "start", TmuxUnit},
		MaxOutput: 8 << 10,
		Env:       sysdep.DevEnv(h.paths, "root"),
	})
	if err != nil {
		return false, sysdep.Errf(sysdep.CodeInternal, "start the tmux session unit: %w", err)
	}
	if res.ExitCode != 0 {
		return false, sysdep.Errf(sysdep.CodeInternal, "start the tmux session unit: systemctl exited %d", res.ExitCode)
	}
	return true, nil
}
