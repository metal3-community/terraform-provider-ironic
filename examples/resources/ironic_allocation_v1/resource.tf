# Basic allocation with resource class
resource "ironic_allocation_v1" "basic_allocation" {
  resource_class = "baremetal"
}

# Named allocation with specific candidate nodes
resource "ironic_allocation_v1" "named_allocation" {
  name           = "my-allocation"
  resource_class = "compute"

  candidate_nodes = [
    "550e8400-e29b-41d4-a716-446655440000",
    "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
  ]
}

# Allocation with traits and extra metadata
resource "ironic_allocation_v1" "advanced_allocation" {
  name           = "gpu-allocation"
  resource_class = "gpu-compute"

  traits = [
    "CUSTOM_GPU",
    "CUSTOM_NVME_SSD",
    "HW_CPU_X86_VMX"
  ]

  extra = {
    project_id  = "my-project"
    environment = "production"
  }
}

# Multiple allocations for a cluster
resource "ironic_allocation_v1" "cluster_allocation" {
  count = 3

  name           = "master-${count.index}"
  resource_class = "baremetal"

  traits = [
    "CUSTOM_CONTROL_PLANE",
  ]

  extra = {
    role  = "master"
    index = tostring(count.index)
  }
}
