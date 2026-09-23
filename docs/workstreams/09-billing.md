# 09 · billing

## 1. Goal

Turn the meter samples hostd already sends into money, correctly, from the
first hour. Stripe is the ledger of record for charges; `usage_hours` is the
ledger of record for what was used; a nightly reconciliation proves they
agree. A user who never stops a guest pays exactly the flat cap, and a user
who stops it half the time pays half.

## 2. Scope: builds

- `internal/billing/` in the api: Stripe customer lifecycle, SetupIntent for
  card on file, trial credit ledger, hourly rollup from `meter_samples` into
  `usage_hours`, pricing and cap logic, hourly usage record push to Stripe,
  monthly invoices via Stripe Billing, webhooks, past-due handling,
  suspension, reconciliation job, admin overrides.
- The `usage_hours`, `invoices` tables and a `credit_ledger` table (added to
  `interfaces/db-schema.md` by this workstream).
- The `/usage`, `/billing/*` routes in `interfaces/api.md` (implemented in
  05's HTTP layer, logic here).
- `repose-admin billing` subcommands: `credit <user> <cents> <reason>`,
  `suspend`, `unsuspend`, `reconcile [--month]`, `explain <project> <hour>`.
- The Stripe test-mode fixture that reproduces the CHECKLIST usage pattern
  (one large guest, 100 hours, 40 GB, 10 GB egress) and asserts the invoice.
- `PRICING.md` numbers wired as constants in one file,
  `internal/billing/prices.go`, with a test that fails if the doc and the
  constants disagree (the test parses the table in `PRICING.md`).

### Billing-exempt accounts (DECISIONS I-16)

`repose-admin users exempt <handle>` sets `billing_status=exempt`. Exempt
users pass the card and trial checks, still accrue `usage_hours`, and are
never pushed to Stripe. With no `STRIPE_*` variables the api starts normally
and billing routes answer `503 billing_disabled`.

## 3. Scope: does not build

- Sampling (03-hostd sends `Samples`; 05 writes `meter_samples`). This
  workstream reads `meter_samples` and nothing upstream of it.
- The dashboard billing page UI (08). This workstream provides the routes
  and the Stripe objects it needs.
- Tax (Stripe Tax is switched on in the Stripe dashboard with no code;
  documented in §5.9), VAT ids, invoices in currencies other than USD.
- Idle auto-stop (R1-5, later). Billing must not depend on it.
- Egress shaping and counting (01-host-nixos, 03-hostd). Billing reads the
  bytes.

## 4. Interfaces

Owns: `usage_hours`, `invoices`, `credit_ledger` in
`interfaces/db-schema.md`; the `/usage` and `/billing/*` route semantics in
`interfaces/api.md`.

Consumes: `interfaces/grpc-hostd.md` (`GuestSample` fields: `state`,
`class`, `disk_alloc_bytes`, `net_tx_bytes_delta`), `interfaces/db-schema.md`
(`meter_samples`, `projects`, `users`).

## 5. Design detail

### 5.1 Prices

From `DESIGN.md` §14 and `PRICING.md`, in cents, USD:

| Item | Value |
|---|---|
| small cap / month | 4900 |
| large cap / month | 9900 |
| xl cap / month | 19900 |
| small hourly | ceil(4900 / 720) = 7 |
| large hourly | ceil(9900 / 720) = 14 |
| xl hourly | ceil(19900 / 720) = 28 |
| storage per GB-month | 10 |
| egress included per project per month | 500 GB |
| egress per GB beyond | 5 |
| trial credit | 1000 |

"Month" is the user's Stripe billing period (anchored when the card is
added and the subscription created, stored truncated to the hour,
DECISIONS I-179), not the calendar month. The cap applies per project per billing period to guest-hours
only; storage and egress are always additive. A project that changes class
mid-period is capped at the sum of `hours_in_class × hourly` bounded by the
larger class's cap; simpler rules were considered and rejected because they
either let a downgrade-then-upgrade evade the cap or overcharge a one-hour
XL trial. Recorded here as the rule; `explain` prints the arithmetic.

### 5.2 Customer and card

