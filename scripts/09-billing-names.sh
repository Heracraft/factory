#!/usr/bin/env bash
# docs/CHECKLIST.md: every command, flag, endpoint, message, table and field
# named in docs/workstreams/09-billing.md exists with that exact name.
set -u
cd "$(dirname "$0")/.."
miss=0
row() {
  local what="$1" pat="$2" where="$3"
  local hit
  hit=$(rg -n --no-heading -m1 -- "$pat" $where 2>/dev/null | head -1)
  if [ -z "$hit" ]; then printf '  MISSING  %-46s %s\n' "$what" "$pat"; miss=1
  else printf '  ok       %-46s %s\n' "$what" "$hit"; fi
}
echo "== tables and columns (migration 0003 + 0001) =="
for t in credit_ledger stripe_events usage_hours invoices; do row "table $t" "create table $t" internal/db/migrations; done
for c in period_start period_end credit_cents price_version stripe_usage_record_id storage_remainder gap; do row "usage_hours.$c" "\b$c\b" internal/db/migrations; done
for c in billing_anchor past_due_since stripe_subscription_id stripe_customer_id billing_status has_card trial_credit_cents project_limit xl_limit; do row "users.$c" "\b$c\b" internal/db/migrations; done
for c in cents reason ref; do row "credit_ledger.$c" "  $c " internal/db/migrations/0003_billing.up.sql; done

echo "== routes (docs/interfaces/api.md) =="
for r in '/v1/usage' '/v1/billing/portal' '/v1/billing/setup' '/v1/billing/invoices' '/v1/billing/webhook'; do row "route $r" "$r" internal/api/http/routes.go; done

echo "== repose-admin billing subcommands =="
for c in Rollup Credit Explain Reconcile Resync; do row "billing${c}" "^func \(e \*Env\) billing${c}\(" internal/admin/cmds.go; done
row "billing suspend|unsuspend (alias of users)" "the same action as \`users" internal/admin/cmds.go
row "usage line in repose-admin help" "^  billing " internal/admin/admin.go

echo "== prices (internal/billing/prices.go) =="
for k in CapSmall CapLarge CapXL HourSmall HourLarge HourXL StoragePerGBMonth EgressIncludedGB EgressPerGB TrialCreditCents ProjectLimitTrial XLLimitTrial ProjectLimitPaid XLLimitPaid; do row "$k" "^\s*$k\b" internal/billing/prices.go; done
row "PriceVersion" "^const PriceVersion" internal/billing/prices.go
for f in Cap Hourly LargerClass Price PeriodFor Balance Credit DebitUsage UsageRef Explain; do row "func $f" "^func $f\(" internal/billing; done
for f in NewRollup NewStripe NewWebhooks NewDunning NewReconciler RecordEnforcement; do row "func $f" "^func $f\(" internal/billing; done

echo "== webhook types (5.6) =="
for k in 'invoice.paid' 'invoice.payment_failed' 'customer.subscription.deleted' 'setup_intent.succeeded' 'payment_method.detached' 'charge.refunded'; do row "$k" "\"$k\"" internal/billing/webhook.go; done

echo "== metrics =="
for k in repose_api_rollup_duration_seconds repose_api_billing_gap_minutes_total repose_api_billing_stripe_push_backlog_seconds repose_api_billing_mismatch_cents repose_api_stripe_usage_push_total repose_api_stripe_webhook_total; do row "$k" "$k" internal/api/metrics/metrics.go; done

echo "== alerts and runbook headings =="
for a in StripePushBacklog BillingMismatch StripeWebhookRejected; do row "alert $a" "alert: $a" ops/alerts.yaml; done
for h in stripepushbacklog billingmismatch stripewebhookrejected; do row "runbook #$h" "^## " docs/ops/RUNBOOK.md >/dev/null; done
for h in StripePushBacklog BillingMismatch StripeWebhookRejected; do row "runbook ## $h" "^## $h" docs/ops/RUNBOOK.md; done

echo "== env (ops/coolify/api.env.example) =="
for v in STRIPE_SECRET_KEY STRIPE_WEBHOOK_SECRET STRIPE_PRICE_COMPUTE STRIPE_PRICE_STORAGE STRIPE_PRICE_EGRESS STRIPE_METER_COMPUTE STRIPE_METER_ID_COMPUTE BILLING_ENFORCE; do row "$v" "^$v" ops/coolify/api.env.example; done

exit $miss
