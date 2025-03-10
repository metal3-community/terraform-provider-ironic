resource "ironic_allocation_v1" "openshift-master-allocation" {
  name  = "master-${count.index}"
  count = 3

  resource_class = "baremetal"

  candidate_nodes = [
    ironic_node_v1.openshift-master-0.id,
    ironic_node_v1.openshift-master-1.id,
    ironic_node_v1.openshift-master-2.id,
  ]

  traits = [
    "CUSTOM_FOO",
  ]
}
