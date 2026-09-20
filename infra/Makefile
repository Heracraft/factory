# repose infrastructure.
#
#   make plan  ENV=prod        what an apply would change
#   make apply ENV=prod        change it (the owner's call, never an agent's)
#   make check                 fmt, validate and policy across every root
#
# ENV is prod or staging. `make bootstrap ENV=staging` applies
# infra/bootstrap, the one root with local state.
#
# Authentication is `az login` on the machine running this (docs/ops/
# AZURE-SETUP.md step 6), plus CLOUDFLARE_API_TOKEN in the environment when
# manage_dns is true.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

TOFU ?= tofu
ENV ?= prod

ifeq ($(ENV),prod)
  ROOT := azure/prod
  VARFILE := prod.tfvars
else ifeq ($(ENV),staging)
  ROOT := azure/staging
  VARFILE := staging.tfvars
else
  $(error ENV must be prod or staging; got "$(ENV)")
endif

LOCAL_VARFILE := $(ENV).local.tfvars
VAR_ARGS := -var-file=$(VARFILE) $(if $(wildcard $(ROOT)/$(LOCAL_VARFILE)),-var-file=$(LOCAL_VARFILE),)

ROOTS := azure/prod azure/staging bootstrap
POLICY_DIR := policy/tfsec

.PHONY: help
help:
	@echo "make plan ENV=prod|staging         show the diff"
	@echo "make apply ENV=...                 apply it"
	@echo "make output ENV=...                show outputs"
	@echo "make init ENV=...                  initialise one root"
	@echo "make validate                      tofu validate every root"
	@echo "make fmt / fmt-check               formatting"
	@echo "make policy                        tfsec, including the hosts-subnet rule"
	@echo "make check                         fmt-check + validate + policy"
	@echo "make bootstrap ENV=staging         apply infra/bootstrap (local state)"
	@echo "make force-unlock ENV=... LOCK_ID=...  release a stuck state lease"
	@echo ""
	@echo "az login for Azure; CLOUDFLARE_API_TOKEN when manage_dns=true"

# The Cloudflare provider reads its token from the environment, and without
# one it fails deep inside a resource read with an authentication error that
# names neither the variable nor the step that creates it. azure/ needs it
# only when manage_dns is true, which the roots default to false for exactly
# this reason, so no root requires it unconditionally any more.
TOKEN_GUARD :=

.PHONY: init
init: $(TOKEN_GUARD)
	$(TOFU) -chdir=$(ROOT) init -input=false

.PHONY: plan
plan: init
	$(TOFU) -chdir=$(ROOT) plan -input=false $(VAR_ARGS)

.PHONY: apply
apply: init
	$(TOFU) -chdir=$(ROOT) apply -input=false $(VAR_ARGS)

.PHONY: output
output:
	$(TOFU) -chdir=$(ROOT) output

# Every root, without contacting a provider or a backend: this is what CI runs
# on a machine with no Azure credentials.
.PHONY: validate
validate:
	for r in $(ROOTS); do \
	  echo "== $$r"; \
	  $(TOFU) -chdir=$$r init -backend=false -input=false >/dev/null; \
	  $(TOFU) -chdir=$$r validate; \
	done

.PHONY: fmt
fmt:
	$(TOFU) fmt -recursive .

.PHONY: fmt-check
fmt-check:
	$(TOFU) fmt -recursive -check -diff .

# The rule that matters most here is REPOSE-NET-001: a single inbound rule on
# the hosts subnet undoes the posture the whole design rests on
# (docs/DESIGN.md §4, §7).
.PHONY: policy
policy:
	tfsec . --custom-check-dir $(POLICY_DIR) --concise-output

.PHONY: check
check: fmt-check validate policy

.PHONY: bootstrap
bootstrap:
	$(TOFU) -chdir=bootstrap init -input=false
	$(TOFU) -chdir=bootstrap apply -input=false -var-file=$(ENV).tfvars

# Only after confirming no apply is running anywhere
# (docs/workstreams/11-infra-opentofu.md §6).
.PHONY: force-unlock
force-unlock:
	@test -n "$(LOCK_ID)" || { echo "LOCK_ID=<id from the error message> is required" >&2; exit 1; }
	$(TOFU) -chdir=$(ROOT) force-unlock $(LOCK_ID)
