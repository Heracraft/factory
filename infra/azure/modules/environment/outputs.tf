output "edge_public_ip" {
  description = "Static address of the edge; ssh.repose.herakraft.co and every host's WireGuard endpoint."
  value       = module.edge.public_ip
}

output "edge_ssh_jump" {
  description = "Operator jump target for reaching hosts."
  value       = module.edge.ssh_jump
}

output "control_public_ip" {
  description = "Static address of the control plane."
  value       = module.coolify.public_ip
}

output "guest_egress_ip" {
  description = "Address every guest on every host egresses from, through the NAT gateway."
  value       = module.network.nat_public_ip
}

output "hosts" {
  description = "Per-host private address, jump target and class."
  value = {
    for name, h in module.host : name => {
      private_ip = h.private_ip
      ssh_jump   = h.ssh_jump
      class      = h.class
    }
  }
}

output "snapshots" {
  description = "Where hostd writes snapshots and the identity it does it as."
  value = {
    account            = module.storage.account_name
    container          = module.storage.container_name
    blob_endpoint      = module.storage.blob_endpoint
    identity_client_id = module.storage.host_identity_client_id
  }
}

output "keyvault" {
  description = "Vault URI and the versioned id of the DEK-wrapping key, for the api's environment."
  value = {
    vault_uri             = module.keyvault.vault_uri
    key_id                = module.keyvault.key_id
    api_policy_configured = module.keyvault.api_policy_configured
  }
}

output "fqdns" {
  description = "DNS names created, empty when manage_dns is false."
  value       = var.manage_dns ? module.dns[0].fqdns : {}
}
