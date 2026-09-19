# Workstream status

One line per claim. Newest at the bottom. Format:
`YYYY-MM-DD | <workstream> | <who> | claimed|progress|done|blocked | <note>`

2026-09-17 | docs | interview session | done | design, decisions, interfaces, workstreams written
2026-09-17 | scaffold | interview session | done | go.mod, cmd stubs, proto contracts, buf config, nix flake skeleton (flake check passes), infra skeleton, justfile, AGENTS.md
2026-09-17 | 00-benchmark | deferred | | owner skipped it (DECISIONS I-12); first M1 host records timings instead
2026-09-19 | 02-guest-base | claude session (ws/guest-base worktree) | claimed | nix/guest/base modules, overlay wrappers and hooks, mkGuestRunner, VM tests, retire packages/core/flake.nix
2026-09-19 | 02-guest-base | claude session (ws/guest-base worktree) | done | nix/guest/base (17 modules), nix/overlay/agents wrappers+hooks+MCP, mkGuestRunner (bin/run with run-time guest args, I-19), VM tests guest-base/guest-docker/guest-desktop + closure check pass, runner booted under Cloud Hypervisor on the dev box with SSH-cert login, docker, desktop; packages/core retired. Not done: hooks.sock end-to-end and hostd AgentEvent (needs 04 guestd), IMDS/guest-to-guest blocking (host nftables, 01/03), CI wiring; gemini-cli is marked for removal in nixpkgs (12 decides).
