# Network for one repose environment: one VNet, three subnets, one NAT
# gateway for the hosts subnet, one NSG per subnet.
#
# docs/workstreams/11-infra-opentofu.md §2, docs/DESIGN.md §7.
#
# The hosts subnet NSG carries *no* inbound security rules at all. Azure's
# built-in rules still apply underneath: AllowVnetInBound lets the edge reach
# a host's sshd for the nixos-anywhere provisioner and for operator access,
# and DenyAllInBound drops everything from the internet. A single explicit
# inbound rule here would be a bug, so `security_rule = []` is written out
# rather than left to default, and infra/policy/checkov enforces it.

terraform {
  required_version = ">= 1.6.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.0, < 5.0"
    }
  }
}

locals {
  tags = merge(var.tags, {
    "repose:role" = "network"
  })
}

resource "azurerm_virtual_network" "main" {
  name                = "vnet-repose-${var.env}"
  resource_group_name = var.resource_group_name
  location            = var.location
  address_space       = [var.vnet_cidr]
  tags                = local.tags
}

# --- hosts subnet: no inbound, egress through the NAT gateway ---------------

resource "azurerm_subnet" "hosts" {
  name                 = "snet-hosts"
  resource_group_name  = var.resource_group_name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [var.hosts_subnet_cidr]
}

resource "azurerm_network_security_group" "hosts" {
  name                = "nsg-hosts"
  resource_group_name = var.resource_group_name
  location            = var.location

  # Deliberately empty. See the header comment; checkov policy
  # REPOSE_HOSTS_SUBNET_NO_INBOUND keys on the tag below.
  security_rule = []

  tags = merge(local.tags, {
    "repose:role"    = "network"
    "repose:inbound" = "none"
  })

  lifecycle {
    postcondition {
      condition     = length(self.security_rule) == 0
      error_message = "The hosts subnet NSG must carry no security rules; hosts have no inbound (docs/DESIGN.md §4)."
    }
  }
}

resource "azurerm_subnet_network_security_group_association" "hosts" {
  subnet_id                 = azurerm_subnet.hosts.id
  network_security_group_id = azurerm_network_security_group.hosts.id
}

resource "azurerm_public_ip" "nat" {
  name                = "pip-natgw-hosts"
  resource_group_name = var.resource_group_name
  location            = var.location
  allocation_method   = "Static"
  sku                 = "Standard"
  zones               = var.zone == null ? null : [var.zone]
  tags                = local.tags

  lifecycle {
    # Guests egress from this address; a replacement changes what every
    # upstream sees and invalidates any allowlist a tenant has set.
    prevent_destroy = true
  }
}

resource "azurerm_nat_gateway" "hosts" {
  name                    = "natgw-hosts"
  resource_group_name     = var.resource_group_name
  location                = var.location
  sku_name                = "Standard"
  idle_timeout_in_minutes = var.nat_idle_timeout_minutes
  zones                   = var.zone == null ? null : [var.zone]
  tags                    = local.tags
}

resource "azurerm_nat_gateway_public_ip_association" "hosts" {
  nat_gateway_id       = azurerm_nat_gateway.hosts.id
  public_ip_address_id = azurerm_public_ip.nat.id
}

resource "azurerm_subnet_nat_gateway_association" "hosts" {
  subnet_id      = azurerm_subnet.hosts.id
  nat_gateway_id = azurerm_nat_gateway.hosts.id
}

# --- edge subnet: the only subnet the internet talks to ---------------------

resource "azurerm_subnet" "edge" {
  name                 = "snet-edge"
  resource_group_name  = var.resource_group_name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [var.edge_subnet_cidr]
}

