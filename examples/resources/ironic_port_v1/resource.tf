resource "ironic_port_v1" "openshift-master-0-port-0" {
  node_uuid   = ironic_node_v1.openshift-master-0.id
  pxe_enabled = true
  address     = "00:bb:4a:d0:5e:38"
}
