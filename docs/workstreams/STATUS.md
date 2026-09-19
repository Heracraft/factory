# Workstream status

One line per claim. Newest at the bottom. Format:
`YYYY-MM-DD | <workstream> | <who> | claimed|progress|done|blocked | <note>`

2026-09-17 | docs | interview session | done | design, decisions, interfaces, workstreams written
2026-09-17 | scaffold | interview session | done | go.mod, cmd stubs, proto contracts, buf config, nix flake skeleton (flake check passes), infra skeleton, justfile, AGENTS.md
2026-09-17 | 00-benchmark | deferred | | owner skipped it (DECISIONS I-12); first M1 host records timings instead
2026-09-19 | 04-guestd | opus-5 session (ws/04-guestd) | claimed | vsock server, all requests, notifies, hook socket, repose-hook, fakes, VM test
2026-09-19 | 04-guestd | opus-5 session (ws/04-guestd) | done | cmd/guestd (+ `call` client), cmd/repose-hook, internal/vsockrpc, internal/guestd/*, internal/fakes/guestd, nix packages.guestd and the NixOS VM test. `nix flake check ./nix` green, `go test -race` green, lint clean. Interfaces: vsock-guestd.md; DECISIONS I-18..I-21. NOT done: the real-guest item (needs a host and hostd, workstream 03); `mkGuest`/guest base installation of the unit is 02's.
