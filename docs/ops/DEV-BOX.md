# The dev box

`woker-1` in resource group `dev-cluster`, `Standard_D8alds_v7` (AMD, 8 vCPU,
16 GB), East US, Tailscale `100.89.156.32`. This is where docs are written,
agents run, and Go and Nix builds happen. It is **not** a Repose host: it is
an AMD size and cannot tell you anything about nested virtualization.

## The Nix store lives on the temp disk

The OS disk is 30 GB, which a swarm of agents building guest closures fills
in an afternoon. The size comes with a 440 GB local temp disk (`nvme1n1`)
that is fast and free, so `/nix` is bind-mounted from it:

```
/dev/nvme1n1  ->  /mnt/nixstore  -bind->  /nix
```

Set up by `sudo scripts/nix-store-on-tempdisk.sh`, which is idempotent.

**What "temp disk" means.** It is scratch space on the physical server the
VM is currently running on, not a disk that belongs to the VM. A reboot from
inside the VM keeps it. **Stopping or deallocating the VM from Azure, or an
Azure maintenance move, brings it back empty.** Everything in the Nix store
is a cache that rebuilds from `nix/flake.lock`, so nothing is lost except
time: expect 10 to 20 minutes of downloads to get a guest closure back.

**After a deallocate**, `nix` itself is gone (it lived in the store). Run:

```
sudo scripts/nix-store-on-tempdisk.sh
```

It reformats the blank disk, remounts, and reinstalls Nix. Then `nix develop
./nix` again. Anything in `~/.cache/nix` and `~/go` is on the OS disk and
survives.

**Do not put anything that matters on `/mnt/nixstore`** other than the
store. Worktrees, the repo, `~/.azure`, `~/.config` all stay on the OS disk.

If the box gets resized to a size without a local disk (no `d` after the
size number, e.g. `D8as_v7`), the script refuses at step 0; move `/nix` back
to the OS disk first (`umount /nix; mv /mnt/nixstore /nix` with the OS disk
grown to fit).

## Azure CLI here

`az` is not installed on the box. `nix develop ./nix` provides it, as does
`nix shell nixpkgs#azure-cli`. Login is the copied `~/.azure` from a laptop
browser login, because security defaults block device-code login from a
browserless VM. The refresh token lasts about 90 days of inactivity.

## Free space

Keep 40 GB free on `/nix` before launching a wave of agents; `df -h /nix`.
`nix-collect-garbage -d` reclaims old closures; it never touches anything
the current flake lock references once rebuilt.
