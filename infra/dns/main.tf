# Cloudflare records for the herakraft.co names repose answers on.
#
# This is a module, not a root, so DNS is created in the same apply as the
# addresses it points at (docs/workstreams/11-infra-opentofu.md §2). A record
# that outlives the IP it names is how `ssh.repose.herakraft.co` starts
# resolving to somebody else's VM.
#
# Names: docs/DECISIONS.md R4-12 and I-15. repose.herakraft.co (dashboard),
# api.repose.herakraft.co, auth.repose.herakraft.co (Logto),
# ssh.repose.herakraft.co (gateway). The wildcard for preview URLs is not
# created; previews are out of scope for the first release (DESIGN §18).

terraform {
  required_version = ">= 1.6.0"
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = ">= 5.0, < 6.0"
    }
  }
}

resource "cloudflare_dns_record" "a" {
  for_each = var.records

  zone_id = var.zone_id
  name    = each.key
  type    = "A"
  content = each.value.address
  ttl     = each.value.proxied ? 1 : var.ttl
  proxied = each.value.proxied
  comment = each.value.comment
}
