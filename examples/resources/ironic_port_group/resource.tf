# Example usage of ironic_port_group resource with the Framework implementation

terraform {
  required_providers {
    ironic = {
      source  = "metal3-community/ironic"
      version = "~> 0.5.0" # Framework version
    }
  }
}

provider "ironic" {
  url           = "http://localhost:6385/v1"
  microversion  = "1.99"
  auth_strategy = "noauth"
}

# Create a node first
resource "ironic_node" "test_node" {
  name   = "test-node"
  driver = "fake-hardware"
}

# Basic portgroup configuration
resource "ironic_port_group" "basic" {
  name      = "test-portgroup-basic"
  node_uuid = ironic_node.test_node.id
  mode      = "active-backup" # Default mode
}

# Portgroup with custom configuration
resource "ironic_port_group" "advanced" {
  name      = "test-portgroup-advanced"
  node_uuid = ironic_node.test_node.id
  mode      = "802.3ad"
  address   = "aa:bb:cc:dd:ee:ff"

  extra = {
    bond_mode   = "LACP"
    lacp_rate   = "fast"
    mii_mon     = "100"
    environment = "production"
    created_by  = "terraform"
  }
}

# Outputs to show the created resources
output "basic_portgroup_id" {
  value = ironic_port_group.basic.id
}

output "advanced_portgroup_id" {
  value = ironic_port_group.advanced.id
}

output "advanced_portgroup_extra" {
  value = ironic_port_group.advanced.extra
}
