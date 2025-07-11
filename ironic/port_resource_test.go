//go:build acceptance
// +build acceptance

package ironic

import (
	"context"
	"fmt"
	"testing"

	th "github.com/appkins-org/terraform-provider-ironic/testhelper"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccIronicPortV1_basic(t *testing.T) {
	var port ports.Port
	portName := th.RandomString("TerraformACC-Port-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccCheckPortV1Destroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPortV1Basic(portName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckPortV1Exists("ironic_port.port_1", &port),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"address",
						"52:54:00:cf:2d:31",
					),
					resource.TestCheckResourceAttr("ironic_port.port_1", "pxe_enabled", "true"),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"is_smart_nic",
						"false",
					),
				),
			},
		},
	})
}

func TestAccIronicPortV1_withNode(t *testing.T) {
	var port ports.Port
	var node nodes.Node
	portName := th.RandomString("TerraformACC-Port-", 8)
	nodeName := th.RandomString("TerraformACC-Node-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccCheckPortV1Destroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPortV1WithNode(nodeName, portName),
				Check: resource.ComposeTestCheckFunc(
					CheckNodeExists("ironic_node.node_1", &node),
					testAccCheckPortV1Exists("ironic_port.port_1", &port),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"address",
						"52:54:00:cf:2d:32",
					),
					resource.TestCheckResourceAttrPtr(
						"ironic_port.port_1",
						"node_uuid",
						&node.UUID,
					),
				),
			},
		},
	})
}

func TestAccIronicPortV1_localLinkConnection(t *testing.T) {
	var port ports.Port
	portName := th.RandomString("TerraformACC-Port-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccCheckPortV1Destroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPortV1LocalLinkConnection(portName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckPortV1Exists("ironic_port.port_1", &port),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"address",
						"52:54:00:cf:2d:33",
					),
				),
			},
		},
	})
}

func TestAccIronicPortV1_update(t *testing.T) {
	var port ports.Port
	portName := th.RandomString("TerraformACC-Port-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccCheckPortV1Destroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPortV1Basic(portName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckPortV1Exists("ironic_port.port_1", &port),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"address",
						"52:54:00:cf:2d:31",
					),
					resource.TestCheckResourceAttr("ironic_port.port_1", "pxe_enabled", "true"),
				),
			},
			{
				Config: testAccPortV1Update(portName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckPortV1Exists("ironic_port.port_1", &port),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"address",
						"52:54:00:cf:2d:31",
					),
					resource.TestCheckResourceAttr("ironic_port.port_1", "pxe_enabled", "false"),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"physical_network",
						"provisioning",
					),
				),
			},
		},
	})
}

func TestAccIronicPortV1_importBasic(t *testing.T) {
	portName := th.RandomString("TerraformACC-Port-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccCheckPortV1Destroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPortV1Basic(portName),
			},
			{
				ResourceName:      "ironic_port.port_1",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccIronicPortV1_migration(t *testing.T) {
	var port ports.Port
	portName := th.RandomString("TerraformACC-Port-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"ironic": {
						VersionConstraint: "~> 1.3.0", // Last SDKv2 version
						Source:            "terraform-providers/ironic",
					},
				},
				Config: testAccPortV1BasicSDK(portName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckPortV1ExistsSDK("ironic_port.port_1", &port),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"address",
						"52:54:00:cf:2d:31",
					),
				),
			},
			{
				ProtoV5ProviderFactories: protoV5ProviderFactories(),
				Config:                   testAccPortV1Basic(portName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckPortV1Exists("ironic_port.port_1", &port),
					resource.TestCheckResourceAttr(
						"ironic_port.port_1",
						"address",
						"52:54:00:cf:2d:31",
					),
				),
			},
		},
	})
}

func testAccCheckPortV1Destroy(s *terraform.State) error {
	ironicClient, err := testAccProvider.Meta().(*Clients).GetIronicClient()
	if err != nil {
		return err
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ironic_port" {
			continue
		}

		_, err := ports.Get(context.TODO(), ironicClient, rs.Primary.ID).Extract()
		if err == nil {
			return fmt.Errorf("Port still exists")
		}
		if _, ok := err.(gophercloud.ErrDefault404); !ok {
			return err
		}
	}

	return nil
}

func testAccCheckPortV1Exists(n string, port *ports.Port) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		clients := &Clients{}
		err := clients.loadIronicClient(testAccProvider.Meta().(*Config))
		if err != nil {
			return err
		}

		ironicClient, err := clients.GetIronicClient()
		if err != nil {
			return err
		}

		found, err := ports.Get(context.TODO(), ironicClient, rs.Primary.ID).Extract()
		if err != nil {
			return err
		}

		if found.UUID != rs.Primary.ID {
			return fmt.Errorf("Port not found")
		}

		*port = *found

		return nil
	}
}

func testAccCheckPortV1ExistsSDK(n string, port *ports.Port) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		ironicClient, err := testAccProvider.Meta().(*Clients).GetIronicClient()
		if err != nil {
			return err
		}

		found, err := ports.Get(context.TODO(), ironicClient, rs.Primary.ID).Extract()
		if err != nil {
			return err
		}

		if found.UUID != rs.Primary.ID {
			return fmt.Errorf("Port not found")
		}

		*port = *found

		return nil
	}
}

func testAccPortV1Basic(portName string) string {
	return fmt.Sprintf(`
resource "ironic_port" "port_1" {
  address = "52:54:00:cf:2d:31"
  pxe_enabled = true
}`)
}

func testAccPortV1BasicSDK(portName string) string {
	return testAccPortV1Basic(portName)
}

func testAccPortV1WithNode(nodeName, portName string) string {
	return fmt.Sprintf(`
resource "ironic_node" "node_1" {
  name   = "%s"
  driver = "fake-hardware"
}

resource "ironic_port" "port_1" {
  node_uuid   = ironic_node.node_1.id
  address     = "52:54:00:cf:2d:32"
  pxe_enabled = false
}`, nodeName)
}

func testAccPortV1LocalLinkConnection(portName string) string {
	return fmt.Sprintf(`
resource "ironic_port" "port_1" {
  address = "52:54:00:cf:2d:33"
  local_link_connection = {
    switch_id   = "0a:1b:2c:3d:4e:5f"
    port_id     = "Ethernet3/1"
    switch_info = "switch1"
  }
}`)
}

func testAccPortV1Update(portName string) string {
	return fmt.Sprintf(`
resource "ironic_port" "port_1" {
  address          = "52:54:00:cf:2d:31"
  pxe_enabled      = false
  physical_network = "provisioning"
}`)
}
