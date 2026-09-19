# Production's resource group and state account already exist; they were
# created by hand on 2026-09-19 (docs/ops/AZURE-SETUP.md steps 4 and 5) and
# are deliberately not managed from here. These values describe them, for the
# `tofu import` commands at the top of main.tf and so the intended settings
# are written down.

subscription_id     = "5f27aace-dd8c-4dc0-95bf-b59ee8de7d70"
env                 = "prod"
resource_group_name = "repose-prod"
location            = "eastus"

state_account_name   = "reposetfstate3912"
state_container_name = "tfstate"
