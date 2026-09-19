# Workstream status

One line per claim. Newest at the bottom. Format:
`YYYY-MM-DD | <workstream> | <who> | claimed|progress|done|blocked | <note>`

2026-09-17 | docs | interview session | done | design, decisions, interfaces, workstreams written
2026-09-17 | scaffold | interview session | done | go.mod, cmd stubs, proto contracts, buf config, nix flake skeleton (flake check passes), infra skeleton, justfile, AGENTS.md
2026-09-17 | 00-benchmark | deferred | | owner skipped it (DECISIONS I-12); first M1 host records timings instead
2026-09-19 | 03-hostd | ws/03-hostd-hostdev agent (Fable 5.1) | claimed | cmd/hostd, internal/hostd/*, cmd/hostdev (I-17), internal/fakes/{hostd,guestd}, buf-generated proto; host-level checklist items need a real host and are marked
