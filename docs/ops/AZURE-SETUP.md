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

10. **Cloudflare R2.** Create a bucket `repose-pg-backups` and an API token
    with object read/write on that bucket only. Coolify's Postgres backups
    go here. Note the account id, access key, secret and endpoint.

11. **DNS for `herakraft.co`.** Nothing to create yet, but confirm you can
    add records. Agents will need these once IPs exist:
    `repose` (dashboard), `api.repose`, `ssh.repose`, and later
    `*.repose` for previews. If Logto stays on your personal server, its
    hostname stays as it is.

12. **Logto.** In your existing Logto: create an API resource with
    identifier `https://api.repose.herakraft.co`; a Native application
    named `repose-cli` (device flow and loopback redirect
    `http://127.0.0.1:*/callback` allowed); a Single-page application named
    `repose-web` with redirect `https://repose.herakraft.co/callback`.
    Confirm the GitHub connector is enabled. Copy the app ids and the
    issuer URL; agents need them as environment variables, never in git.

13. **Stripe.** An account in test mode is enough to start. Copy the test
    secret key. The objects the api needs are step 17, once there is a
    hostname to point a webhook at. Live mode is a milestone M4 gate, not a
    prerequisite.

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

17. **Stripe objects and the webhook** (after the api has a hostname; do it
    in test mode first and repeat in live mode before launch). In the Stripe
    dashboard, or with the CLI:

    - One **product**, `repose`.
    - Three **billing meters**, event names `repose_compute_cents`,
      `repose_storage_cents`, `repose_egress_cents`, each aggregating
      `sum` over the payload key `value`, keyed on `stripe_customer_id`.
      Note each meter's `mtr_...` id as well as its event name.
    - Three **prices** on the product, one per meter, USD, recurring
      monthly, usage-based, **$0.01 per unit** — the unit is one cent,
      because the platform computes the amounts and Stripe adds nothing of
      its own (DECISIONS I-61).
    - A **webhook endpoint** at
      `https://api.repose.herakraft.co/v1/billing/webhook` subscribed to
      `invoice.paid`, `invoice.payment_failed`,
      `customer.subscription.deleted`, `setup_intent.succeeded`,
      `payment_method.detached` and `charge.refunded`. **Create it with the
      API version the deployed `stripe-go` pins** (`2025-10-29.clover`
      today; `go doc github.com/stripe/stripe-go/v83.APIVersion` prints the
      current one). An endpoint on another version has every delivery
      rejected as a version mismatch, which looks like a silent billing
      outage. Copy the signing secret.
    - **Stripe Tax** on, and the customer address marked required on the
      card form, so invoices carry a tax line (09-billing.md §5.9).
    - The **customer portal** configured to allow updating the payment
      method and the billing address.

    Then set, in Coolify, the variables `ops/coolify/api.env.example`
    lists under "Billing": `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`,
    `STRIPE_PRICE_{COMPUTE,STORAGE,EGRESS}` and
    `STRIPE_METER_ID_{COMPUTE,STORAGE,EGRESS}`. Until `STRIPE_SECRET_KEY`
    is set the api runs normally and the billing routes answer
    `503 billing_disabled` (DECISIONS I-16); with it set but the webhook
    secret or the prices missing, the api refuses to start rather than
    billing nothing quietly.

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
