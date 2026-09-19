subscription_id     = "5f27aace-dd8c-4dc0-95bf-b59ee8de7d70"
env                 = "staging"
resource_group_name = "repose-staging"
location            = "eastus"

# Null: staging keeps its state in production's account under the key
# staging.tfstate (see infra/azure/staging/main.tf).
state_account_name = null
