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

- [x] Benchmark numbers in `RESEARCH.md`, gate passed or Hetzner decision
      recorded. Evidence: the standalone M0 benchmark was deferred by the
      owner (DECISIONS I-12) and the first real host measures itself
      instead: `RESEARCH.md` §11 "First host timings" (host-01,
      `Standard_D16s_v7`, create 20 s, freeze p99 under 0.5 s), §12 (M2:
      build 38.6 s, snapshot 24.6 s, stop 4.5 s, start 14.5 s) and §13 (M3:
      create to running 47 s, menu apply 5 s, eval and build timings). No
      axis came close to the 20 percent question, so the Hetzner fallback
      (R3-20) stays a fallback; I-39 records the v7 sizes actually used.
- [ ] A second human has completed login, run, attach, stop, start, secrets,
      config apply, snapshot restore, destroy on their own laptop.
- [x] Tenant isolation verified on a shared host: guest A cannot ping,
      ARP, or port-scan guest B or the host; guest A cannot read the store's
      `.links`; guest A cannot reach 169.254.169.254; a certificate for A is
      rejected by B's sshd and by the gateway route. Evidence: `test/isolation`
      on host-01 with two tenants, 2026-09-21 00:37Z and 00:39Z
      (`ops/checks/out/isolation-go-20260921T003759Z.txt`, `...003951Z.txt`:
      18 PASS, 1 SKIP by design), rows listed in `workstreams/14-security.md`
      §9; the gateway half of the certificate row is
      `TestCertificateForACannotOpenBAtGateway`, the sshd half is the guest
      base's `AuthorizedPrincipalsFile` (02's VM test) since putting an
      operator key on the host to try it directly was declined. The
      mechanisms as deployed are re-read in
      `security/review-2026-09-21.md` "Verified as deployed".
- [ ] A user Nix fragment that (a) has a syntax error, (b) references a
      missing attribute, (c) runs 31 minutes, (d) exceeds the closure cap,
      (e) uses `builtins.fetchurl` to an arbitrary URL each produces the
      documented error and nothing else happens. Four of five closed on
      host-01 through the deployed api and the CLI (`workstreams/
      12-nix-config-pipeline.md` §9 first row for the transcripts): (a)
      `config error: syntax error at syntax.nix:1:34, unexpected ';'`
      (M5 session, 2026-09-21 02:29Z), (b) `attribute 'ripgrepp' missing
      at missing.nix:1:36 (did you mean ...)`, (e) `eval-time fetch not
      allowed at fetch.nix:1:33; use pkgs.fetchurl { url = ...; hash =
      ...; }`, (d) `closure is 26.6 GB, limit is 20 GB; largest paths:`
      with ten paths (M3 session, 2026-09-21 00:47Z); each exit 10 and
      the guest untouched. (c) waits: `ops/checks/menu.sh
      --with-build-timeout` takes 30 minutes of host-01 by design and is
      held until the M3 kernel sweep is off the host.
- [ ] Postgres backup and restore are configured in the owner's Coolify
      (Backups tab); nothing here. Evidence: the schedule exists there
      (DECISIONS I-112).
- [ ] Snapshot restore of a guest onto a *different* host rehearsed.
- [ ] Host loss rehearsed: deallocate a host, restore its projects elsewhere
      from Blob, users notified.
- [ ] Stripe: test-mode invoice for the fixed usage pattern matches to the
      cent; live-mode charge of the owner's own card succeeded; failed
      payment path exercised with a Stripe test card.
- [x] Grafana dashboards exist for: host capacity, per-guest resources,
      builds (duration, failures), gateway (sessions, auth failures),
      snapshots (age per project), billing (usage per hour), abuse (top
      processes by CPU across fleet, top egress). Evidence: all seven
      load into a real Grafana 12.4.0 with no provisioning error
      (`ops/check.sh --grafana`), and 38 of their 43 Prometheus panel
      queries return real production data
      (`ops/dashboards/validate.py --query`); the five that do not are
      two Stripe panels (off by I-16), two build-failure panels with no
      failure yet, and `repose_host_guests`, which hostd registers and
      never populates (`10-observability.md` §9).
- [x] Alerts wired: host memory 80 percent, host unreachable, snapshot older
      than 36 hours for a running project, build queue stuck, gateway auth
      failure spike, egress over 1 TB per project per day. Evidence: 17
      rules (those six plus I-56's two and billing's three and the rest),
      each with a `promtool test rules` case and a RUNBOOK heading of its
      own — all three checks run by `ops/check.sh`. Loaded against real
      production series they evaluate healthy: none firing, none in
      error. *Wired* here means loaded and tested, not yet delivering:
      routing them to the owner's Alertmanager is part of the WireGuard
      peer that `10-observability.md` §9 still has open.
- [x] Privacy policy and terms published, containing the process-sample
      boundary verbatim and the Anthropic hosted-use statement (users
      authenticate with their own credentials; the platform stores none).
      Evidence: `https://repose.herakraft.co/privacy` and `/terms` (the
      m3-web live Playwright suite asserts both passages in a real browser,
      11/11 green 2026-09-20); `test/isolation` `TestPolicyTextContainsThe
      RequiredPassages` pins the source; since the M5 review both routes
      are prerendered so the passages are in the served HTML (`curl -s
      https://repose.herakraft.co/privacy | tr -s '[:space:]' ' ' | grep -c
      'We sample the processes'` is 1 after the web roll; the built
      `build/prerendered/privacy.html` carried it at 2026-09-21 02:18Z).
- [ ] The Anthropic API key leaked in commit `b1a5915` has been rotated
      (done 2026-09-17) and the history has been rewritten or the repo made
      private before it is shared with contributors.
- [ ] `repose --version` prints a version, and `curl -fsSL
      https://repose.herakraft.co/install.sh | sh` installs it on macOS
      arm64, macOS x86_64, Linux x86_64, Linux arm64.
- [x] `ops/RUNBOOK.md` has entries for every alert above. Evidence:
      `ops/check.sh` fails when an alert in `ops/alerts.yaml` has no
      RUNBOOK heading and passes on `main` (17 rules, 17 headings,
      2026-09-21); HostMemory80, HostUnreachable, SnapshotStale,
      BuildQueueStuck, GatewayAuthSpike and EgressHigh are the six the
      row names.
