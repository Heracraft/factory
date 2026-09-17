# infra

OpenTofu for the Azure-specific parts only: network, hosts, edge VM, Coolify
VM, Blob, Key Vault (`azure/`), and the Cloudflare R2 bucket for Postgres
backups (`r2/`). Everything else is provider-agnostic by design
(docs/DESIGN.md §16). See docs/workstreams/11-infra-opentofu.md.

Never commit `*.tfstate` or `*.auto.tfvars`.
