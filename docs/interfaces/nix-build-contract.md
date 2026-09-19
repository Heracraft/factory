# Nix build contract: hostd ⇄ the platform flake

What hostd runs when it receives `Build` (docs/interfaces/grpc-hostd.md),
and what the platform flake in `nix/` must expose for it. Workstream 03
invokes; workstream 12 authors the Nix. DECISIONS I-20.

## The flake

The repository's `nix/flake.nix`, checked out at
`/var/lib/repose/base/<base_ref>/` (hostd clones the repository with
`--base-repo-url` when the checkout is missing; `base_ref` is a git
revision), declares:

```nix
inputs.fragment = { url = "path:./guest/fragment-placeholder"; flake = false; };
outputs = { self, nixpkgs, home-manager, microvm, fragment, ... }: {
  # A NixOS system: base module + home-manager fragment at
  # "${fragment}/fragment.nix" applied to user dev. Only
  # config.system.build.toplevel is required.
  guestSystem = <nixosSystem>;
};
```

The placeholder input exists so the lock file is valid; hostd always
overrides it.

## What hostd runs

For a `Build` with `revision_id` R, `fragment` F and `limits` L, in order:

1. Writes F to `/var/lib/repose/builds/R/fragment.nix` (0600).
2. Evaluates:

   ```
   timeout <L.eval_s> nix eval --raw --no-write-lock-file \
     --option restrict-eval true --option allow-import-from-derivation false \
     --option pure-eval true --option eval-cache false --max-call-depth 10000 \
     --override-input fragment path:/var/lib/repose/builds/R \
     path:/var/lib/repose/base/<base_ref>/nix#guestSystem.config.system.build.toplevel.drvPath
   ```

   Exit 124 is `eval_failed` "evaluation exceeded <eval_s> s". Any other
   failure is `eval_failed` with the summary line, `fragment_line` parsed
   from the first `fragment.nix:L:C` after the last `error:` line, and the
   verbatim stderr (capped 32 KB) after a blank line.
3. Builds the derivation:

   ```
   systemd-run --scope -p CPUQuota=<L.cores*100>% -p MemoryMax=16G -- \
   timeout <L.build_s> nix build --no-link --print-out-paths --print-build-logs \
     --option sandbox true --max-jobs 1 --cores <L.cores> \
     --option substituters "https://cache.nixos.org https://cache.repose.herakraft.co" \
     <drvPath>^*
   ```

   Every stderr line is streamed as `BuildLog`. Exit 124 is
   `build_timeout` "build exceeded <build_s> s; last derivation: <name>".
   Any other failure is `build_failed` with "build of <drv> failed" and
   the verbatim tail.
4. `nix path-info -S <out>`; over `L.closure_bytes` is `closure_too_large`
   with the ten largest paths from `nix path-info -rS`, and no GC root.
5. Registers `/nix/var/nix/gcroots/repose/rev-<project_id>-<revision_id>`
   and keeps the newest three per project.
6. Reads `<out>/kernel` and `<out>/initrd` targets for `kernel_changed`.

The attribute path after `#` is the hostd flag `--eval-attr`
(default `guestSystem.config.system.build.toplevel.drvPath`), so 12 can
rename the output without a hostd change.

## What the built closure must contain

hostd boots the guest from the closure directly (DECISIONS I-19): it needs
`<out>/kernel`, `<out>/initrd`, `<out>/init` and `<out>/kernel-params`,
which every NixOS toplevel has. Anything microvm.nix needs on the kernel
command line must be in `boot.kernelParams`; hostd appends `init=`,
`console=ttyS0` and `ip=<guest>::<gateway>:<netmask>::eth0:off`.

## Fixtures

Real Nix output for the error mapping lives in
`internal/hostd/nixbuild/testdata/`, produced by exactly the eval command
above against a minimal flake with the `fragment` input.
