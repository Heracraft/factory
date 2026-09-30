# repose-checkout prints the absolute path of the project's checkout, by
# the rule every reader of it shares (interfaces/guest-conventions.md "The
# checkout", DECISIONS I-368): the directory ~/.repose/checkout names,
# else ~/<slug> (a machine set up before I-368), else the home directory,
# where a machine with no checkout works. The CLI's checkoutVar and
# guestd's findCheckout apply the same rule. A function of pkgs, called by
# tmux.nix and profile.nix; profile.nix installs it.
{ pkgs }:
pkgs.writeShellApplication {
  name = "repose-checkout";
  runtimeInputs = [ pkgs.jq pkgs.coreutils ];
  text = ''
    home=''${HOME:-/home/dev}
    name=
    if [ -f "$home/.repose/checkout" ]; then
      IFS= read -r name < "$home/.repose/checkout" || true
    fi
    case "$name" in ""|.*|*/*) name= ;; esac
    if [ -n "$name" ] && [ -d "$home/$name" ]; then
      printf '%s\n' "$home/$name"
      exit 0
    fi
    slug=
    if [ -s "$home/.repose/project.json" ]; then
      slug=$(jq -r '.slug // empty' "$home/.repose/project.json" 2>/dev/null || true)
    fi
    case "$slug" in ""|.*|*/*) slug= ;; esac
    if [ -n "$slug" ] && [ -d "$home/$slug" ]; then
      printf '%s\n' "$home/$slug"
      exit 0
    fi
    printf '%s\n' "$home"
  '';
}
