---
title: Pricing and billing
description: Rates, the monthly cap, what a stopped project costs, invoices and failed payments.
section: Account
order: 30
---

## What you pay for

Three things, per project.

**Compute**, while the machine is `running`, by the minute:

| Size    | vCPU | Memory | Per hour | Monthly cap |
| ------- | ---- | ------ | -------- | ----------- |
| `small` | 2    | 4 GB   | $0.07    | $49         |
| `large` | 4    | 8 GB   | $0.14    | $99         |
| `xl`    | 8    | 16 GB  | $0.28    | $199        |

A project's compute in one billing period never goes over its size's monthly cap. A machine left running all month costs exactly the cap. One you stop every night costs less. If a project ran at more than one size in a period, the cap of the largest size it ran at applies.

**Disk**, $0.10 per GB per month on the disk's full size, whatever the machine's state, until you destroy the project. A `large` project's 40 GB disk is $4.00 a month, running or stopped, full or empty. Snapshots are free while the project exists, and a destroyed project's final snapshot is kept for 30 days at no charge.

**Egress**, data the machine sends to the internet: 500 GB per project per month included, then $0.05 per GB. Incoming data is free, and your SSH traffic (including port forwards and the desktop) doesn't count.

### Examples

- A `large` machine running all month with a 40 GB disk: $99 + $4 = $103.
- The same machine stopped 10 hours every night: about $58 + $4 = $62.
- A `small` project you stopped after a day and forgot: $1.68 for the day, then $2.00 a month for its disk until you destroy it.

## Your first day

A new account gets a day of compute free: a credit worth 24 hours of `large`, which is 48 hours of `small`. It's used at the normal rates, including disk. The Billing page shows how much is left. Nothing stops once it runs out; the next hour goes on your card. The credit doesn't expire and can't be paid out.

## A card first

No machine starts until your account has a card on file, trial or not. Add one on the dashboard's [Billing](https://repose.herakraft.co/billing) page. The form is Stripe's, which also asks for the billing address used to work out tax. repose never sees the card number.

Until a card is on file, `repose run` stops with:

```
Add a card at https://repose.herakraft.co/billing first.
```

(exit code 7).

## Seeing what you've spent

- `repose status` and `repose projects` show each project's cost today and this month.
- The dashboard's project page adds the month projected at the current rate.
- The Billing page shows this month's usage by size, and your invoices.

All of these and your invoice come from the same hourly usage records, so they agree. They're totalled a few minutes past each hour, so the figures can lag by up to about an hour.

If a server loses track of a machine for a while, the minutes it has no record of aren't charged. They're never estimated.

## Invoices

Your billing period starts on the day you added your card and runs a month. At the end of it, Stripe charges your card for compute, disk and egress, with tax added based on your billing address. The free credit is used before anything reaches Stripe, so it shows up as a smaller invoice.

The Billing page lists invoices with links to view them and download PDFs. **Manage card, address and invoices in Stripe** opens Stripe's billing portal, where you can change your card or address.

## If a payment fails

| When         | What happens                                                                                            |
| ------------ | ------------------------------------------------------------------------------------------------------- |
| Day 0        | The payment fails. Running machines keep running. Starting a machine is refused.                        |
| Days 1 and 2 | Stripe emails you. You can pay from the billing portal at any time.                                     |
| Day 3        | Every running machine is snapshotted and stopped, and you get a notification. The account is suspended. |
| Day 33       | Snapshots start being deleted.                                                                          |

Paying at any point before then makes the account active again. Machines don't restart by themselves after you pay; start the ones you want with `repose start`, so nothing runs up compute you didn't ask for.

## Limits

Until your first invoice is paid: 3 projects, at most 1 of them `xl`. After that: 10 projects. Higher limits are set by hand; there is no self-serve way to raise them yet.

## Stopping the charges

- `repose stop` ends compute charges. Disk charges continue.
- `repose destroy` ends all charges for the project. The final snapshot is kept free for 30 days.
- Deleting your account (dashboard, Account page) stops every machine at once and closes billing at the end of the period.
