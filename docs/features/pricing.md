# Billing and the trial

What a user sees about money: the card they add before the first guest
starts, the $10 that runs out, the number in `repose status`, the invoice at
the end of the month, and what happens when a payment fails. The prices
themselves are in [`../PRICING.md`](../PRICING.md); the implementation is
[`../workstreams/09-billing.md`](../workstreams/09-billing.md).

## Adding a card

```
$ repose run
error: add a card before starting a guest
       https://repose.herakraft.co/billing
```

The dashboard's billing card collects it through Stripe; the platform never
sees the number. Until the card is on file, `repose run`, `repose start` and
creating a project all answer `payment_required` with
`detail.reason = card_required`. The trial credit does not change this: a
card comes before compute (DECISIONS R2-10), because a stranger with free
compute is the abuse vector.

The other reasons the same error carries:

| `detail.reason` | What the user did | What fixes it |
|---|---|---|
| `card_required` | no card on file | add one |
| `trial_depleted` | $10 of credit used, no paid usage yet | nothing — it means the card is missing, not the credit; `has_card` is checked first |
| `past_due` | an invoice failed | update the card in the billing portal |
| `suspended` | three days past due, or an operator suspension | pay, or email |

An account an operator has marked billing-exempt (`repose-admin users exempt`)
passes all of these; it still meters, so the numbers below are still real for
it (DECISIONS I-16).

## The $10

A new account gets $10 of credit. It is consumed at exactly the rates a paid
account pays — a large guest takes 14 cents an hour out of it, a 40 GB volume
takes about half a cent an hour — so the trial is also the first test of the
meters. `repose status` and the dashboard show what is left. When it reaches
zero nothing stops; the next hour is charged to the card.

A credit is never refunded as money and never expires. An operator can add
more with `repose-admin billing credit`, which is also how a goodwill
correction or a refund is recorded.

## What a user is charged for

Three things, per project:

- **Guest-hours**, by size class, for every minute the guest is `running`.
  A stopped guest accrues none. The total for a project in one billing
  period never exceeds that class's monthly price, so a guest left running
  all month costs exactly the flat price and one stopped every night costs
  less.
- **Storage**, on the *allocated* volume size, for as long as the project
  exists — running, stopped, whatever. This is the line that surprises
  people: a stopped project is not a free project. `repose destroy` is what
  stops it.
- **Egress**, 500 GB included per project per billing period, then $0.05 a
  GB. Traffic to the gateway (SSH, noVNC, hooks) is not counted; only what
  leaves the guest for the internet.

The billing period is a month from the day the account added its card, not
the calendar month. An account anchored on the 31st bills on the 28th in
February and returns to the 31st in March, which is how Stripe does it.

### Changing size mid-period

The cap follows the largest class the project ran in during the period. A
project that spends a month at XL and is downgraded to small on the last day
is still held to the XL cap, not the small one; a project that runs one hour
at XL and the rest at small pays for that one hour and is nowhere near any
cap. Both fall out of the same rule, and `repose-admin billing explain`
prints it for the hour in question.

## Seeing the number

```
$ repose status
todo-app   large   running  2h14m   claude: working   $0.31 so far today
```

`cost_today_cents` and `cost_month_cents` on a project, and `GET /usage` for
a range, all come from the same `usage_hours` rows the invoice is built
from, so the CLI, the dashboard and the Stripe invoice cannot disagree.
Usage is rolled up once an hour, at five past, so the figure lags by up to
an hour and a bit.

A gap in the host's samples under-bills that hour: the minutes it did not
hear about are not charged and are never estimated. This is deliberate — it
makes a host outage visible as a short bill rather than an invented one.

## The invoice

Monthly, through Stripe, charged to the card on file, with three lines:
compute, storage and egress, each in cents. Trial credit is consumed before
anything reaches Stripe, so it appears as a smaller invoice rather than a
negative line. Tax is added by Stripe from the address on the card.

`GET /billing/invoices` and the dashboard list them; the billing portal
(`POST /billing/portal`) is where a user changes the card, downloads an
invoice, or updates their address.

## A failed payment

Stripe retries on its own schedule. Meanwhile:

| When | What happens |
|---|---|
| day 0 | the invoice fails; the account is `past_due`; guests keep running; starting a new one is refused |
| days 1–2 | Stripe's reminder emails; the user can pay from the portal at any point |
| day 3 | every running guest is snapshotted and stopped, a `billing_stopped` notification goes out, the account becomes `suspended` |
| day 33 | the 30-day retention that started at suspension runs out and the snapshots go |

Nothing is deleted before day 33 (DECISIONS R4-11). Paying at any point
before then returns the account to `active` and unblocks starts —
but it does **not** start the guests again. That is the user's call, made
with `repose start`, so nobody is surprised by a bill for compute that
restarted itself.

## "I think I was overcharged"

`repose-admin billing explain <project> <hour>` prints every input and every
step for one hour: how many samples the hour had, the period totals before
it, which cap applied, the storage remainder, the credit taken and what
reached Stripe. A correction is a credit row, never an edit to the usage
history — `ops/RUNBOOK.md` has the procedure.

## Deferred

- Per-seat or team pricing (no teams in the first release, DECISIONS R5-6).
- Currencies other than USD, and VAT ids beyond what Stripe Tax collects.
- Idle auto-stop, which would change what a guest-hour means (R1-5).
- Annual or committed-use discounts.
