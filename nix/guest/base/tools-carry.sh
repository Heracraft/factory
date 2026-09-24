# repose-tools-install: the guest half of the tools carry (DECISIONS I-221,
# I-222). The CLI writes ~/.repose/tools-wanted.json (guest-conventions.md
# "Tools carry") and runs `repose-tools-install plan` in the carry's ssh;
# the user unit repose-tools-carry runs `repose-tools-install run` in the
# background.
#
#   plan  a few ms: prints `#installing <names>` for what the guest lacks,
#         writes their commands to $XDG_RUNTIME_DIR/repose-installing (one
#         per line, read by command-not-found), prints a `#warn` when the
#         project's node major cannot be made the default, and starts the
#         unit. Nothing is installed here.
#   run   installs each missing tool: a nixpkgs package that has
#         bin/<command> first (nix-locate, else the attribute named like
#         the command), with `nix profile add`; else the laptop's manager,
#         into a directory on the login PATH. What fails is logged in
#         ~/.repose/tools-install.log and said once by the next `repose run`
#         (~/.repose/tools-notices). A failed tool is not retried until the
#         laptop's entry for it changes. The pass ends by writing the carry
#         marker, so the CLI stops sending an unchanged list.
#
# Idempotent: a tool on PATH is skipped, a pass that stopped midway (a
# reboot) is resumed by the unit at the next boot because no marker was
# written, and a list that changed during a pass gets another pass.

wanted="$HOME/.repose/tools-wanted.json"
state="$HOME/.repose/tools"
failed="$state/failed"
log="$HOME/.repose/tools-install.log"
notices="$HOME/.repose/tools-notices"
marker="$HOME/.repose/carry/tools"
rt="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
installing="$rt/repose-installing"
user="${USER:-$(id -un)}"

# The login PATH's directories: this process's PATH (a login shell's, which
# since I-227 starts with every package manager's user bin dir), then the
# directories that must count even when it was not (a plain ssh command on
# an older base). ~/go/bin and ~/.cargo/bin are on it, so a tool the user
# installed with `go install` or `cargo install` is not installed again.
search_path() {
  printf '%s' "${PATH:-}:$HOME/.local/bin:$HOME/.local/share/pnpm:$HOME/.npm-global/bin:$HOME/go/bin:$HOME/.cargo/bin:$HOME/.bun/bin:$HOME/.deno/bin:$HOME/.nix-profile/bin:$HOME/.local/state/nix/profile/bin:/etc/profiles/per-user/$user/bin:/run/current-system/sw/bin"
}

has_cmd() {
  PATH="$(search_path)" command -v "$1" >/dev/null 2>&1
}

# where_cmd prints the directory a command resolves from, "" for none.
where_cmd() {
  local p
  p=$(PATH="$(search_path)" command -v "$1" 2>/dev/null) || return 0
  dirname "$p"
}

node_major() {
  local v
  v=$(PATH="$(search_path)" node --version 2>/dev/null) || return 0
  v=${v#v}
  printf '%s' "${v%%.*}"
}

# node_profile_wins: a nodejs in dev's nix profile would be the node of a
# new login shell, because the node found today is not in one of the
# directories env.nix puts before the profile.
node_profile_wins() {
  local d
  d=$(where_cmd node)
  case "$d" in
    "" | "$HOME/.nix-profile/bin" | "$HOME/.local/state/nix/profile/bin" | "/etc/profiles/per-user/$user/bin" | /run/current-system/sw/bin) return 0 ;;
  esac
  return 1
}

items_tsv() {
  jq -r '.items[] | [.name, (.manager // ""), (.pkg // ""), (.version // ""), (.bins | join(" "))] | join("\u001f")' "$wanted"
}

item_key() { printf '%s|%s|%s|%s' "$1" "$2" "$3" "$4"; }

is_failed() { [ -f "$failed" ] && grep -qxF "$1" "$failed"; }

