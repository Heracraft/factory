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
   Defaults are usually 10 to 20 per family, so a host cannot be created
   until this is approved. Approval is often automatic within minutes for
   these sizes; if it goes to a ticket it can take a day. Do this first.

3. **Register resource providers** (once per subscription; harmless if
   already done): Microsoft.Compute, Microsoft.Network, Microsoft.Storage,
   Microsoft.KeyVault, Microsoft.ManagedIdentity. Portal: Subscription →
   Resource providers → Register.

4. **Create a resource group** named `factory-prod` in East US. OpenTofu
   will put everything in it, and deleting it later deletes everything.

5. **Create the OpenTofu state store by hand.** A storage account named
   `factorytfstate` plus a random suffix (names are global), a container
   named `tfstate`. Standard LRS is fine. This is the one thing that must
   exist before the first `tofu init`, and it must never be managed by
   OpenTofu itself.

6. **Decide how OpenTofu authenticates.** For now: `az login` on the machine
   running it (the dev shell provides `az`). Later, for CI, a service
   principal with Contributor on `factory-prod` and Key Vault Administrator
   on the vault. Do not create a subscription-wide Owner principal.

7. **Set a budget alert.** Cost Management → Budgets: $1,000 a month for
   the pre-launch month, raised to $2,500 at launch, with emails at 50, 80
   and 100 percent. A forgotten `D16s_v5` burns $18 a day; a `D64s_v5`, $74.

8. **Know the two settings agents must get right, so you can check them.**
   Hosts must be Intel Dsv5 (`Standard_D16s_v5` until launch, `D64s_v5` after, DECISIONS I-14) with security type `Standard`.
   The portal defaults to Trusted Launch, which silently disables nested
   virtualization. AMD sizes (any `a` in the size name, like this dev box's
   `D8alds_v7`) are excluded. If East US has no capacity for the size when a
   host is created, the fallback is the same size in `v6`, then another
   Intel region, never AMD.

9. **Have an SSH key for bootstrapping.** nixos-anywhere needs to SSH into
   the fresh Ubuntu VM as `azureuser` with your key. The key on this box
   or your laptop is fine; its public key goes into the OpenTofu variables.

## Outside Azure

10. **Cloudflare R2.** Create a bucket `factory-pg-backups` and an API token
    with object read/write on that bucket only. Coolify's Postgres backups
    go here. Note the account id, access key, secret and endpoint.

11. **DNS for `herakraft.co`.** Nothing to create yet, but confirm you can
    add records. Agents will need these once IPs exist:
    `factory` (dashboard), `api.factory`, `ssh.factory`, and later
    `*.factory` for previews. If Logto stays on your personal server, its
    hostname stays as it is.

12. **Logto.** In your existing Logto: create an API resource with
    identifier `https://api.factory.herakraft.co`; a Native application
    named `factory-cli` (device flow and loopback redirect
    `http://127.0.0.1:*/callback` allowed); a Single-page application named
    `factory-web` with redirect `https://factory.herakraft.co/callback`.
    Confirm the GitHub connector is enabled. Copy the app ids and the
    issuer URL; agents need them as environment variables, never in git.

13. **Stripe.** An account in test mode is enough to start. Copy the test
    secret key and set up a webhook endpoint later when the API exists.
    Live mode is a milestone M4 gate, not a prerequisite.

14. **Resend.** Verify the sending domain (`herakraft.co` or
    `factory.herakraft.co`) and copy an API key.

15. **GitHub.** The repo should be private until the leaked-key history is
    handled (see `CHECKLIST.md`). Agents need push access to `main`.

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
