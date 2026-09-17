# Pricing

Billed from the first hour through Stripe. Card on file before the first
guest starts. The shape is hourly with a monthly cap per project, so a
person who never stops their guest pays a flat fee and a person who stops it
at night pays less, from one rule (DECISIONS R4-7).

## Tiers

| Class | vCPU | RAM | Default volume | Hourly | Monthly cap |
|---|---|---|---|---|---|
| small | 2 | 4 GB | 20 GB | $0.07 | $49 |
| large | 4 | 8 GB | 40 GB | $0.14 | $99 |
| xl | 8 | 16 GB | 80 GB | $0.28 | $199 |

Hourly is the cap divided by 720 rounded up to the cent, so 720 running
hours in a month cost exactly the cap and never more. Guest-hours accrue per
minute of `running`, rounded up to the minute. A stopped project accrues no
guest-hours.

On top, for every project in any state until destroyed:

- Storage: $0.10 per GB-month of *allocated* volume size, prorated hourly.
  A 40 GB large volume is $4.00 a month whether it holds 2 GB or 39 GB.
  Allocated rather than used because thin provisioning means the platform
  has reserved that space and the user chose the size.
- Egress: 500 GB a month included per project, then $0.05 per GB. Ingress
  is free. Egress counts bytes leaving the guest to the internet, measured
  by the per-guest nftables counter; traffic to the gateway (SSH, noVNC,
  hooks) is not counted.

Example: one large guest running all month with the default volume and
10 GB egress is $99 + $4 + $0 = $103. The same guest stopped every night
for 10 hours is about $58 + $4 = $62.

## Trial

A new account gets $10 of credit, consumed at the rates above, card
required to start the first guest anyway. Ten dollars is about 70 large
guest-hours, enough to leave an agent running overnight twice and see the
result. When the credit reaches zero, the next hour is charged to the card;
nothing stops. The trial is a credit rather than free days because a credit
meters exactly like paid usage, so the trial is also the first test of the
meters (DECISIONS R4-8).

## Limits

Until the first paid invoice: 3 projects, at most 1 xl. After: 10 projects.
More on request, manually, because a stranger with free compute is the
abuse vector and a paying customer with a history is not.

## Invoicing

Monthly through Stripe Billing. Usage records are pushed hourly from the
`usage_hours` rollup, so the Stripe invoice and the platform's own numbers
agree to the cent, and the release checklist proves it with a fixed usage
pattern. A failed payment: reminders on days 1 and 2; guests stopped on day
3; nothing destroyed for 30 days; account suspended at 30 days with
snapshots kept another 30.

## The cost floor

The prices above have to cover an always-on guest on the chosen host. On an
Azure `D64s_v5` at $3.07 an hour on demand (about $2,240 a month), memory
is the binding constraint at 30 large guests per host (256 GB minus 16 GB
reserve, no memory oversubscription; CPU is oversubscribed 2:1). That gives
this floor per always-on guest, compute only:

| Class | D64s_v5 on demand | D64s_v5 1-year reserved ($1.89/h) | Hetzner AX162-R (about €230/mo, 256 GB) |
|---|---|---|---|
| small | ~$37 | ~$23 | ~€4 |
| large | ~$75 | ~$46 | ~€8 |
| xl | ~$150 | ~$92 | ~€15 |

Storage on Premium SSD v2 adds roughly $0.08 per GB-month at the disk
level, so the $0.10 storage price is close to cost. Egress on Azure is about
$0.087 per GB after the free allowance, so the included 500 GB is a real
cost the tier absorbs, and $0.05 per GB beyond it is below cost; the
expectation is that almost nobody exceeds it and the ones who do are worth
watching for other reasons.

Read across the table: on Azure on demand, a large at $99 clears its $75
floor by a third and a small at $49 clears $37 by the same, before storage,
support, the control plane, and the edge. That is thin. With reservations
it is comfortable. On Hetzner metal it is a different business, which is
why the host module is written so a second provider is a module and not a
rewrite (DECISIONS R3-20), and why credits are being spent on Azure while
the product is learned rather than on margin.

Nothing in the tier prices is tied to Azure. If hosts move, prices stay and
margin changes.

## What is deliberately not priced

- Snapshots: included. They are small (used blocks, compressed) and the
  retention is fixed.
- Builds: included, capped by time and cores instead.
- Notifications, the gateway, the dashboard: included.
- Support: none promised in the first release beyond email.

## Changing prices

Prices live in one place in the API's configuration and in this doc. A
change is a `DECISIONS.md` entry, a 30-day notice to users by email, and a
new `price_version` on `usage_hours` rows from the effective hour so
historical usage keeps its historical price.
