# Releasing: the queue and the release session

Several agents build on this repository at once. Each one works on its own
worktree branch and queues the branch when it is done; one session, the
conductor, turns the queue into a release: it merges, verifies, moves
`main`, and ships every target the merged changes reach. DECISIONS I-416
records why. `ORCHESTRATION.md` covers the rest of the conductor's job;
this file is the part between "a branch is done" and "it is live".

## For an agent building a change

1. Start in a worktree, never in the main checkout:
   `git worktree add ../kanali-<slug> -b <slug> main` and work there.
2. Claim the work in `docs/workstreams/STATUS.md` (CLAUDE.md).
3. Reserve decision ids before writing them:
   `ops/dev/release-queue id [N]`. It reads every local branch, every
   worktree's uncommitted `docs/DECISIONS.md` and the reservations, under
   one lock, so two agents never write the same `I-<n>`.
4. Finish the change with the evidence its checklist names. Then commit,
   `git merge main`, rerun the checks the merge touches, and queue:

   ```
   ops/dev/release-queue add --live "repose start of a stopped e2e project: under 12 s"
   ```

   `add` refuses a dirty tree, a branch with nothing main lacks, and a branch
   that conflicts with main anywhere but `docs/DECISIONS-INDEX.md`. It
   records the commit, so later commits need another `add`; that works
   while the branch is in a cut too, and the conductor takes the new
   commits with `resume` or in the next cut. It prints what the branch
   ships as (below).
5. Stop there. Don't merge into `main`, push, tag, publish a base or switch
   a host. If the conductor asks for a change, make it on the same branch
   and `add` again.

`ops/dev/release-queue ls` shows the queue. A branch that moved after it
was queued is marked; queue it again or the release takes the old commit.

## What a branch ships as

`add` reads the branch's diff against main and names the deploy targets it
reaches. Go packages count through `go list -deps`: a change to
`internal/obs` reaches every server binary but not the CLI.

| Target | Reached by | Shipped by | Who may run it |
|---|---|---|---|
| `api` | `cmd/api`, `cmd/repose-admin` and what they import | push to main (Coolify, about 2 min) | conductor, with the owner's go-ahead for the release |
| `web` | `apps/web/` (the public docs included) | push to main | same |
| `cli` | `cmd/repose` and what it imports | a `v*` tag (`release.yml`) | same |
| `base` | `nix/guest/`, `cmd/guestd`, `cmd/repose-hook`, shared nix files | `repose-admin base publish --rev <sha on main>` | same |
| `host` | `nix/hosts/`, `cmd/hostd`, shared nix files | a switch of each host | the owner, or the conductor when the owner says so for that release |
| `edge` | `nix/edge/`, `cmd/gateway`, shared nix files | a switch of the edge | the owner (drops live sessions for about a second) |
| `infra` | `infra/` | `tofu apply` | the owner |
| `none` | docs outside `apps/web`, ops notes, tests | nothing | |

## For the conductor: cutting a release

A release is cut when the owner asks for one, not each time a branch is
queued: every release costs a full verification, a deploy and a live
check, so the queue is meant to fill. A fix for something broken in
production (main red, a regression live) is the exception; say so when
asking. `ops/dev/release-queue abandon <release>` undoes a cut that
should wait, and puts its branches back in the queue.

1. **See what is waiting.** `ops/dev/release-queue ls`. Ask each agent
   with a branch in flight (`ListAgents`, `SendMessage`) whether it is
   about to queue; a release that leaves out a branch five minutes from
   done costs a second release. Sessions get renamed: a send that fails
   with "no agent named" means the name changed, not that the session
   ended. Run `ListAgents` again and match the `[ref]`, and tell agents
   your current name, since their replies to an old one are lost.
2. **Cut.** `ops/dev/release-queue cut` makes `~/kanali-r<date>-<n>` on
   branch `release/r<date>-<n>` from main and merges every queued branch
   in queue order, each with `--no-ff`. A conflict that is only the
   decisions index is settled by regenerating it. Any other conflict
   stops the cut: resolve it in the release worktree (or send it back to
   the branch's agent and `drop` the branch), commit, then
   `ops/dev/release-queue resume <release>`.
3. **Freeze when you verify.** Commits that arrive after the cut wait for
   the next release unless they fix the release itself; each one taken
   in means verifying again.
4. **Verify the merge, not the branches.** In the release worktree, for
   everything the release ships as:
   - always: `python3 ops/dev/decisions-index.py --check`; `go build ./...`,
     `go vet ./...`, `golangci-lint run ./...`; `go test -race ./...`
     with a real Postgres for the api packages (on a repose guest, unset
     `REPOSE_PROJECT REPOSE REPOSE_HOOK_AGENT` first, and keep `TMPDIR`
     at `/tmp`: a longer one pushes the hostd fakes' unix sockets past the
     108-byte limit); `go test ./internal/cli
     -run TestDocs`; the `docs/CHECKLIST.md` greps;
   - `web`: `pnpm --filter web exec vitest run`, `svelte-check`, `eslint .`,
     `build`, and the playwright suites (on a repose guest, set
     `PLAYWRIGHT_CHROMIUM_PATH` to the base's
     `~/.cache/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell`,
     and run them alone: they serve on 127.0.0.1:4173, and another
     session's preview or a parallel `go test` there fails them all);
   - `base`: `nix build ./nix#guest-system` and the `guest-closure-size`
     check; the VM checks the merged change touches, on a box that boots
     them (they do not boot on a repose guest);
   - `host` / `edge`: `nix eval` of each configuration's toplevel and a
     `dry-activate` on the target before any switch.
   A failure is fixed in the release worktree when it comes from the merge,
   or sent back to the branch's agent when it is the branch's own.
5. **Ask once.** Send the owner one summary (`repose-ask --options
   yes,no`): the release id, the branches with one line each, the targets,
   and what each target's ship step does to tenants. A yes covers this
   release's push, tag and base publish. A host or edge switch still needs
   the owner's word for that switch.
6. **Move main.** `git -C ~/kanali merge --ff-only release/<id>`; the main
   checkout must be clean. Push `main`. Watch CI on the pushed commit; a
   push redeploys `api` and `web` whether CI passes or not, so a red run is
   fixed at once.
7. **Ship the rest**, in this order when present: `base` publish (the rev
   is the pushed main commit), `cli` tag (the next `v0.1.N`, with notes
   from the merged branches), `host` switches (RUNBOOK "Switch a host to
   main") and the `edge` switch.
8. **Check it live.** Run each branch's `--live` check on throwaway `e2e-*`
   projects, never on the owner's projects, at most two alive at once.
   Paste the output into the release record.
9. **Record and close.** One line in `docs/workstreams/STATUS.md` per
   release: the id, the branches, the targets shipped with versions, the
   live evidence, anything not done. Commit it to main and push. Then
   `ops/dev/release-queue done <release>`, which removes the release
   worktree. Tell each agent whose branch shipped; it may then delete its
   worktree, or the owner does.

## Where the queue lives

In the repository's common git directory (`.git/release-queue/`), shared by
every worktree of the checkout and by no branch: `entries/` (one file per
branch), `releases/` (one per release: base commit, merged commits,
targets), `ids` (reservations). Queueing never makes a commit on any
branch, so it cannot conflict. The record that outlives the machine is the
merge commits on main and the STATUS line.
