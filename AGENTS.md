# Working in this repo

repose is a multi-tenant service that gives each project a persistent NixOS
microVM on a shared host where coding agents keep running after the laptop
closes. The design is in `docs/DESIGN.md`; every settled decision and why is in
`docs/DECISIONS.md`; the component map is `docs/ARCHITECTURE.md`; the order of
work and its gates are `docs/MILESTONES.md`; the parallel split is
`docs/workstreams/`; contracts between components are `docs/interfaces/`;
user-visible behaviour is `docs/features/`; the global definition of done is
`docs/CHECKLIST.md`. Read `docs/README.md` first, then the doc for what you are
touching. The code follows the docs, not the other way round.

## The nix work is for remote environments

Never run a direct install of anything from `nix/` or `packages/core` on this
machine. Host, edge and guest configurations are built here and deployed
there. The dev shell (`nix develop`) is the only thing meant to run locally.
This box is an Azure AMD size; hosts must be Intel (`docs/DESIGN.md` §4), so
nothing measured here says anything about a host.

## Docs are the spec, and the spec changes through DECISIONS.md

When code and a doc disagree, fix the code, unless a `docs/DECISIONS.md`
entry under "Made during implementation" records why the doc is wrong. A
change to any file in `docs/interfaces/` ships in the same commit as the code
that changes the contract, and the old shape stays accepted for one release.
A doc edit without a decision entry is how two agents build against different
contracts and only find out at integration.

## Claim before you build, record when you stop

Add a line to `docs/workstreams/STATUS.md` before starting a workstream and
update it when you stop, with what is done and what is not. Sequential
sessions read that file first; without it, the next session repeats or
undoes work.

## A passing build is not a passing grade

Every workstream doc ends with a checklist whose items name the evidence that
closes them. Close an item with that evidence, pasted in the PR or commit
message, never with "tests pass". The failure this prevents is the workstream
that is 99 percent done: the migration written but not registered, the
systemd unit present but not enabled, the error path that panics, the
command that exists but prints "not implemented". `docs/CHECKLIST.md` lists
the greps that catch these; run them before saying done.

## Ask before creating a branch

Work on `main` unless told otherwise. Creating or switching to another branch
needs the user's approval first.

## Names are fixed

The product, CLI binary, SSH login prefix, config directory, Go module path
and systemd unit prefix are all `repose`. Guest user is `dev`. Size classes
are `small`, `large`, `xl`. Guest states, error codes and id formats are
listed in `docs/interfaces/README.md`. Do not introduce a synonym for any of
them; a second name for the same thing is how a grep misses half the uses.

## Never log what a tenant typed

Log fields may carry ids, states, sizes, durations, error codes and process
names. They never carry prompts, terminal contents, process arguments,
environment variables, file paths inside a guest, secret values, tokens,
certificate bodies, or user email. `docs/ops/OBSERVABILITY.md` has the full
list. The privacy policy promises this in the same words, so a log line that
breaks it is a broken promise, not a style issue.

## Secrets have three homes and nothing else

Named secrets live as ciphertext in Postgres and as files on a tmpfs in the
guest. Tool logins the laptop already has are copied into the guest over SSH
and never touch the API. Claude Code credentials are never copied anywhere;
the user logs in inside the guest. `docs/features/secrets.md` explains why
each of these is where it is. Do not add a fourth place.

## Every host and guest operation is idempotent and reconciles

hostd may restart at any time; the api may resend any command. A command
with a command_id already seen returns the stored result. Guest state is
reconciled from `systemctl list-units 'guest@*'`, LVM and the bbolt state at
start, and reported in `Hello`. Code that assumes a clean start is code that
loses a tenant's guest after a hostd upgrade.