On first `GET /me` after Logto sign-up, the api creates a Stripe customer
(`metadata.user_id`, email) and stores `stripe_customer_id`. `POST
/billing/setup` creates a SetupIntent (`usage: off_session`,
`payment_method_types: [card]`) and returns its client secret; with
`{"flow": "checkout"}` it creates a Stripe Checkout session in setup mode
instead (billing address required and saved on the customer) and returns
its URL, which is what the dashboard uses (DECISIONS I-182). The
`setup_intent.succeeded` webhook attaches the payment method as the
customer's default, copies its billing address onto the customer, creates
the subscription (adopting a live one Stripe already has, I-181) and sets
`users.billing_status` from `trial` (no card) to `trial` (with card,
`has_card = true`); a `trial` account whose credit is already spent goes
to `active` in the same statement (I-184). A guest cannot be created or
started while `has_card` is false, whatever the trial balance: that is R2-10,
card before compute.

### 5.3 Trial credit

`credit_ledger (id, user_id, cents, reason, ref, created_at)`; sign-up
inserts `+1000 "trial"`. Hourly usage first debits the ledger (a negative
row per hour with `ref = usage_hours pk`) until the balance is zero, and only
the remainder becomes a Stripe usage record. A user with a positive balance
and a card is `trial`; when the balance hits zero they become `active`
and the next hour is billed — the transition happens in the same
transaction as the debit that exhausted the balance, because the card gate
refuses a `trial` account with no credit (`trial_depleted`) and an account
that merely used its trial credit must not be locked out of its own guests.
The transition needs a card; an account whose card was removed before its
credit ran out stays `trial` at zero, is refused `trial_depleted`, and
moves to `active` when a card arrives (DECISIONS I-184). `repose-admin billing credit` adds rows for
goodwill or refunds. Balance is `sum(cents)`, computed with an index, never
cached on `users` (the cached column was rejected because two hourly jobs
racing would drift it). `users.trial_credit_cents` remains as a *projection*
of the ledger, updated by an `after insert` trigger inside the same
transaction and under the same row lock as the ledger row, so it cannot
drift; `sum(cents)` is still the balance of record and is what `Balance`
returns (DECISIONS I-78).

### 5.4 Hourly rollup

The rollup is `internal/billing.Rollup` (DECISIONS I-78; workstream 05 had
built it as `internal/api/meter.Rollup` against the calendar month).
`usage_hours` rows carry the `period_start` and `period_end` they were
priced in, so the running totals a later hour reads cannot drift from the
rule that priced the earlier ones.

A job at `:05` past each hour, idempotent, keyed on `(project_id, hour)`:

1. For each project with any `meter_samples` in the hour, or with
   `state != destroyed` (storage accrues while a project exists):
   `running_seconds = count(samples where state = running) × 60`, capped
   at 3600; a sample is a minute. Missing samples (host unreachable) count
   as not running for that minute, and a `billing_gap` metric records it so
   a host outage is visible as under-billing rather than a silent overcharge.
2. `class` = the class in the last sample of the hour (class changes require
   a stopped guest, so within an hour of running it is constant).
3. `gb_alloc` = `disk_alloc_bytes` of the last sample, or
   `projects.volume_bytes` if no sample.
4. `egress_bytes = sum(net_tx_bytes_delta)`.
5. `cost_cents` = guest part + storage part + egress part:
   - guest part: `hourly[class] × running_seconds / 3600`, rounded half up,
     then reduced so that the running total of guest parts for this project
     in the billing period does not exceed the cap (`cap[class]`, see 5.1
     for class changes).
   - storage part: `gb_alloc × 10 / hours_in_period`, fractional cents kept
     as a running remainder per project so the period sums exactly to
     `gb_alloc × 10`.
   - egress part: 0 until the project's period egress exceeds 500 GB, then
     `5 × GB` over, computed on the period running total so the threshold
     hour is charged only for the excess.
6. Insert `usage_hours`, then apply the credit ledger, then push a Stripe
   usage record for the remainder (5.5).

`hours_in_period` is the period's actual length in hours.

### 5.5 Stripe objects

