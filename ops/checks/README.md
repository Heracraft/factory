# M3 checks: the guest half of the M3 gate as scripts

`docs/workstreams/PROMPTS.md` "M3 bring-up / m3" asks for a set of things
done on host-01 through the real api, each closing checklist rows of
workstreams 05, 12, 13 and 14 with pasted evidence. These scripts are those
things, written so the evidence is a file somebody can paste rather than a
terminal history nobody can find again. Each one drives the real api with
the `repose` CLI (and raw HTTP with the CLI's token where the CLI has no
command, such as the menu route), reaches the guest through the gateway,
the host through the edge's operator sshd, and `repose-admin` and Postgres
through `docker exec` on the control VM. None of them applies
infrastructure or touches Coolify; `resilience.sh` is the one that kills or
restarts an api container, and says so.

Setup once: `cp m3.env.example m3.env` and fill in the addresses; log the
CLI in (`repose login`); make sure `ssh -p 2222 root@<edge>` and
`ssh root@<control>` work with the operator key, and that `repose run
--no-attach` has put a `Host <slug>.repose` block in `~/.ssh/repose/config`.

| Script | M3 step | What it proves, and the rows it closes | Needs |
|---|---|---|---|
| `secrets.sh` | 2 | a secret set through the api is `0400 dev` on the guest's tmpfs, exported in a login shell, gone within 5 s of `rm`, refused inside a fragment, and its value is in no route, log, build log, event or audit row. `features/secrets.md` Kind 3; 05 §9 "no route returns a value" (real path); 14 §5 "cannot read secret values", "secret set or delete (name only)" | PROJECT running |
| `menu.sh` | 3 | a catalog package added through `PUT /config {menu}` with the SSE build log, on PATH in the guest with the same `boot_id` and tmux session; the generated header; the takeover flow and the 409; the five restricted-eval refusals with the contract's exact first lines; a fixed-output fetch that builds; GC roots and the live closure; `--with-destroy` for roots gone after destroy; `--with-closure-cap` for `closure_too_large` with ten paths; eval and build timings from `build_done` for `docs/RESEARCH.md`. 12 §9 rows 2 to 5, 7, 8, 11; 05 §9 "menu selection renders to a fragment that builds" (real build) | PROJECT running |
| `notifications.sh` | 4 | a `completed` event from each of the five agents in the real guest, the ntfy delivery timestamp, `repose status` and `repose events`, the ntfy URL absent from logs. 13 §9 "real guest" and "`repose status` shows last event". Claude, Codex and opencode fire their own hooks when logged in inside the guest; otherwise their native payload is replayed through `repose-hook` from a tmux window named after the agent, and the report says so. Gemini and pi use the shipped pane-idle heuristic | PROJECT running, `NTFY_URL` |
| `resilience.sh` | 5 | `grpc`: api-grpc killed mid-build, hostd reconnects, the op completes; `http`: api restarted mid-snapshot; `bump`: a security base publish builds the unheld project and skips the held one, `kernel_changed` when `BASE_REV` bumps the kernel; `expiry`: aged snapshots deleted, never one a restore is reading. 05 §9 "ops survive an api restart", "api-grpc restarts absorbed", "base bump job", "snapshot expiry"; 12 §9 `kernel_changed` and base bump rows | PROJECT running; `PROJECT_HELD`, `BASE_REV` for `bump`; the conductor told before `grpc` and `http` |
| `isolation-host01.sh` | 6 | fills the `REPOSE_ISOLATION_*` environment of `test/isolation` for host-01 with the owner's guest (through the gateway) and a second user's guest (through audited `repose-admin exec`), runs the suite, then the operator password attempt against the host and the edge with the sshd journals. 14 §9 boundary rows, "operator access ... a password attempt is logged" | PROJECT and `PROJECT_B` running, `LOGIN_B` |
| `audit-rows.sh` | 6 | triggers each audited action of 14 §5 that can be triggered without disrupting a tenant and prints the `audit_log` rows grouped by action, naming the ones with no producer | PROJECT running |

Evidence lands in `out/<check>-<stamp>.txt` (git-ignored). The `evidence`
blocks are headed the way the checklist rows are worded, so the paste into
`STATUS.md` or the report is a copy of the block, not a rewrite.

## Things the scripts cannot do alone

- **A kernel-changing base.** Every merged `main` since the scaffold locks
  the same nixpkgs (`b1b8759`), so no published revision changes the guest
  kernel and `kernel_changed` cannot come out true from the catalogue of
  existing commits. `resilience.sh bump` takes `BASE_REV`; a `nix flake
  update nixpkgs` commit is the honest way to get one, and that is a real
  base bump for every guest, so it is the conductor's call, not a check's.
- **Real agent hooks for Claude, Codex and opencode** need the owner logged
  in inside the guest (Claude: `repose run` detects no credentials and
  runs `claude` for the device code; Codex and opencode: their auth files
  are synced from the laptop by `repose run`). Without that the replay
  path is what runs, and the STATUS line says "replayed".
- **The second tenant** for the isolation rows is a second person's
  project (`PROJECT_B`, `LOGIN_B`); their certificate never leaves their
  laptop, so guest B is driven through `repose-admin exec`, which is
  audited and shows up in `audit-rows.sh`'s output as `exec` rows.
- **`hostd for host X cannot act on host Y`** needs a second host.