plan() {
  [ -s "$wanted" ] || return 0
  local hash names=() cmds=() name manager pkg version bins b present want cur
  hash=$(jq -r '.hash' "$wanted")
  if [ -f "$marker" ] && [ "$(cat "$marker")" = "$hash" ]; then
    return 0
  fi
  while IFS=$'\x1f' read -r name manager pkg version bins; do
    [ -n "$name" ] || continue
    present=
    for b in $bins; do
      if has_cmd "$b"; then present=1; break; fi
    done
    [ -z "$present" ] || continue
    is_failed "$(item_key "$name" "$manager" "$pkg" "$version")" && continue
    names+=("$name")
    for b in $bins; do cmds+=("$b"); done
  done < <(items_tsv)
  want=$(jq -r '.node // empty' "$wanted")
  if [ -n "$want" ]; then
    cur=$(node_major)
    if [ "$cur" != "$want" ]; then
      if node_profile_wins; then
        names+=("nodejs_$want")
      else
        echo "#warn This project asks for node $want and the guest's node is ${cur:-missing}, from $(where_cmd node), which comes before the nix profile on PATH; run \`repose config add nodejs_$want\` to make node $want the guest's."
      fi
    fi
  fi
  if [ "${#cmds[@]}" -gt 0 ]; then
    mkdir -p "$rt" 2>/dev/null || true
    printf '%s\n' "${cmds[@]}" > "$installing.new" && mv -f "$installing.new" "$installing" || true
  fi
  if [ "${#names[@]}" -gt 0 ]; then
    echo "#installing ${names[*]}"
  fi
  XDG_RUNTIME_DIR="$rt" systemctl --user start --no-block repose-tools-carry.service 2>/dev/null || true
}

logline() {
  mkdir -p "$(dirname "$log")"
  printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >> "$log"
}

# done_installing drops a tool's commands from the installing file.
done_installing() {
  [ -f "$installing" ] || return 0
  local tmp="$installing.tmp" b
  cp "$installing" "$tmp"
  for b in "$@"; do
    grep -vxF "$b" "$tmp" > "$tmp.2" || true
    mv -f "$tmp.2" "$tmp"
  done
  if [ -s "$tmp" ]; then mv -f "$tmp" "$installing"; else rm -f "$tmp" "$installing"; fi
}

add_installing() {
  mkdir -p "$rt" 2>/dev/null || return 0
  local b
  for b in "$@"; do
    grep -qxF "$b" "$installing" 2>/dev/null || printf '%s\n' "$b" >> "$installing"
  done
}

# resolve_attr prints the nixpkgs attribute whose bin/<cmd> it is, "" for
# none: nix-locate's index when the base has it (the call nix-index's own
# command-not-found makes; each line is `<attr>.<output>`), else the
# attribute named like the command, if nixpkgs has one. The attribute
# named like the command wins, then one outside a package set, then the
# shortest, then the first by name.
resolve_attr() {
  local cmd=$1 out best="" a
  if command -v nix-locate >/dev/null 2>&1; then
    out=$(nix-locate --minimal --no-group --type x --type s --whole-name --at-root "/bin/$cmd" 2>/dev/null || true)
    while IFS= read -r a; do
      [ -n "$a" ] || continue
      case $a in *.*) a=${a%.*} ;; esac
      if [ "$a" = "$cmd" ]; then best=$a; break; fi
      if better_attr "$a" "$best"; then best=$a; fi
    done <<< "$out"
    printf '%s' "$best"
    return 0
  fi
  if nix eval --raw "nixpkgs#$cmd.name" >/dev/null 2>&1; then
    printf '%s' "$cmd"
  fi
}

# better_attr: is $1 a better pick than $2 ("" is the worst)?
better_attr() {
  local a=$1 b=$2 da=0 db=0
  [ -n "$b" ] || return 0
  case $a in *.*) da=1 ;; esac
  case $b in *.*) db=1 ;; esac
  [ "$da" -eq "$db" ] || { [ "$da" -lt "$db" ]; return; }
  [ "${#a}" -eq "${#b}" ] || { [ "${#a}" -lt "${#b}" ]; return; }
  [[ "$a" < "$b" ]]
}

profile_add() {
  local sub=add
  nix profile add --help >/dev/null 2>&1 || sub=install
  timeout 1200 nix profile "$sub" "nixpkgs#$1"
}

# attempt runs one install command with its output in the log; on failure
# `reason` is its last line.
reason=
attempt() {
  local out rc
  logline "running: $*"
  out=$("$@" 2>&1) && rc=0 || rc=$?
  printf '%s\n' "$out" | tail -n 40 >> "$log"
  if [ "$rc" -ne 0 ]; then
    reason=$(printf '%s\n' "$out" | grep -v '^[[:space:]]*$' | tail -n 1 | cut -c1-160)
    [ -n "$reason" ] || reason="exit status $rc"
    return 1
  fi
  return 0
}

