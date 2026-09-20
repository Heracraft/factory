package nixbuild

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

// AllowedURIs derives the `allowed-uris` for a restricted evaluation from
// the base checkout's flake.lock: exactly the locked inputs, each with its
// narHash, and nothing else. The flake machinery fetches locked inputs
// during evaluation, and restricted mode refuses any URI that is not
// listed; the fragment's own directory is added by the caller. Nix matches
// a listed URI exactly or as a prefix up to a `/`, so
// `github:NixOS/nixpkgs/<rev>?narHash=...` admits that one revision and no
// other, and a fragment cannot fetch at evaluation time through the list.
func AllowedURIs(lockPath string) ([]string, error) {
	b, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("flake.lock: %w", err)
	}
	var lock struct {
		Nodes map[string]struct {
			Locked map[string]any `json:"locked"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(b, &lock); err != nil {
		return nil, fmt.Errorf("flake.lock: %w", err)
	}
	seen := map[string]bool{}
	var out []string
	for _, n := range lock.Nodes {
		u := lockedURI(n.Locked)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	sort.Strings(out)
	return out, nil
}

// escape percent-encodes a query value the way Nix prints one: `=` and
// `+` encoded, `/` kept.
func escape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "%2F", "/")
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

// lockedURI renders one locked node the way Nix prints it when checking
// restricted-mode access. Relative path inputs (the fragment placeholder)
// are inside the flake source and need no entry.
func lockedURI(l map[string]any) string {
	nar := str(l, "narHash")
	switch str(l, "type") {
	case "github", "gitlab", "sourcehut":
		if nar == "" {
			return ""
		}
		return fmt.Sprintf("%s:%s/%s/%s?narHash=%s", str(l, "type"), str(l, "owner"), str(l, "repo"), str(l, "rev"), escape(nar))
	case "git":
		if nar == "" {
			return ""
		}
		q := ""
		if rev := str(l, "rev"); rev != "" {
			q = "rev=" + rev + "&"
		}
		return str(l, "url") + "?" + q + "narHash=" + escape(nar)
	case "tarball", "file":
		if nar == "" {
			return ""
		}
		return str(l, "url") + "?narHash=" + escape(nar)
	}
	return ""
}
