package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// A base revision is what every host clones and checks out (I-144), so
// `base publish` refuses one that no host could use (DECISIONS I-173):
// 2026.09.21 was published as `3f83664f`, a short sha that did not exist,
// and every create on that base failed until the next publish.

// DefaultBaseRepo is the platform repository hosts clone bases from
// (nix/hosts/host-01.nix `repose.host.baseRepo.url`). REPOSE_BASE_REPO
// or `--repo` overrides it.
const DefaultBaseRepo = "https://github.com/Heracraft/factory.git"

// githubAPI is GitHub's API root; tests point it at a fake.
var githubAPI = "https://api.github.com"

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// errRevNotOnBranch is a revision the repository has but not on the branch.
var errRevNotOnBranch = errors.New("not on the branch")

// errRevUnknown is a revision the repository does not have.
var errRevUnknown = errors.New("no such commit")

// RevChecker says whether sha is on branch of repo: nil when it is,
// errRevNotOnBranch or errRevUnknown when it is not, any other error when
// the question could not be answered.
type RevChecker func(ctx context.Context, repo, branch, sha string) error

// checkBaseRev is base publish's gate: a full lowercase sha, present on
// the branch hosts are meant to build from.
func (e *Env) checkBaseRev(ctx context.Context, rev, repo, branch string) (string, error) {
	sha := strings.ToLower(strings.TrimSpace(rev))
	if !fullSHA.MatchString(sha) {
		return "", fmt.Errorf("%w: --rev %q is not a full commit sha; hosts check out exactly this string, so give all 40 hex characters (`git rev-parse %s` prints it)", ErrUsage, rev, rev)
	}
	check := e.RevCheck
	if check == nil {
		check = githubRevCheck
	}
	switch err := check(ctx, repo, branch, sha); {
	case err == nil:
		return sha, nil
	case errors.Is(err, errRevUnknown):
		return "", fmt.Errorf("%w: %s has no commit %s; push it and publish again (a host would fail every build on this base)", ErrUsage, repo, sha)
	case errors.Is(err, errRevNotOnBranch):
		return "", fmt.Errorf("%w: %s is in %s but not on %s; merge it to %s first (bases are published from %s only)", ErrUsage, sha, repo, branch, branch, branch)
	default:
		return "", fmt.Errorf("could not check %s against %s %s: %v; try again, or pass --unverified-rev if the repository host is down and the sha is certain", sha, repo, branch, err)
	}
}

var githubRepo = regexp.MustCompile(`^(?:https://github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// githubRevCheck asks GitHub's compare API whether sha is branch or an
// ancestor of it ("identical" or "behind"). A token in GITHUB_TOKEN is
// used when set (a private repository, or the 60-an-hour anonymous
// limit); the public platform repository needs none.
func githubRevCheck(ctx context.Context, repo, branch, sha string) error {
	m := githubRepo.FindStringSubmatch(repo)
	if m == nil {
		return fmt.Errorf("only github.com repositories can be checked (%s)", repo)
	}
	u := fmt.Sprintf("%s/repos/%s/%s/compare/%s...%s", githubAPI, m[1], m[2], url.PathEscape(branch), sha)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "repose-admin")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusUnprocessableEntity:
		return errRevUnknown
	default:
		return fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("GitHub's answer: %w", err)
	}
	switch body.Status {
	case "identical", "behind":
		return nil
	case "ahead", "diverged":
		return errRevNotOnBranch
	}
	return fmt.Errorf("GitHub compare status %q", body.Status)
}