**As built (DECISIONS I-77).** Stripe retired the `usage_type = metered` /
`aggregate_usage = sum` subscription-item usage records this section was
written against; the shape below is the same contract expressed with
billing meters, which is what the current API and `stripe-go` v83 offer.

One product `repose`, three **meters** — `repose_compute_cents`,
`repose_storage_cents`, `repose_egress_cents` — and three metered prices in
USD, one per meter, billing scheme per unit at 1 cent. Each user has one
subscription carrying those three prices, created at first card attach with
the period anchored then (`users.stripe_subscription_id`,
`users.billing_anchor`). Trial credit is not a Stripe line, because a
negative line is not allowed: the credit is consumed before the push and
the invoice shows a "trial credit applied" memo line.

Each `usage_hours` row is pushed as up to three meter events with
`timestamp = hour + 59:59` (the hour's last second, so the hour lands in
the same period on Stripe's side as on the rollup's, DECISIONS I-179), `payload.stripe_customer_id`, `payload.value` in cents,
and `identifier = usage:<project_id>:<hour>:<compute|storage|egress>`, which
is the idempotency key: Stripe enforces it as unique over a rolling window
of at least 24 hours, and it is sent as the HTTP idempotency key as well.
`usage_hours.stripe_usage_record_id` holds `usage:<project_id>:<hour>` and a
row that has one is never pushed again. A part worth zero cents is not
pushed at all. An exempt account's rows are marked `exempt` instead
(DECISIONS I-16).

The rejected alternative was one price per class with per-hour quantities;
it makes the cap impossible to express in Stripe and doubles the number of
prices whenever a class is added. Cents as the unit keeps Stripe a ledger of
amounts we computed.

The Stripe configuration is `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`,
`STRIPE_PRICE_{COMPUTE,STORAGE,EGRESS}`, `STRIPE_METER_{COMPUTE,STORAGE,
EGRESS}` (the event names, defaulted to the three above),
`STRIPE_METER_ID_{COMPUTE,STORAGE,EGRESS}` (the `mtr_...` ids reconciliation
reads summaries from), `STRIPE_PORTAL_CONFIGURATION` and
`STRIPE_AUTOMATIC_TAX`. `repose-admin billing stripe-bootstrap`
(`ops/stripe/bootstrap.sh`) creates every object from the secret key and
prints the whole block (DECISIONS I-180). A secret key without the webhook secret and the three
prices is refused at start rather than silently billing nothing; with no
secret key at all the api starts normally and the billing routes answer
`503 billing_disabled`. `ops/AZURE-SETUP.md` step 17 runs the bootstrap.

### 5.6 Invoices and dunning

Stripe Billing issues the invoice at period end and charges the default
card. Webhooks handled (all idempotent on `event.id` stored in a
`stripe_events` table): `invoice.paid` (insert or update `invoices`, set
`active`; a $0 invoice is recorded and changes nothing else, I-184), `invoice.payment_failed` (set `past_due`, `past_due_since`),
`customer.subscription.deleted`, `setup_intent.succeeded`,
`payment_method.detached` (set `has_card = false`), `charge.refunded`
(credit ledger row). Stripe's own retry schedule (Smart Retries) is on.

Past due: a job every hour stops every running guest of a user whose
`past_due_since` is older than 3 days (snapshot first, reason
`billing`), sends the `billing_stopped` notification, and sets
`billing_status = suspended`. Nothing is destroyed; R4-11's 30-day
retention starts at suspension. On `invoice.paid` the status returns to
`active` and guests stay stopped until the user starts them.

### 5.7 Reconciliation

`repose-admin billing reconcile --month 2026-10` (and a nightly job for
the current period): for each user, sum `usage_hours.cost_cents` minus
credit rows, compare with the sum of Stripe usage record quantities for the
period. Any difference over 0 cents is a `billing_mismatch` alert with the
project and hour; the job never fixes it silently. `explain <project>
<hour>` prints every input and each step of 5.4 for one row, which is the
tool for answering a support ticket.

### 5.8 Limits and abuse hooks

