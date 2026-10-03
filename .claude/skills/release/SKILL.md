---
name: release
description: Cut and ship a Repose release from the release queue as the conductor (/release). Merges every queued worktree branch onto a release/<id> worktree, verifies the merge, asks the owner once, moves main, ships api/web/base/cli as the merged changes need, checks live and records it.
---

You are the conductor cutting a release of Repose. `docs/ops/RELEASE.md`
is the procedure; read it in full first, then `docs/ops/ORCHESTRATION.md`
for the rest of the conductor's role.

1. `ops/dev/release-queue ls`. `ListAgents`, then ask each session with a
   branch in flight whether it is about to queue; wait for the ones that
   say minutes, not hours.
2. `ops/dev/release-queue cut`, and `resume` after resolving any conflict.
3. Run every check RELEASE.md step 3 names for the release's targets, in
   the release worktree. Paste the output in your notes; "tests pass"
   closes nothing.
4. Ask the owner once with `repose-ask --options yes,no`: release id,
   branches, targets, what each ship step does to tenants. Stop on no or
   on no answer.
5. Fast-forward main, push, watch CI, then ship base / CLI as needed. Host
   and edge switches only with the owner's word for that switch.
6. Run each branch's `--live` check on throwaway `e2e-*` projects.
7. STATUS line, commit and push it, `ops/dev/release-queue done <id>`, and
   tell each agent whose branch shipped.