# Ports come from docs/workstreams/06-gateway-edge.md §5.1: 22 is the user
# SSH gateway, 443 the preview-proxy stub, 51820/udp the WireGuard hub, and
# 2222 the operator sshd, which is reachable only from the operator list and
# from inside the VNet (the dev box jumps through it to reach hosts).
resource "azurerm_network_security_group" "edge" {
  name                = "nsg-edge"
  resource_group_name = var.resource_group_name
  location            = var.location

  security_rule = [
    {
      name                                       = "allow-ssh-gateway"
      description                                = "repose SSH gateway (docs/interfaces/ssh-gateway.md)"
      priority                                   = 100
      direction                                  = "Inbound"
      access                                     = "Allow"
      protocol                                   = "Tcp"
      source_port_range                          = "*"
      source_port_ranges                         = []
      destination_port_range                     = "22"
      destination_port_ranges                    = []
      source_address_prefix                      = "Internet"
      source_address_prefixes                    = []
      destination_address_prefix                 = "*"
      destination_address_prefixes               = []
      source_application_security_group_ids      = []
      destination_application_security_group_ids = []
    },
    {
      name                                       = "allow-preview-https"
      description                                = "preview proxy stub (docs/features/ports-and-previews.md)"
      priority                                   = 110
      direction                                  = "Inbound"
      access                                     = "Allow"
      protocol                                   = "Tcp"
      source_port_range                          = "*"
      source_port_ranges                         = []
      destination_port_range                     = "443"
      destination_port_ranges                    = []
      source_address_prefix                      = "Internet"
      source_address_prefixes                    = []
      destination_address_prefix                 = "*"
      destination_address_prefixes               = []
      source_application_security_group_ids      = []
      destination_application_security_group_ids = []
    },
    {
      name                                       = "allow-wireguard"
      description                                = "WireGuard hub every host dials (DECISIONS R4-3)"
      priority                                   = 120
      direction                                  = "Inbound"
      access                                     = "Allow"
      protocol                                   = "Udp"
      source_port_range                          = "*"
      source_port_ranges                         = []
      destination_port_range                     = "51820"
      destination_port_ranges                    = []
      source_address_prefix                      = "Internet"
      source_address_prefixes                    = []
      destination_address_prefix                 = "*"
      destination_address_prefixes               = []
      source_application_security_group_ids      = []
      destination_application_security_group_ids = []
    },
    {
      name                                       = "allow-operator-ssh"
      description                                = "operator sshd, key-only, operator IP list and the VNet"
      priority                                   = 130
      direction                                  = "Inbound"
      access                                     = "Allow"
      protocol                                   = "Tcp"
      source_port_range                          = "*"
      source_port_ranges                         = []
      destination_port_range                     = tostring(var.edge_operator_ssh_port)
      destination_port_ranges                    = []
      source_address_prefix                      = ""
      source_address_prefixes                    = concat(var.operator_cidrs, [var.vnet_cidr])
      destination_address_prefix                 = "*"
      destination_address_prefixes               = []
      source_application_security_group_ids      = []
      destination_application_security_group_ids = []
    },
    {
      name                                       = "allow-bootstrap-ssh"
      description                                = "Ubuntu sshd on 22 before nixos-anywhere runs; operator list only"
      priority                                   = 90
      direction                                  = "Inbound"
      access                                     = "Allow"
      protocol                                   = "Tcp"
      source_port_range                          = "*"
      source_port_ranges                         = []
      destination_port_range                     = "22"
      destination_port_ranges                    = []
      source_address_prefix                      = ""
      source_address_prefixes                    = var.operator_cidrs
      destination_address_prefix                 = "*"
      destination_address_prefixes               = []
      source_application_security_group_ids      = []
      destination_application_security_group_ids = []
    },
  ]

  tags = local.tags
}

resource "azurerm_subnet_network_security_group_association" "edge" {
  subnet_id                 = azurerm_subnet.edge.id
  network_security_group_id = azurerm_network_security_group.edge.id
}

# --- control subnet: the Coolify VM (api, dashboard, Logto, Postgres) -------

resource "azurerm_subnet" "control" {
  name                 = "snet-control"
  resource_group_name  = var.resource_group_name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [var.control_subnet_cidr]
}

resource "azurerm_network_security_group" "control" {
  name                = "nsg-control"
  resource_group_name = var.resource_group_name
  location            = var.location

  security_rule = [
    {
      name                                       = "allow-http"
      description                                = "Coolify proxy; also the ACME http-01 challenge"
      priority                                   = 100
      direction                                  = "Inbound"
      access                                     = "Allow"
      protocol                                   = "Tcp"
      source_port_range                          = "*"
      source_port_ranges                         = []
      destination_port_range                     = "80"
      destination_port_ranges                    = []
      source_address_prefix                      = ""
      source_address_prefixes                    = var.control_web_cidrs
      destination_address_prefix                 = "*"
      destination_address_prefixes               = []
      source_application_security_group_ids      = []
      destination_application_security_group_ids = []
    },
    {
      name                                       = "allow-https"
      description                                = "dashboard, api, Logto"
      priority                                   = 110
      direction                                  = "Inbound"
      access                                     = "Allow"
      protocol                                   = "Tcp"
      source_port_range                          = "*"
      source_port_ranges                         = []
      destination_port_range                     = "443"
      destination_port_ranges                    = []
      source_address_prefix                      = ""
      source_address_prefixes                    = var.control_web_cidrs
      destination_address_prefix                 = "*"
      destination_address_prefixes               = []
      source_application_security_group_ids      = []
      destination_application_security_group_ids = []
    },
    {
      name                                       = "allow-operator-ssh"
      description                                = "operator SSH to the Coolify VM"
      priority                                   = 120
      direction                                  = "Inbound"
      access                                     = "Allow"
      protocol                                   = "Tcp"
      source_port_range                          = "*"
      source_port_ranges                         = []
      destination_port_range                     = "22"
      destination_port_ranges                    = []
      source_address_prefix                      = ""
      source_address_prefixes                    = var.operator_cidrs
      destination_address_prefix                 = "*"
      destination_address_prefixes               = []
      source_application_security_group_ids      = []
      destination_application_security_group_ids = []
    },
  ]

  tags = local.tags
}

resource "azurerm_subnet_network_security_group_association" "control" {
  subnet_id                 = azurerm_subnet.control.id
  network_security_group_id = azurerm_network_security_group.control.id
}
