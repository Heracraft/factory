# M4 gate runbook

The gate (`docs/MILESTONES.md` M4): a real card is charged the right amount
for a known usage pattern (one large guest, 100 hours, 40 GB, 10 GB egress)
and the Stripe invoice matches the `usage` rows to the cent; trial credit
depletes and blocks a start at zero; a failed payment stops guests after 3
days. This page is the whole path from "here is `sk_test_…`" to the evidence
for every open row of `docs/workstreams/09-billing.md` §9, in one sitting.

What the owner provides, exactly:

1. **The test-mode secret key** (`sk_test_…`, Stripe dashboard > Developers
   > API keys). Nothing else for test mode: no publishable key (I-182), no
   webhook secret, no price or meter ids (I-180).
2. For the tax-line row only: **Stripe Tax activated** (Settings > Tax, the
   business address). Optional for everything else; without it the
   invoices carry no tax line and the bootstrap prints
   `STRIPE_AUTOMATIC_TAX=false`.
3. For step 5 only: **the live key and their own card**, on a day they
   choose. Anything that costs money is announced to the conductor first.

Conventions: `ra` below is `docker exec <api container> repose-admin` on the
control VM (the container has `DATABASE_URL` and, after step 1, every
`STRIPE_*` variable). Commands under "dev box" run from a checkout of
`main`, outside or inside `nix develop ./nix`. Read the key into the shell
without echoing it, never on a command line:

```
read -rs STRIPE_SECRET_KEY && export STRIPE_SECRET_KEY    # paste sk_test_..., Enter
```

## 0. Before: `main` is deployed

The api and web must run the code that has `stripe-bootstrap`,
`billing show`, `billing cycle-now`, the Checkout card flow and I-179's
anchor. `curl -s https://api.repose.herakraft.co/v1/billing/webhook -X POST`
answers `503 billing_disabled` until step 1 is done.

## 1. Stripe objects and the api's environment (5 minutes)

Dev box:

```
ops/stripe/bootstrap.sh > /tmp/stripe.env
```

Stderr lists each object as `created` (first run) or `found` (any rerun);
stdout, in `/tmp/stripe.env`, is the block. Paste it into the Coolify
environment of **both** `api` and `api-grpc` (both read billing: the
webhook is served by `api`, the rollup and dunning run under the leader
lock in whichever holds it), save, and let Coolify restart them
(`docs/ops/coolify.md` fact 16). Then `shred -u /tmp/stripe.env`.

Check:

```
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://api.repose.herakraft.co/v1/billing/webhook   # 400: signature required, billing is on
ra billing reconcile        # "no differences" (no customers yet), not "billing is not configured"
```

Evidence for AZURE-SETUP step 17: the stderr of the bootstrap (object ids,
no secrets).

## 2. The fixed pattern against Stripe itself, to the cent (20-30 minutes)

This is the gate's invoice sentence and the failed-payment sentence,
proved on Stripe test clocks with the production rollup and webhook code
(DECISIONS I-185). Nothing waits 100 real hours: the guest's minutes are
written as `meter_samples`, the one input the rollup reads; everything
from there on is the real code and the real Stripe. Dev box:

```
mkdir -p /tmp/rt
REPOSE_STRIPE_TEST_KEY=$STRIPE_SECRET_KEY TMPDIR=/tmp/rt \
  go test -count=1 -v -timeout 60m -run TestStripeTestModeM4Gate ./internal/billing/ 2>&1 | tee /tmp/m4-gate.log
grep -E 'M4 |--- (PASS|FAIL)' /tmp/m4-gate.log
```

It runs the bootstrap itself (without a webhook endpoint, so it is safe
before or after step 1), then two scenarios in parallel, each on its own
test clock frozen 33 days back and its own throwaway database:

- **paid** (`pm_card_visa`): 100 running hours of a large guest, 40 GB for
  the whole period, 10 GB egress. Expect
  `usage_hours compute 1400 + storage 400 + egress 0 = 1800 less trial credit 1000 = 800`
  and `invoice in_… lines compute C + storage S + egress 0 = subtotal 800 … status paid`,
  where C and S are the per-line amounts after the credit and equal
  `usage_hours`' own split; `meter summaries … reconcile: no differences`;
  `trial credit ran out at …; status active, balance 0`;
  `invoice.paid evt_… applied` and `limits after invoice.paid 10/10`.
- **failed** (`pm_card_chargeCustomerFail`, the 4000 0000 0000 0341 card):
  the same invoice, unpaid; the real `invoice.payment_failed` event moves
  the account to `past_due`; the dunning job does nothing on day 2 and on
  day 3 plus an hour enqueues a stop with `snapshot: true`, reason
  `billing`, records `billing_stopped` and suspends the account; paying
  the invoice with a good card and applying the real `invoice.paid` makes
  it `active` with the guest left stopped.

`REPOSE_STRIPE_KEEP=1` keeps the clocks (and their customers and invoices)
for screenshots; otherwise they are deleted at the end. The invoice's
hosted URL is in the log. Paste the `M4 ` lines as the evidence.

If it fails at "meter summaries did not reach" but the invoice check
passes, Stripe's aggregation was slow; the invoice is the authority. If an
invoice is not found after the period end, the clock's advance is still
running on Stripe's side; rerun.

## 3. The live api, a real guest, test mode (about 3 hours, mostly waiting)

