# Secrets

Three kinds of secret, three treatments, and no fourth place. Each treatment
exists because the other two would be wrong for that kind.

## What the user sees

```
$ repose secrets set DATABASE_URL
Enter value (input hidden): ********
Stored for todo-app. Available as $DATABASE_URL and /run/repose/secrets/DATABASE_URL.

$ repose secrets list
NAME               UPDATED
DATABASE_URL       2026-09-17 14:02
GEMINI_API_KEY     2026-09-15 09:41

$ repose secrets rm GEMINI_API_KEY
Removed. Running processes that already read it keep their copy until restart.
```

Synced logins happen silently inside `repose run`:

```
Syncing logins: gh, codex, opencode, git identity
```

## Kind 1: tool logins the laptop already has

Copied at every `repose run` over the SSH session into the guest, owned by
`dev`, mode 0600. The list is exact and lives in
`interfaces/guest-conventions.md`: gh's `hosts.yml`, Codex's `auth.json`,
opencode's `auth.json`, and the two git identity keys. The platform never
sees these; they travel laptop to guest inside SSH.

Why copy rather than store: the user already has them, they rotate on the
laptop, and holding a copy of a GitHub token for every user in a database is
a liability with no benefit. Why copy at all: the agent must push while the
laptop is closed, so agent forwarding is not enough.

Rules that must hold:

- A file missing on the laptop is skipped silently; a file present is copied
  every run, so a rotated token reaches the guest on the next run.
- The copy never overwrites a guest file that is newer than the laptop's,
  because a login done inside the guest (Codex device auth, say) would be
  clobbered. Mtime decides; the CLI prints which side won when it skips.
- `~/.claude/.credentials.json`, `~/.gemini/oauth_creds.json`, and any SSH
  private key are never copied. A test feeds a laptop home containing all
  of them and asserts the tar stream contains none.

## Kind 2: Claude Code

Never copied, never stored. The user logs in inside the guest; the fallback
is a setup token stored as a named secret. agents.md carries the reasoning
and the Anthropic policy behind it.

## Kind 3: named secrets

`repose secrets set NAME` and the dashboard's secrets page. Stored by the
API as ciphertext in Postgres, encrypted with a per-user data key that is
itself wrapped by an Azure Key Vault key (DECISIONS R3-10). Delivered to the
guest at start and on every change as `/run/repose/secrets/NAME` on a
tmpfs, mode 0400, owner `dev`, and exported into login shells through
`/run/repose/secrets.env`.

Why central: an unattended agent needs them when no laptop is connected, and
a stopped guest that restarts at 03:00 for a base bump needs them too. Why
envelope encryption in Postgres rather than one Key Vault secret per value:
Key Vault is priced and rate-limited per operation, and a guest start would
need one call per secret.

Rules that must hold:

- Names match `[A-Z][A-Z0-9_]{0,63}`. Values up to 64 KB. Binary values are
  base64 on the wire and raw in the file.
- The API never returns a value. `GET /secrets` lists names and timestamps
  only. The dashboard has no "reveal".
- A `set` on a running guest pushes `UpdateSecrets` and the file is updated
  within 5 seconds; `secrets.env` is regenerated. Already running processes
  are not restarted; the CLI says so.
- A `rm` deletes the ciphertext row and the guest file. The audit log
  records the action, the name, and never the value.
- Secrets are per project. The same name in two projects is two secrets.
  Copying between projects is a later feature.
- Secrets appear in no log, no build log, no event summary, no error
  message. Build logs are scanned for every current secret value of that
  project before storage and matching substrings are replaced with
  `[redacted]`, because a Nix build that echoes an environment variable is a
  common way a token leaks into a log that lives 90 days.
- Rotating the Key Vault key re-wraps every DEK without touching
  ciphertext; `repose-admin secrets rewrap` does it and is rehearsed before
  launch.

## Where secrets are not

Not in the repo (gitignored `.env` files do not sync; see
sync-at-launch.md). Not in the fragment (a Nix expression is a build input,
lands in the world-readable store, and shows in build logs; the pipeline
rejects a fragment containing a string that matches a current secret value
of that project). Not in tmux history the platform can see; the platform
cannot see tmux history.

## Depends on

Workstreams 05 (secrets routes, envelope encryption, Key Vault), 03
(`UpdateSecrets` delivery), 04 (`WriteSecrets`), 07 (`secrets` commands,
login sync), 08 (dashboard page), 12 (fragment scan, build log redaction).

## Deferred

Per-project LUKS so an operator with root on a host cannot read a stopped
tenant's disk (R3-10 revisit). User-level secrets shared across projects.
Copy between projects. Secret versions and rollback.
