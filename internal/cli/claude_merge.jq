# repose: merge the laptop's Claude Code settings.json into the guest's
# (DECISIONS I-196, docs/workstreams/15-dev-ergonomics.md §5.3).
#
# Run in the guest with the base's jq, as
#   jq -n --arg mode MODE --arg home LAPTOP_HOME --arg cfg LAPTOP_CLAUDE_DIR_OR_EMPTY --arg dest "$HOME" \
#      --slurpfile g GUEST.json --slurpfile l LAPTOP.json \
#      --slurpfile p PLATFORM.json --rawfile missing MISSING -f claude_merge.jq
#
#   rewrite   the laptop file with its home directory rewritten to the
#             guest's ($dest, /home/dev in every guest)
#   commands  every hook and statusLine command the merge would keep, one
#             base64 line each, for the guest to check with command -v
#   merge     the result: the guest file as the base, the laptop file on
#             top, permissions unioned, the platform's repose-hook entries
#             stripped from both sides and appended last, and the commands
#             listed (base64) in MISSING dropped
#
# Running merge twice with the same inputs gives the same bytes: the
# platform entries are removed before and added after, and the unions are
# sorted.

# swap($from; $to): a string that is $from becomes $to, and $from + "/"
# becomes $to + "/" wherever it sits ("python3 /Users/x/.claude/a.py",
# "Read(/Users/x/src/**)"). split/join is literal, so no regex escaping.
def swap($from; $to):
  if $from == "" or $from == $to then .
  elif . == $from then $to
  else split($from + "/") | join($to + "/") end;

# rewrite: the laptop's Claude config directory ($cfg, set when
# CLAUDE_CONFIG_DIR moved it off ~/.claude) becomes the guest's
# ~/.claude, in its absolute and its ~/ and $HOME/ forms; then the
# laptop's home becomes the guest's.
def rewrite($home; $cfg; $dest):
  (if $cfg != "" and ($cfg | startswith($home + "/")) then $cfg[($home | length) + 1:] else "" end) as $crel
  | walk(
      if type == "string" then
        swap($cfg; $dest + "/.claude")
        | if $crel == "" then . else
            swap("~/" + $crel; "~/.claude") | swap("$HOME/" + $crel; "$HOME/.claude") | swap("${HOME}/" + $crel; "${HOME}/.claude")
          end
        | swap($home; $dest)
      else . end);

# hooks_keep(f): keep only the hook entries for which f is true; a matcher
# group left with no hooks goes, and so does an event left with no groups.
def hooks_keep(f):
  if (.hooks | type) == "object" then
    .hooks |= (
      with_entries(
        .value |= (
          if type == "array" then
            map(if (.hooks | type) == "array" then .hooks |= map(select(f)) else . end)
            | map(select((.hooks | type) != "array" or (.hooks | length) > 0))
          else . end))
      | with_entries(select((.value | type) != "array" or (.value | length) > 0)))
  else . end;

def platform_entry: ((.command // "") | tostring | contains("repose-hook"));

def strip: hooks_keep(platform_entry | not);

def commands:
  [ ( (.hooks // {}) | if type == "object" then .[] else empty end
      | if type == "array" then .[] else empty end
      | (.hooks // []) | if type == "array" then .[] else empty end
      | .command? // empty | strings ),
    ( .statusLine | if type == "object" then (.command // empty | strings) else empty end ) ]
  | unique;

def listed($missing): . as $c | ($c | type) == "string" and any($missing[]; . == $c);

def drop_missing($missing):
  hooks_keep((.command // null) | listed($missing) | not)
  | if (.statusLine | type) == "object" and ((.statusLine.command // null) | listed($missing))
    then del(.statusLine) else . end;

def merge($g; $l; $p; $missing):
  ($g | strip) as $G
  | ($l | strip) as $L
  | ($G * $L)
  | reduce ("allow", "deny", "ask") as $k (.;
      if (($G.permissions? // {})[$k] != null) or (($L.permissions? // {})[$k] != null)
      then .permissions[$k] = ((($G.permissions? // {})[$k] // []) + (($L.permissions? // {})[$k] // []) | unique)
      else . end)
  | drop_missing($missing)
  | reduce ((($p.hooks? // {}) | keys_unsorted)[]) as $ev (.;
      .hooks[$ev] = ((.hooks[$ev] // []) + $p.hooks[$ev]));

if $mode == "rewrite" then
  $l[0] | rewrite($home; $cfg; $dest)
elif $mode == "commands" then
  (($g[0] | strip) * ($l[0] | strip)) | commands | .[] | @base64
elif $mode == "merge" then
  merge($g[0]; $l[0]; $p[0]; ($missing | split("\n") | map(select(length > 0) | @base64d)))
else
  error("unknown mode " + $mode)
end
