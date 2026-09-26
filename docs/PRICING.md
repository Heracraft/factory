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
for 10 hours is about $59 + $4 = $63 (14 hours × 30 days × $0.14 = $58.80).

## Trial

A new account's first day of compute is on us (DECISIONS I-205, amending
R4-8): a credit of one day on large, 24 guest-hours at the large rate,
which is also 48 hours on small, consumed at the rates above. A card is
still required to start the first guest. When the credit reaches zero, the
next hour is charged to the card; nothing stops. It stays a credit rather
than free hours of one class, so there is one meter, and the trial is also
the first test of the meters. Everything a user reads calls it "your first
day of compute" and never names an amount: the dashboard shows the time
left ("24 hours left on large (48 on small)").

## Limits

Until the first paid invoice: 3 projects, at most 1 xl. After: 10 projects.
More on request, manually, because a stranger with free compute is the
abuse vector and a paying customer with a history is not.

## Invoicing

Monthly through Stripe Billing. Usage records are pushed hourly from the
`usage_hours` rollup, so the Stripe invoice and the platform's own numbers
agree to the cent, and the release checklist proves it with a fixed usage
pattern. A failed payment: reminders on days 1 and 2; on day 3 guests are
snapshotted and stopped and the account is suspended; the snapshots are
deleted on day 33, 30 days after suspension (DECISIONS R4-11,
`docs/features/pricing.md` "A failed payment").

## The cost floor

The prices above have to cover an always-on guest on the chosen host.
Memory is the binding constraint at 30 large guests per host (256 GB minus
16 GB reserve, no memory oversubscription; CPU is oversubscribed 2:1). Hosts
are `D64s_v7` (DECISIONS I-39): this subscription cannot create v5 or v6
sizes. No `D64s_v7` price has been looked up; the v7 column below is four
times the `D16s_v7` retail price in `docs/RESEARCH.md` §2a ($1.058 an hour),
so about $4.23 an hour, or $3,090 a month. The v5 columns are the list
prices this table was first written with ($3.07 an hour on demand, about
$2,240 a month), kept because I-39 treats them as the economics of a v5
reservation or Hetzner. The floor per always-on guest, compute only:

| Class | D64s_v7 on demand (derived) | D64s_v5 on demand | D64s_v5 1-year reserved ($1.89/h) | Hetzner AX162-R (about €230/mo, 256 GB) |
|---|---|---|---|---|
| small | ~$51 | ~$37 | ~$23 | ~€4 |
| large | ~$103 | ~$75 | ~$46 | ~€8 |
| xl | ~$206 | ~$150 | ~$92 | ~€15 |

Storage on Premium SSD v2 adds roughly $0.08 per GB-month at the disk
level, so the $0.10 storage price is close to cost. Egress on Azure is about
$0.087 per GB after the free allowance, so the included 500 GB is a real
cost the tier absorbs, and $0.05 per GB beyond it is below cost; the
expectation is that almost nobody exceeds it and the ones who do are worth
watching for other reasons.

Read across the table: on the v7 hosts actually running, on demand, a
full host of large guests at $99 does not cover its $103 floor, and a small
at $49 does not cover $51, before storage, support, the control plane, and
the edge. On v5 on demand, a large clears $75 by a third; with a v5
reservation it is comfortable. A v7 reservation price has not been looked
up. On Hetzner metal it is a different business, which is
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
