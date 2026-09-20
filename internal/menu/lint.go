package menu

import (
	"fmt"
	"strings"
)

// SystemAllowlist is the set of NixOS option prefixes a `nixos` snippet (and
// a fragment's repose.system) may set. It is a copy of
// nix/guest/system-allowlist.json, which nix/guest/contract.nix reads at
// evaluation; TestAllowlistMatchesNix fails when the two drift. Reviewed
// under docs/SECURITY.md boundary 8: never networking, users, boot,
// virtualisation or services.openssh.
var SystemAllowlist = []string{
	"services.postgresql",
	"services.redis",
	"services.mysql",
	"services.memcached",
	"services.rabbitmq",
	"services.meilisearch",
	"services.nats",
}

// LintSnippet checks that every top-level attribute path a `nixos` snippet
// defines starts with an allowlisted prefix. The snippet is a sequence of
// `path = value;` definitions; the lexer skips strings and comments and
// tracks brace depth, so only depth-zero paths count.
func LintSnippet(snippet string) error {
	for _, p := range topLevelPaths(snippet) {
		if !allowed(p) {
			return fmt.Errorf("option %q is outside the allowlist %v", p, SystemAllowlist)
		}
	}
	return nil
}

func allowed(path string) bool {
	for _, pre := range SystemAllowlist {
		if path == pre || strings.HasPrefix(path, pre+".") {
			return true
		}
	}
	return false
}

// topLevelPaths returns the attribute paths defined at brace depth zero.
func topLevelPaths(s string) []string {
	var paths []string
	depth := 0
	i := 0
	var ident strings.Builder
	flush := func() {
		if depth == 0 && ident.Len() > 0 {
			paths = append(paths, ident.String())
		}
		ident.Reset()
	}
	for i < len(s) {
		c := s[i]
		switch {
		case c == '#':
			ident.Reset()
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '"':
			ident.Reset()
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' {
					i++
				}
				i++
			}
			i++
		case c == '\'' && i+1 < len(s) && s[i+1] == '\'':
			ident.Reset()
			i += 2
			for i+1 < len(s) && (s[i] != '\'' || s[i+1] != '\'') {
				i++
			}
			i += 2
		case c == '{' || c == '[' || c == '(':
			ident.Reset()
			depth++
			i++
		case c == '}' || c == ']' || c == ')':
			ident.Reset()
			depth--
			i++
		case c == '=' && depth == 0:
			flush()
			i++
		case c == ';':
			ident.Reset()
			i++
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			// whitespace between an identifier and its `=` keeps the ident
			if ident.Len() > 0 && depth == 0 {
				j := i
				for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
					j++
				}
				if j < len(s) && s[j] == '=' {
					i = j
					continue
				}
			}
			ident.Reset()
			i++
		case isIdentChar(c):
			if depth == 0 {
				ident.WriteByte(c)
			}
			i++
		default:
			ident.Reset()
			i++
		}
	}
	return paths
}

func isIdentChar(c byte) bool {
	return c == '.' || c == '_' || c == '-' || c == '\'' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
