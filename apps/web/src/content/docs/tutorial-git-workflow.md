---
title: A git workflow for several agents
description: One branch per task, review on your laptop, merge what's good, throw the rest away.
section: Tutorials
order: 23
---

The workflow below is what a day with repose settles into once you stop treating the machine as a second laptop. It assumes you've read [Git with repose](/docs/tutorial-git).

The idea: **a task is a branch**. Every agent gets its own worktree and branch on the machine, you never edit the machine's `main`, and your laptop is where branches get reviewed and merged. The machine is where the work happens; your laptop is where the decisions happen.

## 1. Sync once, in the morning

```
repose run
```

Your `main` goes up. From here on the machine's `main` is a base for branches, not a place anyone works.

## 2. One agent per task, each in a worktree

```
repose run --worktree "add rate limiting to the public API; tests in test/ratelimit"
repose run --worktree "replace the hand-rolled date parsing with date-fns; keep the tests green"
repose run --worktree --agent codex "write the migration for the audit_log table and wire it into the model"
```

Each command prints where the agent works:

```text
Worktree: ~/todo-app-claude on branch repose/claude
Worktree: ~/todo-app-claude-2 on branch repose/claude-2
Worktree: ~/todo-app-codex on branch repose/codex
```

Three agents, three directories, three branches, no shared files. The names follow the tmux windows: the first Claude gets `claude`, the next `claude-2`, and a name whose worktree or branch already exists is skipped. Detach and let them run. `repose ps` shows who's still busy:

```
$ repose ps
WINDOW      COMMAND  ACTIVE
0:shell     bash     2h ago
1:claude    claude   now
2:claude-2  claude   4m ago
3:codex     codex    now
```

You get a [notification](/docs/notifications) as each one finishes or asks a question. `repose questions` lists what's waiting on you; `repose reply` answers without attaching.

## 3. Fetch and review on your laptop

```
$ git fetch repose
 * [new branch]  repose/claude   -> repose/repose/claude
 * [new branch]  repose/claude-2 -> repose/repose/claude-2
 * [new branch]  repose/codex    -> repose/repose/codex
```

Review each branch the way you'd review a colleague's:

```
git log --oneline main..repose/repose/claude
git diff main...repose/repose/claude
git diff main...repose/repose/claude -- test/
```

Run the tests on the machine without attaching, with the dev shell and secrets the agent had:

```
repose exec -- sh -c 'cd ~/todo-app-claude && npm test'
```

Or open the worktree in your editor over SSH with `repose code` and read it there.

## 4. Merge what's good

```
git merge repose/repose/claude
git merge repose/repose/codex
```

A branch that isn't good enough gets a second round: `repose attach`, switch to that agent's window, and tell it what to change. It commits on the same branch; you fetch again. Or ask for a pull request instead and review on GitHub: the machine has your `gh` login, so "push the branch and open a PR" works in a prompt.

## 5. Send the merged result back up

Your laptop's `main` now has two of the three branches merged. Send it to the machine:

```
repose run
```

The machine's `main` catches up. The worktrees still have their branches, based on the old `main`; that's fine, they're finished. The third agent, still working, isn't touched: `repose run` never syncs a worktree.

## 6. Tidy

On the machine, or through `repose exec`:

```
repose exec -- git worktree remove ~/todo-app-claude
repose exec -- git branch -D repose/claude
```

On your laptop, `git branch -rd repose/repose/claude` drops the fetched copy. Worktrees you leave in place cost only disk.

## The rules that make this work

- **Agents commit.** Put it in the prompt: "commit as you go" or "commit when the tests pass". An uncommitted change on the machine is the one thing that can get in the way of a sync.
- **Nobody works on the machine's `main`.** Then `repose run` is always safe, and a branch is always a clean diff against something you know.
- **Review on the laptop, not on the machine.** `git diff main...branch` on a copy the agent can't touch is a review; reading the agent's own summary is not.
- **Risky experiments get a fork.** `repose fork` copies the whole machine, worktrees, database and all, so an agent can try something destructive on the copy. [Projects and lifecycle](/docs/lifecycle#fork-a-project).

When you have more than three or four agents going at once, the review step becomes the bottleneck. [Run a swarm](/docs/tutorial-conductor) is the next step: an agent that merges for you.
