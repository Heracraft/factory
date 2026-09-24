# Claude login once per user: the shared credential folder (proposal, 2026-09-24)

**Status: open, not decided. Two experiments and one question to
Anthropic come first.** This continues item 9 of
`2026-09-23-dev-ergonomics.md` ("users logging in N times is not
acceptable"). If it goes ahead, it needs a `DECISIONS.md` entry and a
`SECURITY.md` note, because it adds a place a secret lives. CLAUDE.md
says secrets have three homes.

## Where things stand

- **Today:** the user runs `/login` inside each guest (R2-8 amended,
  R2-15). The login persists on the guest volume, so it's one login per
  project.
- **The CLI:** it detects a missing `~/.claude/.credentials.json` and
  attaches instead of sending the prompt (`internal/cli/tmux.go:52-62`,
  `run.go:471`).
- **The fallback:** R2-15 documents `claude setup-token` stored as the
  `CLAUDE_CODE_OAUTH_TOKEN` named secret. `ANTHROPIC_API_KEY` is also
  documented.
- **The settings carry:** it strips `env`, `apiKeyHelper` and cloud-auth
  helpers from `settings.json` (I-196, `internal/cli/carry_claude.go:207-214`).

## What the terms say (https://code.claude.com/docs/en/legal-and-compliance)

- A hosted sandbox that preinstalls Claude Code must run the binary
  unmodified and must not remove any auth method.
- It must not pay for or intermediate usage. Each end user authenticates
  with their own credentials.
- Verbatim: *"developers may not collect, store, or intermediate Claude.ai
  credentials or session tokens — sign-in to a Claude account must
  complete through Anthropic's own flow."*
- Explicitly allowed: *"an end user signing in to the unmodified Claude
  Code binary with their own Claude subscription, including where a
  platform hosts Claude Code."*
- Enforcement against third-party use of subscription OAuth began in April
  2026
  (https://www.theregister.com/2026/02/20/anthropic_clarifies_ban_third_party_claude_access/).

**What this rules out:**
- The stored setup-token fallback (R2-15): repose storing the token in
  Postgres is "store". **R2-15 should be revisited on its own** whatever
  happens here.
- The CLI copying `.credentials.json` from the laptop to a guest, or from
  one guest to another: plausibly "intermediate".

The setup-token also loses Remote Control, claude.ai connectors and
`/schedule`: "it can only make model requests"
(https://code.claude.com/docs/en/authentication). Remote Control from the
phone app is the only voice path, since voice dictation doesn't work over
SSH. So a full `/login` is the login worth keeping in any case.

## The idea: one shared folder per user, holding only the credential file

The owner proposed this on 2026-09-24. Each user gets one folder on the
host, shared into every one of that user's guests. Claude Code's
credential file lives there.

- The user signs in once, through Anthropic's own flow, in any guest.
- Repose code never reads, copies, moves or proxies the file. It
  provides storage that the user's own guests share, like a home directory
  mounted on several of the user's machines.
- The login survives destroying a project, restoring a snapshot and
  rolling back the base. The file isn't in snapshots.
- If a refresh rotates the token, every guest sees the new file, so
  rotation may stop being a problem (experiment B).

### Rule: share the credential file only, never all of `~/.claude`

`~/.claude/settings.json` holds hooks, and hooks run commands. If the whole
folder were shared, a full-permission agent in project A could write a hook
that runs in every other project of that user. That breaks the headline
promise that other projects are out of reach. It would also clash with the
settings carry and with the per-guest `~/.claude.json`.

With only the credential file shared, the worst a rogue agent can do:
- read the token, which it can already do in its own guest;
- corrupt the file, which logs the user out in every guest. That is
  annoying but recoverable.

### One host only

The folder is on one host's disk, so guests on two hosts mean one login
per host.
- Today there's only host-01.
- Later, either schedule a user's guests onto one host, or accept one
  login per host. Both are far better than one per project.

### Cost

- A new secret location (host disk, outside any guest volume). This needs
  a decision entry and a `SECURITY.md` note, and `docs/features/secrets.md`
  must say where it is and why.
- A new writable virtiofs share per user on each guest, which is a small
  extra attack surface on the host.
- Destroying the account must delete the folder.

## Experiments (in order; each gates the next)

Use e2e projects on host-01 only, never a tenant's. The owner does any
host-side mount or switch.

### A. How does Claude Code write `.credentials.json`?

This decides whether a symlink or bind mount can work.

1. In a guest, `strace -f -e trace=openat,rename,renameat2,unlink,symlink
   -o /tmp/cc.trace claude`, then force a token refresh. Either wait past
   the access token's expiry, or edit `expiresAt` in the file to a past
   time and send a prompt.
2. Check:
   - Does it write in place, or write a temp file and rename it over the
     original?
   - Does it follow a symlink, or replace it?
3. Interpret:
   - If it renames over the file, a symlink into the shared folder is
     replaced by a local file and sharing silently stops.
   - A file bind mount makes the rename fail with EBUSY, and the refresh
     breaks.
   - The shared unit then has to be a directory Claude Code writes into.
     `CLAUDE_CONFIG_DIR` pointed there would share far too much (see the
     rule above), so another way must be found or the idea fails here.
4. Record the result, with the Claude Code version, in this file.

### B. Do two guests on one file stay logged in?

This is the rotation question.

1. Hand-mount one host directory into two e2e guests. Place it so Claude
   Code's credential file is the shared one, per A's result.
2. Log in once in guest 1. Start `claude` in both, and send a prompt in
   each every 30 minutes for over 24 hours, so several access tokens
   expire.
3. Record:
   - Does either guest ever get logged out or hit a 401 it doesn't
     recover from?
   - After guest 1 refreshes, does guest 2's running session pick up the
     new token (re-read after a 401), or does it need a restart?
4. Also test two guests refreshing at the same moment (force both expired,
   prompt both). File locking over virtiofs is limited. The acceptable
   result is at worst one logout, never a corrupt file that needs manual
   repair.

### C. Ask Anthropic

Only if A and B pass. Send one email describing exactly this design: the
user logs in through Anthropic's flow; the file stays on storage only that
user's VMs share; repose never reads, copies or transmits it; the unmodified
binary. Ask whether this counts as "store or intermediate". Keep the answer
in this file.

## If it fails

The best remaining experience is one click per new project:
- a `BROWSER` shim plus auto-forward (09-23 item 5), so the authorize page
  opens on the laptop;
- the user clicks once, and the login stays with the project for life.

Check whether Claude Code honours `BROWSER` over SSH; it falls back to
"Paste code here" otherwise.

Several agents in one project's guest (windows in one tmux session) share
one login, so the pain is proportional to projects, not agents.

Either way, **this does not block the private invite beta**: one click per
new project is fine for a handful of invited users.

## If it passes: build sketch

1. hostd creates `/var/lib/repose/users/<user_id>/claude-auth/`, mode
   0700 and owned by the guest's mapped uid, when the user's first guest
   starts.
2. hostd shares it into each of the user's guests as a read-write virtiofs
   tag.
3. The guest mounts it and arranges the credential file per A's result.
4. Reconcile at hostd start, following the idempotency rule in CLAUDE.md.
5. Account deletion removes the folder.
6. The CLI's missing-credentials check (`tmux.go:52-62`) still applies, now
   true for all of a user's projects after one login.
7. Tests:
   - a VM test with two guests sharing the folder;
   - a test that `settings.json` is *not* shared;
   - a live run of experiment B on the release base.
