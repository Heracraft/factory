# Workstream status

One line per claim. Newest at the bottom. Format:
`YYYY-MM-DD | <workstream> | <who> | claimed|progress|done|blocked | <note>`

2026-09-17 | docs | interview session | done | design, decisions, interfaces, workstreams written
2026-09-17 | scaffold | interview session | done | go.mod, cmd stubs, proto contracts, buf config, nix flake skeleton (flake check passes), infra skeleton, justfile, AGENTS.md
2026-09-17 | 00-benchmark | deferred | | owner skipped it (DECISIONS I-12); first M1 host records timings instead
2026-09-19 | 01-host-nixos | ws/01-host-nixos agent (Fable 5.1) | claimed | nix/hosts/: disko, kernel, network, nftables, storage, virt, hostd unit + stub, gc, observability, registration, ssh, hardening, azure, VM tests; flake check and host toplevel build locally; real-host items marked
2026-09-19 | 01-host-nixos | ws/01-host-nixos agent (Fable 5.1) | done (local) | nix/hosts complete: nixosConfigurations.host and host-bench build, nix flake check passes with 3 VM tests (network isolation, thin pool, services/registration/ssh audit); host-conventions.md rewritten, DECISIONS I-18, runbook entries. Not done: every real-host item (nixos-anywhere, lsblk on Azure, Grafana view, ss on the Azure NIC) and CI (no CI exists yet); buf lint fails on the untouched proto scaffold. hostd is the stub until 03 merges packages.hostd.
