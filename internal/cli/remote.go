package cli

import "strings"

// normalizeRemote implements docs/interfaces/cli-config.md's rule: strip
// scheme and git@, replace the ':' after host with '/', strip a trailing
// .git, lowercase the host. "git@github.com:A/B.git" and
// "https://github.com/a/b" both become "github.com/a/b".
func normalizeRemote(remote string) string {
	r := strings.TrimSpace(remote)
	if i := strings.Index(r, "://"); i >= 0 {
		r = r[i+3:]
	}
	r = strings.TrimPrefix(r, "git@")
	r = strings.TrimSuffix(r, "/")
	r = strings.TrimSuffix(r, ".git")

	host, rest, ok := splitHostRest(r)
	if !ok {
		return strings.ToLower(r)
	}
	return strings.ToLower(host + "/" + strings.TrimPrefix(rest, "/"))
}

// splitHostRest finds the host and the path after it, whether the path is
// separated by ':' (scp-like syntax) or '/' (URL-like, scheme already
// stripped).
func splitHostRest(r string) (host, rest string, ok bool) {
	colon := strings.Index(r, ":")
	slash := strings.Index(r, "/")
	switch {
	case colon >= 0 && (slash < 0 || colon < slash):
		return r[:colon], r[colon+1:], true
	case slash >= 0:
		return r[:slash], r[slash:], true
	default:
		return "", "", false
	}
}
