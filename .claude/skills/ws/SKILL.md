---
name: ws
description: Start work on one Repose workstream by number (e.g. /ws 03). Loads the shared preamble and that workstream's launch block from docs/workstreams/PROMPTS.md, then executes it.
---

You are starting workstream `$ARGUMENTS` of Repose in this checkout, which
should be a git worktree on a branch named `ws/<nn>-<name>` created by the
owner. Confirm that with `git branch --show-current`; if you are on `main`,
stop and tell the owner to create the worktree first (the commands are in
`docs/workstreams/PROMPTS.md` under "Launching agents in parallel").

Then:

1. Read `docs/workstreams/PROMPTS.md`. Treat its "Shared preamble" section as
   your standing instructions for this whole session, and find the
   per-workstream block whose heading starts with the number in
   `$ARGUMENTS` (for example `### 03 hostd`). That block is your task.
2. Read, in order, exactly as the preamble says: `CLAUDE.md`, `docs/README.md`,
   `docs/DESIGN.md`, `docs/DECISIONS-INDEX.md` (then `docs/DECISIONS.md` at the
   entries you need), `docs/workstreams/README.md`, the
   workstream doc named in the block, and every `docs/interfaces/` file that
   doc lists under "Interfaces owned / consumed". Do not skim.
3. Add your claim line to `docs/workstreams/STATUS.md` and commit it.
4. Execute the block to completion under the preamble's rules. Finish with
   the report the preamble asks for.

If `$ARGUMENTS` starts with `m` (`m1`, `m2`, `m3`, `m3-web`, ...), there is
no numbered block: read `docs/MILESTONES.md` and the matching "M<n>
bring-up" section of `docs/workstreams/PROMPTS.md` (a suffix such as
`-web` names one of that section's sessions), and run that integration
session instead, closing the host-level checklist items the wave's
workstreams left open, and recording real timings in `docs/RESEARCH.md`.
Claim it in STATUS.md as `m<n>-integration` (or `m<n>-<suffix>`).
