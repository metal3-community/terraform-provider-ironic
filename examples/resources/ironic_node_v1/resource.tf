resource "ironic_node_v1" "openshift-master-0" {
  name = "openshift-master-0"

  inspect   = true # Perform inspection
  clean     = true # Clean the node
  available = true # Make the node 'available'

  ports = [
    {
      "address"     = "00:bb:4a:d0:5e:38"
      "pxe_enabled" = "true"
    },
  ]

  properties = {
    "local_gb" = "50"
    "cpu_arch" = "x86_64"
  }

  driver = "ipmi"
  driver_info = {
    "ipmi_port"      = "6230"
    "ipmi_username"  = "admin"
    "ipmi_password"  = "password"
    "ipmi_address"   = "192.168.111.1"
    "deploy_kernel"  = "http://172.22.0.1/images/ironic-python-agent.kernel"
    "deploy_ramdisk" = "http://172.22.0.1/images/ironic-python-agent.initramfs"
  }
}
