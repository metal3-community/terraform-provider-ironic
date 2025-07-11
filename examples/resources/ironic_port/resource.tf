# Create a node first
resource "ironic_node" "example_node" {
  name   = "example-node"
  driver = "ipmi"

  driver_info = {
    ipmi_address  = "192.168.1.100"
    ipmi_username = "admin"
    ipmi_password = "password"
  }
}

# Basic port configuration
resource "ironic_port" "basic_port" {
  node_uuid   = ironic_node.example_node.id
  address     = "00:bb:4a:d0:5e:38"
  pxe_enabled = true
}

# Port with local link connection for switch connectivity
resource "ironic_port" "switch_port" {
  node_uuid   = ironic_node.example_node.id
  address     = "00:bb:4a:d0:5e:39"
  pxe_enabled = false

  local_link_connection = {
    switch_id   = "0a:1b:2c:3d:4e:5f"
    port_id     = "Ethernet3/1"
    switch_info = "switch1"
  }

  physical_network = "provisioning"
}

# Smart NIC port with extra metadata
resource "ironic_port" "smart_nic_port" {
  address      = "00:bb:4a:d0:5e:40"
  is_smart_nic = true

  extra = {
    description = "Smart NIC port for acceleration"
    vlan_id     = 100
  }
}
