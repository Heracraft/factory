# Workstream status

One line per claim. Newest at the bottom. Format:
`YYYY-MM-DD | <workstream> | <who> | claimed|progress|done|blocked | <note>`

2026-09-17 | docs | interview session | done | design, decisions, interfaces, workstreams written
2026-09-17 | scaffold | interview session | done | go.mod, cmd stubs, proto contracts, buf config, nix flake skeleton (flake check passes), infra skeleton, justfile, AGENTS.md
2026-09-17 | 00-benchmark | deferred | | owner skipped it (DECISIONS I-12); first M1 host records timings instead
2026-09-19 | 03-hostd | ws/03-hostd-hostdev agent (Fable 5.1) | claimed | cmd/hostd, internal/hostd/*, cmd/hostdev (I-17), internal/fakes/{hostd,guestd}, buf-generated proto; host-level checklist items need a real host and are marked
2026-09-19 | 03-hostd | ws/03-hostd-hostdev agent (Fable 5.1) | progress | done without a host: cmd/hostd (register, mTLS stream with reconnect/replay/sample buffering, every command against LVM/nft/tc/systemd-run/CH API/virtiofsd/nix behind interfaces with fakes, bbolt state, reconcile, GC roots, samples, console capture, metrics, control socket, operator subcommands), cmd/hostdev (I-17), internal/fakes/{hostd,guestd}, internal/vsockrpc, proto I-18 fields, nix packages hostd/hostdev, decisions I-18..I-20, nix-build-contract.md, runbook rows. NOT done (needs a real Intel host): every host-level checklist item in 03-hostd.md §9 (real CreateGuest, isolation curls, blob restore diff, freeze p99, nix-collect-garbage, console rotation in Loki, chaos kill -9 mid-command on a host, RESEARCH.md timings). Fluent Bit config is 10's; the api-side audit_log for `hostd audit-login` is 05's.
