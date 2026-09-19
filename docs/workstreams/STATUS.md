# Workstream status

One line per claim. Newest at the bottom. Format:
`YYYY-MM-DD | <workstream> | <who> | claimed|progress|done|blocked | <note>`

2026-09-17 | docs | interview session | done | design, decisions, interfaces, workstreams written
2026-09-17 | scaffold | interview session | done | go.mod, cmd stubs, proto contracts, buf config, nix flake skeleton (flake check passes), infra skeleton, justfile, AGENTS.md
2026-09-17 | 00-benchmark | deferred | | owner skipped it (DECISIONS I-12); first M1 host records timings instead
2026-09-19 | 02-guest-base | claude session (ws/guest-base worktree) | claimed | nix/guest/base modules, overlay wrappers and hooks, mkGuestRunner, VM tests, retire packages/core/flake.nix
