# Azure and external accounts: what to do before agents start

Plain-English checklist. Everything here is a one-time human action that
OpenTofu cannot do for you, or that agents should not be trusted to do with
your money. Do them in order; most take minutes, the quota request can take
a day.

## In Azure

1. **Pick the subscription and confirm the credits are on it.** Note the
   credit expiry date; put it in your calendar a month early. Everything
   below goes into this subscription.

2. **Request vCPU quota in East US.** Portal: Quotas → Compute → filter
   region East US. Request:
   - `Standard DSv5 Family vCPUs`: 64 for now (the pre-launch host is a
     `D16s_v5` at 16, the Coolify VM 4, the edge 2, and room for a `D32s_v5`
     if 16 is tight). Raise it to 160 before launch for the `D64s_v5`.
   - `Total Regional vCPUs`: 100 for now.
   Checked 2026-09-19: the subscription already has 65 on both lines, so no
   request is needed until launch. Approval is often automatic within minutes for
   these sizes; if it goes to a ticket it can take a day. Do this first.

3. **Register resource providers** (done 2026-09-19; once per subscription): Microsoft.Compute, Microsoft.Network, Microsoft.Storage,
   Microsoft.KeyVault, Microsoft.ManagedIdentity. Portal: Subscription →
   Resource providers → Register.

4. **Create a resource group** named `repose-prod` in East US. Done
   2026-09-19. OpenTofu will put everything in it, and deleting it later
   deletes everything.

5. **Create the OpenTofu state store by hand.** Done 2026-09-19: storage
   account `reposetfstate3912` in `repose-prod`, container `tfstate`, blob
   versioning on, Standard LRS, no public access. Backend key
   `azure.tfstate`, auth via `az login`. This account must never be managed
   by OpenTofu itself.

6. **Decide how OpenTofu authenticates.** For now: `az login` on the machine
   running it (the dev shell provides `az`). Later, for CI, a service
   principal with Contributor on `repose-prod` and Key Vault Administrator
   on the vault. Do not create a subscription-wide Owner principal.

7. **Set a budget alert.** Cost Management → Budgets: $1,000 a month for
   the pre-launch month, raised to $2,500 at launch, with emails at 50, 80
   and 100 percent. A forgotten `D16s_v7` burns $25 a day; a `D64s_v7`, about $100.

8. **Know the two settings agents must get right, so you can check them.**
   Hosts must be Intel Dsv7 (`Standard_D16s_v7` until launch, `D64s_v7` after, DECISIONS I-14 and I-39) with security type `Standard`. This subscription cannot create v5 or v6 sizes at all (verified 2026-09-20); the v7 families come with a 350 vCPU quota each, so no quota request is needed for them.
   The portal defaults to Trusted Launch, which silently disables nested
   virtualization. AMD sizes (any `a` in the size name, like this dev box's
   `D8alds_v7`) are excluded. If East US has no capacity for the size when a
   host is created, the fallback is the same size in `v6`, then another
   Intel region, never AMD.

9. **Have an SSH key for bootstrapping.** nixos-anywhere needs to SSH into
   the fresh Ubuntu VM as `azureuser` with your key. The key on this box
   or your laptop is fine; its public key goes into the OpenTofu variables.

## Outside Azure

10. **Cloudflare R2 and the API token.** The *bucket* is not a manual step:
    `infra/r2` creates `repose-pg-backups` with its 35-day lifecycle rule.
    What is manual is the credential — an API token with object read/write on
    that bucket only — because a token created by OpenTofu would sit in the
    state file in clear text for the life of the bucket (`DECISIONS.md`
    I-21). Create the token, note the account id (an identifier, not a
    secret; it goes in `infra/r2/r2.tfvars`), then:

    ```bash
    export CLOUDFLARE_API_TOKEN=...
    make -C infra apply ENV=r2
    tofu -chdir=infra/r2 output coolify_s3_destination   # the form Coolify asks for
    ```

    The same token, scoped to DNS edit on `herakraft.co`, is what
    `manage_dns = true` needs (step 11). Coolify's backup destination and the
    restore rehearsal are `docs/ops/coolify.md`.

11. **DNS for `herakraft.co`.** The zone already answers every name under it
    from a **proxied wildcard**, so the repose names resolve today — to
    Cloudflare's proxy, which carries neither SSH nor WireGuard. That makes
    `ssh.repose.herakraft.co` actively wrong rather than merely missing
    (`DECISIONS.md` I-72). Until a token is in `CLOUDFLARE_API_TOKEN` and
    `manage_dns = true`, create these four by hand as A records **with the
    proxy off**: `ssh.repose` → the edge's IP, and `repose`, `api.repose`,
    `auth.repose` → the control plane's. The table with the reasons is in
    `infra/README.md`, "DNS while manage_dns is false"; `*.repose` for
    previews is later. If Logto stays on your personal server, its hostname
    stays as it is.

12. **Logto.** In your existing Logto: create an API resource with
    identifier `https://api.repose.herakraft.co`; a Native application
    named `repose-cli` (device flow and loopback redirect
    `http://127.0.0.1:*/callback` allowed); a Single-page application named
    `repose-web` with redirect `https://repose.herakraft.co/callback`.
    Confirm the GitHub connector is enabled. Copy the app ids and the
    issuer URL; agents need them as environment variables, never in git.

13. **Stripe.** An account in test mode is enough to start. Copy the test
    secret key and set up a webhook endpoint later when the API exists.
    Live mode is a milestone M4 gate, not a prerequisite.

14. **Resend.** Verify the sending domain (`herakraft.co` or
    `repose.herakraft.co`) and copy an API key.

15. **GitHub.** The repo should be private until the leaked-key history is
    handled (see `CHECKLIST.md`). Agents need push access to `main`.

16. **Cachix.** Create a cache named `repose` at cachix.org (public; the
    agents are public binaries). Copy its public key (`repose.cachix.org-1:
    ...`) into `repose.host.overlayCache.publicKey` with the URL
    `https://repose.cachix.org` in `nix/hosts/host-01.nix` (or the generic
    host), and add the cache's auth token to the GitHub repository as the
    secret `CACHIX_AUTH_TOKEN`. CI then pushes the agent overlay on every
    push to `main` and hosts substitute the agents instead of fetching
    upstream (DECISIONS I-46). Until then builds fetch the release
    binaries themselves, which is slower, not wrong.

## What you do not need to do

- Create VMs, networks, disks, Key Vault, Blob containers: OpenTofu does
  that (workstream 11).
- Install NixOS on anything by hand: nixos-anywhere does that from the
  flake (workstream 01, 06).
- Set up Grafana: your existing Loki and Grafana are reused; workstream 10
  adds Prometheus there and needs a WireGuard peer on that server, which is
  the one manual step on your personal box.

## The benchmark you chose to skip

The M0 gate would have measured the nested-virtualization penalty before
committing to Azure hosts. Skipping it means the first M1 host is the
benchmark in practice: workstream 03's checklist records real build, Docker
and clone timings from that host into `RESEARCH.md`, and the Hetzner
fallback (`DECISIONS.md` R3-20) stays available if they are bad.
