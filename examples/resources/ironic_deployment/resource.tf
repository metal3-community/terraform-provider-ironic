resource "ironic_deployment" "masters" {
  count     = 3
  node_uuid = element(ironic_allocation_v1.openshift-master-allocation.*.node_uuid, count.index)

  instance_info = {
    image_source   = "http://172.22.0.1/images/redhat-coreos-maipo-latest.qcow2"
    image_checksum = "26c53f3beca4e0b02e09d335257826fd"
    capabilities   = "boot_option:local,secure_boot:true"
  }

  user_data    = var.user_data
  network_data = var.network_data
  metadata     = var.metadata
}