What step 2 cannot show: the customer made at first sign-in, the hosted
card form, webhooks arriving at the real endpoint, samples from a real
guest on host-01, and an invoice cut from them. Use a non-exempt account
(a second GitHub login, or an operator-created one; exempt accounts are
never pushed, I-16).

1. **Customer at first `GET /me`.** Sign in at
   `https://repose.herakraft.co`; then `ra billing show <handle>` prints a
   `stripe customer cus_…`, and the Stripe dashboard shows it with
   `metadata.user_id`.
2. **Card.** `/billing` > "Add a card" opens Stripe Checkout. Test card
   `4242 4242 4242 4242`, any future expiry, any CVC, and a real-looking
   address. It returns to `/billing` with "Card saved." and, within a few
   seconds, "A card is on file." `ra billing show <handle>`:
   `has_card true`, a `stripe subscription sub_…`, and `billing anchor`
   equal to the subscription's start truncated to the hour (I-179). In the
   Stripe dashboard the endpoint's deliveries show `setup_intent.succeeded`
   answered 200.
3. **Trial depletion on the real account.** Leave ten cents:
   `ra billing credit <handle> -990 "M4 gate: leave 10 cents"`. Start a
   large guest (`repose run` or the dashboard) and leave it running past
   the next `:05`. `ra billing show <handle>`: `billing active`,
   `credit balance 0 cents`, `first billed hour` set. Then in the Stripe
   portal ("Manage card, address and invoices in Stripe") remove the card;
   after `payment_method.detached` arrives, `repose start <project>` is
   refused `payment_required` with `card_required` (the guest that was
   running keeps running, §6). Add the card again (step 2) and the start
   goes through. That is the gate's "blocks a start at zero" as I-185
   reads it; the `trial_depleted` refusal itself is proven in
   `TestTrialCreditDepletesThroughTheRollup` (`go test -run
   TestTrialCreditDepletesThroughTheRollup -v ./internal/api/http/`).
4. **A short known pattern, invoiced now.** Keep a large guest running for
   two whole hours, then stop it. After the next `:05`:

   ```
   ra billing show <handle>                    # "billed compute C + storage S + egress E = N cents"
   ra billing explain <project> <YYYY-MM-DDTHH> # for each hour, if anyone asks
   ra billing reconcile                        # no differences (give Stripe a few minutes)
   ra billing cycle-now <handle>               # dry run: waits for Stripe to hold N, says what it would do
   ra billing cycle-now <handle> --yes         # ends the period; prints the invoice id and N
   ```

   `cycle-now` resets the subscription's billing cycle, which makes Stripe
   invoice the usage so far immediately, and moves the rollup's anchor to
   the same hour so the next period agrees on both sides (I-185). About an
   hour later Stripe finalises and charges the test card;
   `ra billing show <handle>` then lists the invoice as `paid` with
   `total_cents` equal to N plus any tax, and `/billing` shows it with its
   PDF. Evidence: the `show` output before, the `cycle-now` line, the
   `show` output after, and the invoice id.
5. **Webhooks at the real endpoint.** By now the endpoint's delivery list
   in the Stripe dashboard has `setup_intent.succeeded`,
   `payment_method.detached` and `invoice.paid` answered 200. The other
   three (`invoice.payment_failed`, `customer.subscription.deleted`,
   `charge.refunded`) have been applied from real Stripe payloads in
   step 2 and replayed from recordings in `TestAllSixWebhooks` /
   `TestWebhookReplayIsANoOp`; to see them hit the live endpoint too, use
   the dashboard's "Send test event" on the endpoint: an unknown customer
   is recorded and ignored with 200.
6. **Clean up.** Destroy the project; `ra billing credit <handle> 990
   "M4 gate: restore"` if the account is to be used again.

## 4. Tax line (when the owner has activated Stripe Tax)

Rerun `ops/stripe/bootstrap.sh`, which now prints
`STRIPE_AUTOMATIC_TAX=true` and otherwise the same ids; set that one
variable in Coolify. New subscriptions carry automatic tax; an existing
test account gets it by removing and re-adding its card after deleting its
subscription in the Stripe dashboard (the `customer.subscription.deleted`
webhook clears it and the next card makes a new one). Repeat step 3.4 and
screenshot the invoice: its tax line is the evidence.

## 5. Live mode, one charge of the owner's card (the owner's call)

Announce it to the conductor first. Dev box, with the live key read the
same way:

```
ops/stripe/bootstrap.sh --live > /tmp/stripe-live.env
```

Replace the test block in both `api` and `api-grpc` with it. The live
customer is new (test and live objects are separate): the owner signs in,
adds their card at `/billing`, runs a small guest for a known hour or two,
stops it, and then step 3.4's `billing show`, `cycle-now` and `show`
again. The charge lands about an hour after `cycle-now --yes`. The invoice
id, `billing show`'s billed line and `explain` for each hour are the
evidence, pasted with the arithmetic. A refund is the owner's decision:
refund in the Stripe dashboard; `charge.refunded` writes the matching
credit row by itself.

## If something is off

- The api will not start after the paste: the log names the missing
  variable; the block was cut short. Rerun the bootstrap and paste again.
- Webhooks rejected (`StripeWebhookRejected`): the endpoint's API version
  or secret, `ops/stripe/bootstrap.sh --rotate-webhook` (RUNBOOK).
- `reconcile` shows a difference with `UNPUSHED` > 0: `ra billing resync`.
- `cycle-now` waits and gives up: Stripe has not aggregated the events
  yet, or a meter id is wrong; nothing was changed, run it again later.
