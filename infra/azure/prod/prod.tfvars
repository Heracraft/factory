# Production values that belong in review. Secrets and per-operator values go
# in prod.local.tfvars, which is not committed; see prod.local.tfvars.example.
#
# Join tokens are never written to a file at all: they are single-use, expire
# in 24 hours, and are passed on the apply that adds a host
# (infra/README.md, "Adding a host").

subscription_id     = "5f27aace-dd8c-4dc0-95bf-b59ee8de7d70"
resource_group_name = "repose-prod"
zone                = "1"

# DECISIONS I-14: the pre-launch host. Launch values are Standard_D64s_v5,
# azure-d64s-v5 and 2048, applied with guests stopped.
host_size           = "Standard_D16s_v5"
host_class          = "azure-d16s-v5"
host_data_disk_gb   = 512
host_data_disk_iops = 16000
host_data_disk_mbps = 600

# The portal defaults to Trusted Launch, which silently disables nested
# virtualization. Only "Standard" is accepted and the variable has no default.
host_security_type = "Standard"

edge_size    = "Standard_D2s_v5"
coolify_size = "Standard_D4s_v5"

# Hosts are added one at a time; see infra/README.md.
hosts = []

snapshots_account_name = "reposesnapshots3912"
keyvault_name          = "repose-kv-3912"

dns_zone = "herakraft.co"