install_one() {
  local name=$1 manager=$2 pkg=$3 version=$4 bins=$5 primary attr b
  primary=${bins%% *}
  reason=
  if command -v nix >/dev/null 2>&1; then
    attr=$(resolve_attr "$primary")
    if [ -n "$attr" ] && attempt profile_add "$attr" && has_cmd "$primary"; then
      logline "$name: installed nixpkgs#$attr"
      return 0
    fi
  fi
  # The laptop's manager, into a directory on the login PATH.
  case "$manager" in
    npm | pnpm | bun)
      attempt timeout 1200 npm install -g --no-fund --no-audit "$pkg@${version:-latest}" || return 1 ;;
    go)
      mkdir -p "$HOME/.local/bin"
      attempt timeout 1200 env GOBIN="$HOME/.local/bin" go install "$pkg@${version:-latest}" || return 1 ;;
    cargo)
      if [ -n "$version" ]; then
        attempt timeout 1200 cargo install --locked --root "$HOME/.local" "$pkg" --version "$version" || return 1
      else
        attempt timeout 1200 cargo install --locked --root "$HOME/.local" "$pkg" || return 1
      fi ;;
    uv | pipx)
      attempt timeout 1200 uv tool install "$pkg${version:+==$version}" || return 1 ;;
    *)
      [ -n "$reason" ] || reason="no nixpkgs package has bin/$primary"
      return 1 ;;
  esac
  for b in $bins; do
    if has_cmd "$b"; then
      logline "$name: installed with $manager"
      return 0
    fi
  done
  reason="installed with $manager, but none of its commands is on PATH"
  return 1
}

notice() {
  printf '%s\n' "$*" >> "$notices"
  logline "$*"
}

node_pass() {
  local want cur prev
  want=$(jq -r '.node // empty' "$wanted")
  [ -n "$want" ] || return 0
  cur=$(node_major)
  [ "$cur" != "$want" ] || return 0
  node_profile_wins || return 0 # plan said so already
  prev=$(cat "$state/node" 2>/dev/null || true)
  if [ -n "$prev" ] && [ "$prev" != "nodejs_$want" ]; then
    nix profile remove "$prev" >/dev/null 2>&1 || true
  fi
  if attempt profile_add "nodejs_$want" && [ "$(bash -lc 'node --version' 2>/dev/null | sed 's/^v//; s/\..*//')" = "$want" ]; then
    printf '%s\n' "nodejs_$want" > "$state/node"
    logline "node: nodejs_$want is the node of new shells"
    return 0
  fi
  nix profile remove "nodejs_$want" >/dev/null 2>&1 || true
  rm -f "$state/node"
  notice "Could not make node $want the guest's node: ${reason:-another node comes first on PATH}. Run \`repose config add nodejs_$want\`."
}

pass() {
  local name manager pkg version bins b present key
  mkdir -p "$state"
  node_pass
  while IFS=$'\x1f' read -r name manager pkg version bins; do
    [ -n "$name" ] || continue
    present=
    for b in $bins; do
      if has_cmd "$b"; then present=1; break; fi
    done
    key=$(item_key "$name" "$manager" "$pkg" "$version")
    if [ -n "$present" ] || is_failed "$key"; then
      # shellcheck disable=SC2086
      done_installing $bins
      continue
    fi
    # shellcheck disable=SC2086
    add_installing $bins
    if install_one "$name" "$manager" "$pkg" "$version" "$bins"; then
      :
    else
      printf '%s\n' "$key" >> "$failed"
      notice "Could not install $name: $reason"
    fi
    # shellcheck disable=SC2086
    done_installing $bins
  done < <(items_tsv)
}

run() {
  sleep "${REPOSE_TOOLS_DELAY:-2}"
  local hash
  while [ -s "$wanted" ]; do
    hash=$(jq -r '.hash' "$wanted")
    if [ -f "$marker" ] && [ "$(cat "$marker")" = "$hash" ]; then
      break
    fi
    pass
    if [ "$(jq -r '.hash' "$wanted")" = "$hash" ]; then
      mkdir -p "$(dirname "$marker")"
      printf '%s\n' "$hash" > "$marker"
      break
    fi
  done
  rm -f "$installing"
  # keep the log small
  if [ -f "$log" ] && [ "$(wc -c < "$log")" -gt 1048576 ]; then
    tail -c 524288 "$log" > "$log.tmp" && mv -f "$log.tmp" "$log"
  fi
}

case "${1:-}" in
  plan) plan ;;
  run) run ;;
  *)
    echo "usage: repose-tools-install plan|run" >&2
    exit 64 ;;
esac