`users.project_limit` defaults to 3 and `xl_limit` to 1 until the first
`invoice.paid`, then 10 and 10. `repose-admin suspend <user> <reason>`
stops guests and blocks starts; `unsuspend` reverses. Both write
`audit_log`.

### 5.9 Tax and legal

Stripe Tax enabled on the subscription with `automatic_tax`. Customer
address collected by the Stripe card form (`billing_details.address`
required). No code beyond passing `automatic_tax: {enabled: true}`. Refunds
are manual in the Stripe dashboard plus a `credit` row for the record.

### 5.10 Cost display

`GET /usage` sums `usage_hours` for the range; `cost_today_cents` and
`cost_month_cents` on `Project` are computed by the same function for the
user's local day and the current billing period, so the CLI, dashboard and
invoice never disagree.

## 6. Failure modes

| Situation | Outcome |
|---|---|
| Stripe unreachable during the hourly push | `usage_hours` row exists with null record id; the next hourly run retries every null row; alert `StripePushBacklog` (`repose_api_billing_stripe_push_backlog_seconds`) if any row is older than 6 hours |
| Duplicate webhook delivery | ignored by `stripe_events` primary key |
| Webhook signature invalid | 400, logged with the event type only, alert after 5 in 10 minutes |
| Sample gap for a running guest | under-billed minutes, `billing_gap` metric, never estimated |
| Class changed mid-period | cap rule 5.1, `explain` shows it |
| Card removed while guests run | `has_card = false`, guests keep running until invoice failure path; starts blocked with `payment_required` |
| Trial balance negative (race) | impossible by construction: debit inside the same transaction as the `usage_hours` insert with `select ... for update` on the user row |
| Reconciliation mismatch | alert `BillingMismatch` (`repose_api_billing_mismatch_cents`) with details; humans decide; `credit` for the fix |
| Suspended user calls start | `payment_required` with `detail.reason = suspended` |

## 7. Testing

- Unit: pricing table parity test against `PRICING.md`; rollup math with
  golden hours (partial hour, cap hit mid-hour, class change, storage
  remainder summing exactly, egress threshold hour); ledger debit ordering.
- Integration with real Postgres: rollup over fixture `meter_samples`,
  idempotent re-run produces no change, credit ledger transaction under
  concurrency (two runners).
- Stripe test mode, in CI with a test key: the CHECKLIST fixture (one large
  guest, 100 running hours across a period, 40 GB, 10 GB egress) pushed with
  a test clock advanced to period end; assert invoice lines: compute 1400,
  storage 400, egress 0, total 1800 minus trial credit 1000 = 800. Failed
  payment with test card `4000000000000341` triggers the 3-day stop with the
  clock advanced. As built: `TestStripeTestModeM4Gate`, run with
  `REPOSE_STRIPE_TEST_KEY` (DECISIONS I-185, `docs/ops/M4-GATE.md` §2). The
  1400/400/0/1800 are asserted on `usage_hours`; the invoice's three lines
  are the parts left after the credit (a negative line is not allowed,
  §5.5), which sum to 800 and equal `usage_hours`' own split per line.
- Webhook replay test with recorded fixtures.
- Real: the owner's own card charged once in live mode for a small amount,
  invoice checked against `explain` output.

## 8. Rollback

Migrations for `usage_hours`, `invoices`, `credit_ledger`, `stripe_events`
have down scripts. Stripe prices are versioned by id; a price change creates
a new price and updates the subscription items, never edits the old price.
Turning billing off is setting `BILLING_ENFORCE=false`, which keeps rolling
up and pushing but stops blocking starts and stopping guests, and must be
logged as an audit event when flipped.

## 9. Checklist

Audited row by row by the M4 bring-up (2026-09-23, m4-billing). Rows whose
evidence is the real Stripe account wait for the test key only; the exact
commands that close them are `docs/ops/M4-GATE.md`, and the section is
named on each row. Test evidence below is from
`TMPDIR=/tmp/rt go test -count=1 -v ./internal/billing/ ./internal/admin/ ./internal/api/http/`.

