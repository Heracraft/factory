# Definition of done

This is the global checklist. Every workstream doc ends with its own list; this
one applies to all of them and to the release. An item is closed by the
evidence named in it, not by a passing build. "It compiles" closes nothing. "I
ran it once on my machine" closes nothing that says "on a host" or "in a
guest".

The failure this prevents: a workstream that is 99 percent done with the last
percent being the thing that makes it usable (the migration that was never
written, the systemd unit that is not enabled, the error path that panics, the
README that says "TODO"). Each of these has happened to someone; the list is
written so they cannot happen quietly.

## For every change

- [ ] The change matches `DESIGN.md` and the workstream doc, or a
      `DECISIONS.md` entry under "Made during implementation" explains the
      deviation. Evidence: the entry exists, with alternatives.
- [ ] Every interface the change touches is updated in `interfaces/` in the
      same commit. Evidence: `git show --stat` includes the interface doc.
- [ ] No hardcoded values that belong in config: hostnames, IPs, ports, sizes,
      prices, paths outside the documented conventions. Evidence: `rg` for the
      literal returns only the config definition.
- [ ] Errors are handled at every call site, with the failure named in the
      message. No `_ = err`, no bare `panic` outside `main`. Evidence: `rg
      '_ = err|panic\(' cmd internal` is empty or each hit has a comment
      saying why.
- [ ] Logs are structured, carry `component`, `project_id` where relevant,
      and never carry secrets, tokens, certificate bodies, prompts, terminal
      contents, process arguments, or user email. Evidence: reviewer grepped
      new log calls.
- [ ] A metric or log event exists for the new failure modes (see
      `ops/OBSERVABILITY.md`). Evidence: named in the PR description.
- [ ] Tests: unit tests for logic; an integration test against a real
      Postgres for anything touching the schema; a host-level test for
      anything in hostd that touches LVM, nftables, CH, or nix, run on a real
      host and its output pasted in the PR. Evidence: CI green plus the
      pasted output.
- [ ] `go vet`, `staticcheck`, `gofmt`, `nix flake check`, `tofu validate`
      clean.
- [ ] The workstream doc's checklist has been re-read and every item that
      the change affects has its evidence updated.

## For every workstream, before it is called done

- [ ] Every command, flag, endpoint, message, table and field named in the
      workstream doc exists with that exact name. Evidence: a script or grep
      listing each name and where it is defined, pasted in the PR.
- [ ] Every error path in the workstream doc's "Failure modes" section
      produces the documented user-visible message. Evidence: each one
      triggered deliberately, output pasted.
- [ ] The workstream runs from a clean checkout following only its doc and
      `ops/RUNBOOK.md`. Evidence: someone (or a fresh agent) did it and the
      steps they had to guess are now in the doc.
- [ ] Rollback is written: how to undo the workstream's migration, unit, or
      config without data loss. Evidence: the section exists and was
      exercised once.
- [ ] The `ops/RUNBOOK.md` has a symptom entry for each way this workstream
      breaks in production.
- [ ] Nothing is left as `TODO`, `FIXME`, `XXX`, or "not implemented" in the
      workstream's files unless a `DECISIONS.md` entry defers it. Evidence:
      `rg 'TODO|FIXME|XXX|not implemented' <paths>` is empty or each hit is
      referenced.

## Release (M5)

- [ ] Benchmark numbers in `RESEARCH.md`, gate passed or Hetzner decision
      recorded.
- [ ] A second human has completed login, run, attach, stop, start, secrets,
      config apply, snapshot restore, destroy on their own laptop.
- [ ] Tenant isolation verified on a shared host: guest A cannot ping,
      ARP, or port-scan guest B or the host; guest A cannot read the store's
      `.links`; guest A cannot reach 169.254.169.254; a certificate for A is
      rejected by B's sshd and by the gateway route.
- [ ] A user Nix fragment that (a) has a syntax error, (b) references a
      missing attribute, (c) runs 31 minutes, (d) exceeds the closure cap,
      (e) uses `builtins.fetchurl` to an arbitrary URL each produces the
      documented error and nothing else happens.
- [ ] Postgres restore rehearsed from a real backup, on a scratch Coolify
      or a throwaway database, with the time recorded. The dump comes from
      the Postgres service's Backups tab; the destination is the owner's
      own S3 storage and nothing here holds a credential for it
      (DECISIONS I-103). `ops/restore-rehearsal.sh <dump>` does it and
      prints the timings.
- [ ] Snapshot restore of a guest onto a *different* host rehearsed.
- [ ] Host loss rehearsed: deallocate a host, restore its projects elsewhere
      from Blob, users notified.
- [ ] Stripe: test-mode invoice for the fixed usage pattern matches to the
      cent; live-mode charge of the owner's own card succeeded; failed
      payment path exercised with a Stripe test card.
- [ ] Grafana dashboards exist for: host capacity, per-guest resources,
      builds (duration, failures), gateway (sessions, auth failures),
      snapshots (age per project), billing (usage per hour), abuse (top
      processes by CPU across fleet, top egress).
- [ ] Alerts wired: host memory 80 percent, host unreachable, snapshot older
      than 36 hours for a running project, build queue stuck, gateway auth
      failure spike, egress over 1 TB per project per day.
- [ ] Privacy policy and terms published, containing the process-sample
      boundary verbatim and the Anthropic hosted-use statement (users
      authenticate with their own credentials; the platform stores none).
- [ ] The Anthropic API key leaked in commit `b1a5915` has been rotated
      (done 2026-09-17) and the history has been rewritten or the repo made
      private before it is shared with contributors.
- [ ] `repose --version` prints a version, and `curl -fsSL
      https://repose.herakraft.co/install.sh | sh` installs it on macOS
      arm64, macOS x86_64, Linux x86_64, Linux arm64.
- [ ] `ops/RUNBOOK.md` has entries for every alert above.
