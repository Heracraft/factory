package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// The machine's checkout lives at /home/dev/<name>, where <name> is the
// laptop folder the first sync came from (DECISIONS I-368). The guest
// records the name in checkoutFile; guestd, the tmux session and
// repose-checkout read it by the same rule as checkoutVar
// (interfaces/guest-conventions.md "The checkout").
const checkoutFile = "~/.repose/checkout"

// checkoutVar is shell that sets repose_co to the checkout's absolute
// path in the guest: the directory checkoutFile names, else ~/<slug> (a
// machine set up before I-368, whose guestd made that directory at every
// start), else the home directory itself, where a machine with no
// checkout works. slug is a validated project slug.
func checkoutVar(slug string) string {
	return fmt.Sprintf(`repose_n=
[ -f %[1]s ] && IFS= read -r repose_n < %[1]s || true
case "$repose_n" in ""|.*|*/*) repose_n= ;; esac
if [ -n "$repose_n" ] && [ -d "$HOME/$repose_n" ]; then repose_co="$HOME/$repose_n"
elif [ -d "$HOME/%[2]s" ]; then repose_co="$HOME/%[2]s"
else repose_co="$HOME"; fi
`, checkoutFile, slug)
}

// checkoutCreate is shell, after checkoutVar, that makes the checkout
// when the guest has none: ~/<want> when that is free (missing, or an
// empty directory), else ~/<slug>; the name goes in checkoutFile. It
// prints "#created" when it made one. want is checkoutName's answer.
func checkoutCreate(slug, want string) string {
	return fmt.Sprintf(`if [ "$repose_co" = "$HOME" ]; then
  repose_n=%[2]s
  if [ -z "$repose_n" ] || { [ -e "$HOME/$repose_n" ] && { [ ! -d "$HOME/$repose_n" ] || [ -n "$(ls -A "$HOME/$repose_n")" ]; }; }; then repose_n=%[1]s; fi
  mkdir -p "$HOME/$repose_n" ~/.repose
  printf '%%s\n' "$repose_n" > %[3]s
  repose_co="$HOME/$repose_n"
  echo '#created'
fi
`, slug, shQuote(want), checkoutFile)
}

// checkoutName is the directory a first sync from the checkout at root
// makes in the guest: the folder's own name, made safe the way a project
// name is ("job search" is job-search, a leading dot goes), or "" when
// nothing is left, and the guest uses the slug.
func checkoutName(root string) string {
	if root == "" {
		return ""
	}
	return dirProjectName(filepath.Base(root))
}

// checkoutReport is shell, after checkoutVar, that prints the checkout's
// name under the home as "#checkout <name>", or a bare "#checkout" when
// the machine has none.
const checkoutReport = `if [ "$repose_co" = "$HOME" ]; then echo '#checkout'; else printf '#checkout %s\n' "${repose_co##*/}"; fi
`

// parseCheckout finds checkoutReport's line in out: the name, and
// whether the line was there at all.
func parseCheckout(out string) (name string, ok bool) {
	for _, l := range strings.Split(out, "\n") {
		if l == "#checkout" {
			return "", true
		}
		if n, found := strings.CutPrefix(l, "#checkout "); found {
			return strings.TrimSpace(n), true
		}
	}
	return "", false
}

// guestCheckoutName asks the guest where the checkout is: its name under
// /home/dev, or "" when the machine has none and work happens in the
// home directory.
func guestCheckoutName(ctx context.Context, t sshTarget, slug string) (string, error) {
	out, err := runSSH(ctx, t, checkoutVar(slug)+checkoutReport, nil)
	if err != nil {
		return "", stepFailed("find the checkout on the machine", err, "")
	}
	name, _ := parseCheckout(string(out))
	return name, nil
}

// guestHomePath is the absolute path of name under the guest's home;
// the home itself for "".
func guestHomePath(name string) string {
	if name == "" {
		return "/home/dev"
	}
	return "/home/dev/" + name
}

// homeShell is name under the home as a shell word for a guest script:
// "$HOME"/'<name>', or "$HOME".
func homeShell(name string) string {
	if name == "" {
		return `"$HOME"`
	}
	return `"$HOME"/` + shQuote(name)
}

// tildePath is name as the guest's shell spells it: "~/<name>", or "~".
func tildePath(name string) string {
	if name == "" {
		return "~"
	}
	return "~/" + name
}