- [x] `prices.go` constants equal the `PRICING.md` table; the parity test
      exists and passes. Evidence: `--- PASS: TestPricesMatchPricingDoc`
      (parses the table in `PRICING.md`) and `TestPriceHour`.
- [ ] Customer created at first `GET /me`; SetupIntent flow attaches a card;
      `has_card` flips on webhook. Evidence: Stripe test-mode transcript.
      **Waits for the test key: M4-GATE.md §3.1-3.2.** Offline: `TestCustomerSetupIntentAndSubscription`,
      `TestSetupCheckoutCollectsTheAddress` (the dashboard's hosted form,
      I-182), `TestSubscriptionAnchorsThePeriodAndEventsLandInIt`,
      `TestSubscriptionIsAdoptedNotDoubled`, `TestSubscriptionFallsBackWhenStripeRefusesTax`;
      the browser half `apps/web/tests/billing.spec.ts` 5/5. — waits on: owner
      (Stripe test key; M4-GATE.md §3.1-3.2).
- [x] Start and create blocked without a card, with `payment_required`.
      Evidence: `--- PASS: TestBillingGateBlocksCompute` (start and create,
      every reason: `card_required`, `past_due`, `suspended`, `trial_depleted`;
      exempt passes).
- [x] Trial credit inserted at signup, debited before Stripe, balance
      computed from the ledger. Evidence: `TestTrialCreditIsDebitedBeforeStripe`,
      `TestCreditLedgerUnderConcurrency` (two runners), and
      `TestTrialCreditDepletesThroughTheRollup` (sign-in inserts 1000; the
      real rollup spends the last 5 cents: `depleting hour …: cost 7 cents,
      credit 5, 2 billed to Stripe; account active, balance 0`).
- [x] Hourly rollup produces the golden `usage_hours` rows for every case in
      §7, including exact storage sums and the egress threshold hour.
      Evidence: `TestRollupGoldenHours`, `TestStorageSumsExactlyOverAnyPeriod`,
      `TestEgressThresholdHour`, `TestPartialHourRoundsHalfUp` (the golden
      values are the tables in those tests).
- [x] Rollup is idempotent: run twice, zero diff. Evidence:
      `TestRollupIsIdempotent`.
- [x] Cap: a guest running 720 hours in a period is charged exactly the cap;
      360 hours exactly half. Evidence: `TestRollupCapOverAPeriod`,
      `TestCapOverAWholePeriod`.
- [x] Class change mid-period follows 5.1 and `explain` shows it. Evidence:
      `TestClassChangeMidPeriodCap`, `TestExplainShowsTheArithmetic`.
- [ ] Usage records pushed with idempotency keys, ids stored, never
      re-pushed. Evidence: Stripe test-mode log with one record per row.
      **Waits for the test key: M4-GATE.md §2** (`TestStripeTestModeM4Gate`
      pushes through the real rollup and reconciles against Stripe's meter
      summaries). Offline: `TestChecklistUsageFixtureInvoicesTo800Cents`
      (a second push sends nothing; a replay with the same identifiers
      changes no total), `TestPushRetriesAfterAStripeFailure`. — waits on:
      owner (Stripe test key; M4-GATE.md §2).
- [ ] Stripe fixture in §7 yields an invoice of 800 cents after credit.
      Evidence: CI job output with the invoice id. **Waits for the test key:
      M4-GATE.md §2**, whose `M4 paid:` lines carry the invoice id and its
      lines against `usage_hours` (DECISIONS I-185). Offline:
      `TestChecklistUsageFixtureInvoicesTo800Cents` (1400 + 400 + 0 = 1800,
      less 1000 = 800 pushed). — waits on: owner (Stripe test key; M4-GATE.md
      §2, the `M4 paid:` lines).
- [x] All six webhooks handled idempotently; replay test passes; signature
      failures rejected. Evidence: `TestAllSixWebhooks`,
      `TestWebhookReplayIsANoOp`, `TestWebhookSignatureFailuresAreRejected`,
      `TestBillingWebhookRoute` (the route: no bearer, 400 on a bad
      signature with only the type logged, 200 on a duplicate),
      `TestZeroInvoicePaidRaisesNothing` (I-184). Real Stripe payloads for
      `invoice.paid` and `invoice.payment_failed` go through the same
      handler in M4-GATE.md §2.
