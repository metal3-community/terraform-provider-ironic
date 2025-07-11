data "ironic_inventory" "default-master-0" {
  uuid = ironic_node.default-master-0.id
}

# Example output usage
output "cpu_info" {
  value = {
    architecture = data.ironic_inventory.default-master-0.cpu.architecture
    count        = data.ironic_inventory.default-master-0.cpu.count
    model_name   = data.ironic_inventory.default-master-0.cpu.model_name
  }
}

output "memory_mb" {
  value = data.ironic_inventory.default-master-0.memory.physical_mb
}

output "disk_devices" {
  value = [
    for disk in data.ironic_inventory.default-master-0.disks : {
      name   = disk.name
      size   = disk.size
      model  = disk.model
      serial = disk.serial
    }
  ]
}

output "network_interfaces" {
  value = [
    for nic in data.ironic_inventory.default-master-0.nics : {
      name = nic.name
      mac  = nic.mac
      ipv4 = nic.ipv4
    }
  ]
}
