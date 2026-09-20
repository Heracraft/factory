package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Env bundles what almost every command needs: config, the API client,
// and the laptop-local caches (docs/interfaces/cli-config.md).
type Env struct {
	Dir     string // ~/.config/repose
	Cfg     Config
	Client  *Client
	Cache   ProjectsCache
	Cwd     string
	HomeDir string
	Out     io.Writer
	ErrOut  io.Writer
	JSON    bool
	Verbose bool

	// TargetFor builds the sshTarget for a project's slug; nil means
	// hostTarget (the real "<slug>.repose" alias). Tests point it at an
	// in-process fake guest instead.
	TargetFor func(slug string) sshTarget

	httpClient *http.Client
}

func (e *Env) target(slug string) sshTarget {
	if e.TargetFor != nil {
		return e.TargetFor(slug)
	}
	return hostTarget(slug)
}

// newEnv builds an Env from the on-disk config and credentials. apiURLFlag
// and projectFlag are the --api-url/--project overrides ("" means unset).
func newEnv(apiURLFlag string, jsonOut, verbose bool) (*Env, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(dir)
	if err != nil {
		return nil, err
	}
	if v := os.Getenv("REPOSE_API_URL"); v != "" {
		cfg.APIURL = v
	}
	if apiURLFlag != "" {
		cfg.APIURL = apiURLFlag
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cache, err := loadProjectsCache(dir)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	var tokens TokenSource
	if creds, ok, err := loadCredentials(dir); err == nil && ok && (creds.RefreshToken != "" || creds.AccessToken != "") {
		tokens = newOIDCTokenSource(dir, httpClient, creds)
	} else {
		tokens = notLoggedInSource{}
	}

	return &Env{
		Dir: dir, Cfg: cfg, Cache: cache, Cwd: cwd, HomeDir: home,
		Client: newClient(cfg.APIURL, tokens), Out: os.Stdout, ErrOut: os.Stderr,
		JSON: jsonOut, Verbose: verbose, httpClient: httpClient,
	}, nil
}

type notLoggedInSource struct{}

func (notLoggedInSource) AccessToken(context.Context, bool) (string, error) {
	return "", errors.New("not logged in")
}

// resolveArg resolves --project/$REPOSE_PROJECT plus cwd, per §5.3.
func (e *Env) resolveArg(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return os.Getenv("REPOSE_PROJECT")
}

func (e *Env) saveCache() error { return saveProjectsCache(e.Dir, e.Cache) }

// exitCodeFor maps any error this package returns to a process exit code
// and prints the right message, per docs/interfaces/cli-config.md's table.
// It is the single place main.go's error handling goes through.
func exitCodeFor(err error, stderr io.Writer) int {
	if err == nil {
		return ExitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.msg != "" {
			fmt.Fprintln(stderr, ee.msg)
		}
		return ee.code
	}
	var notLoggedIn *notLoggedInError
	if errors.As(err, &notLoggedIn) {
		fmt.Fprintln(stderr, "Not logged in. Run `repose login`.")
		return ExitNotLoggedIn
	}
	var unreachable *unreachableError
	if errors.As(err, &unreachable) {
		fmt.Fprintf(stderr, "Cannot reach the api: %v\n", unreachable.cause)
		return ExitGeneric
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "unauthenticated":
			fmt.Fprintln(stderr, "Not logged in. Run `repose login`.")
			return ExitNotLoggedIn
		case "payment_required":
			fmt.Fprintln(stderr, "Add a card at https://repose.herakraft.co/billing first.")
			return ExitPaymentRequired
		case "capacity":
			fmt.Fprintln(stderr, "No capacity right now; try again in a few minutes. (We have been alerted.)")
			return ExitCapacity
		default:
			fmt.Fprintf(stderr, "%s: %s\n", apiErr.Code, apiErr.Message)
			return ExitGeneric
		}
	}
	fmt.Fprintln(stderr, err)
	return ExitGeneric
}
