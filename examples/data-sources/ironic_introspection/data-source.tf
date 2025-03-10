data "ironic_introspection" "openshift-master-0" {
  uuid = ironic_node_v1.openshift-master-0.id
}