- [ ] Past-due 3-day stop with snapshot and notification; `invoice.paid`
      reactivates without starting guests. Evidence: test with test clock.
      **Waits for the test key: M4-GATE.md §2, `M4 failed:` lines.**
      Offline: `TestPastDueThreeDayStop` (day 2 nothing, day 4 stop with
      `snapshot: true` and reason `billing`, `billing_stopped` event,
      suspended, audit row; `invoice.paid` leaves the guest stopped). — waits
      on: owner (Stripe test key; M4-GATE.md §2, the `M4 failed:` lines).
- [x] Reconciliation job and `explain` exist; a deliberate mismatch raises
      the alert and fixes nothing. Evidence: `TestReconcileReportsAndFixesNothing`,
      `TestReconcileWithoutAReaderSaysSo`, `TestExplainShowsTheArithmetic`.
- [x] Limits 3/1 before first paid invoice, 10/10 after. Evidence:
      `TestAllSixWebhooks` (3/1 before `invoice.paid`, 10/10 after) and
      `TestZeroInvoicePaidRaisesNothing` (a $0 invoice raises nothing).
- [x] `repose-admin billing credit|suspend|unsuspend|reconcile|explain`
      exist and write `audit_log`. Evidence: `TestBillingSubcommands`
      (each command run and its `audit_log` row read back), plus `show`,
      `cycle-now` (`TestAccountTotalsAndCycleNow`) and `stripe-bootstrap`
      (`TestStripeBootstrapCommandGuards`, I-180).
- [x] `BILLING_ENFORCE=false` behaves as §8 and logs an audit event.
      Evidence: `TestBillingEnforceFalseStopsNothing`,
      `TestBillingEnforceFalseLetsStartsThrough`, `TestEnforcementFlipIsAudited`.
- [ ] Stripe Tax enabled and address collected. Evidence: screenshot of a
      test invoice with tax line. **Waits for the owner to activate Stripe
      Tax, then M4-GATE.md §4.** The address is collected by the hosted
      card form (`billing_address_collection=required`,
      `TestSetupCheckoutCollectsTheAddress`) and copied to the customer
      (`TestSubscriptionAnchorsThePeriodAndEventsLandInIt`). — waits on: owner
      (activate Stripe Tax, then M4-GATE.md §4).
- [ ] Live-mode charge of the owner's card matches `explain`. Evidence:
      invoice id and the arithmetic pasted. **Waits for the owner's live
      key and card: M4-GATE.md §5** (`billing cycle-now` makes the invoice
      the same day). — waits on: owner (live key and card; M4-GATE.md §5).
- [x] Metrics for the rollup duration, the sample gap, the Stripe push
      backlog and the reconciliation difference exist. They carry the
      `repose_api_*` prefix every api family uses (DECISIONS I-49, I-60,
      I-78), not the `repose_billing_*` names this list was written with:
      `repose_api_rollup_duration_seconds`,
      `repose_api_billing_gap_minutes_total`,
      `repose_api_billing_stripe_push_backlog_seconds`,
      `repose_api_billing_mismatch_cents`. Evidence:
      `TestBillingMetricsAreExported` (scrapes the registry for the four
      names); m3-web recorded 4 of the 6 billing panels with real data from
      the production scrape (STATUS 2026-09-20).
- [x] `PRICING.md` and `features/pricing.md` match the implementation.
      Evidence: re-read 2026-09-23 by m4-billing; `features/pricing.md`
      updated for the hosted card form and the `trial_depleted` row
      (I-182, I-184); `PRICING.md` needed no change.
- [x] `ops/RUNBOOK.md` has: push backlog, mismatch alert, user says they
      were overcharged (use `explain`). Evidence: headings
      `## StripePushBacklog`, `## BillingMismatch`,
      `## A user says they were overcharged` (now starting from
      `billing show`), and `## StripeWebhookRejected` (now pointing at
      `bootstrap.sh --rotate-webhook`).
