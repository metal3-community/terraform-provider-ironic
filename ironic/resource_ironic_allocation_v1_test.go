//go:build acceptance
// +build acceptance

package ironic

import (
	"context"
	"fmt"
	"testing"

	th "github.com/appkins-org/terraform-provider-ironic/testhelper"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/allocations"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Creates a node, and an allocation that should use it
func TestAccIronicAllocation(t *testing.T) {
	var node nodes.Node
	var allocation allocations.Allocation

	nodeName := th.RandomString("TerraformACC-Node-", 8)
	allocationName := th.RandomString("TerraformACC-Allocation-", 8)
	resourceClass := th.RandomString("baremetal-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccAllocationDestroy,
		Steps: []resource.TestStep{
			// Create a test allocation, and check that it allocates the node we expected it to
			{
				Config: testAccAllocationResource(nodeName, resourceClass, allocationName),
				Check: resource.ComposeTestCheckFunc(
					CheckNodeExists("ironic_node_v1."+nodeName, &node),
					testAccCheckAllocationExists(
						"ironic_allocation_v1."+allocationName,
						&allocation,
					),

					// Ensure that the allocation is active, and found the node we expected
					resource.TestCheckResourceAttr(
						"ironic_allocation_v1."+allocationName,
						"state",
						"active",
					),
					resource.TestCheckResourceAttrPtr(
						"ironic_allocation_v1."+allocationName,
						"node_uuid",
						&node.UUID,
					),
				),
			},

			// Ensure that the node's instance_uuid was updated
			{
				Config: testAccAllocationResource(nodeName, resourceClass, allocationName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPtr(
						"ironic_node_v1."+nodeName,
						"instance_uuid",
						&allocation.UUID,
					),
				),
			},
		},
	})
}

func TestAccIronicAllocationV1_importBasic(t *testing.T) {
	nodeName := th.RandomString("TerraformACC-Node-", 8)
	allocationName := th.RandomString("TerraformACC-Allocation-", 8)
	resourceClass := th.RandomString("baremetal-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccAllocationDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAllocationResource(nodeName, resourceClass, allocationName),
			},
			{
				ResourceName:      "ironic_allocation_v1." + allocationName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccIronicAllocationV1_migration(t *testing.T) {
	var allocation allocations.Allocation
	nodeName := th.RandomString("TerraformACC-Node-", 8)
	allocationName := th.RandomString("TerraformACC-Allocation-", 8)
	resourceClass := th.RandomString("baremetal-", 8)

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
				Config: testAccAllocationResourceSDK(nodeName, resourceClass, allocationName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAllocationExistsSDK("ironic_allocation_v1."+allocationName, &allocation),
					resource.TestCheckResourceAttr("ironic_allocation_v1."+allocationName, "state", "active"),
				),
			},
			{
				ProtoV5ProviderFactories: protoV5ProviderFactories(),
				Config:                   testAccAllocationResource(nodeName, resourceClass, allocationName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAllocationExists("ironic_allocation_v1."+allocationName, &allocation),
					resource.TestCheckResourceAttr("ironic_allocation_v1."+allocationName, "state", "active"),
				),
			},
		},
	})
}

// Calls gophercloud directly to ensure the allocation exists
func testAccCheckAllocationExists(
	name string,
	allocation *allocations.Allocation,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		clients := &Clients{}
		err := clients.loadIronicClient(testAccProvider.Meta().(*Config))
		if err != nil {
			return err
		}

		client, err := clients.GetIronicClient()
		if err != nil {
			return err
		}

		rs, ok := state.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("not found: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no allocation ID is set")
		}

		result, err := allocations.Get(context.TODO(), client, rs.Primary.ID).Extract()
		if err != nil {
			return fmt.Errorf("allocation (%s) not found: %s", rs.Primary.ID, err)
		}

		*allocation = *result

		return nil
	}
}

// Calls gophercloud to ensure the allocation was destroyed
func testAccAllocationDestroy(state *terraform.State) error {
	clients := &Clients{}
	err := clients.loadIronicClient(testAccProvider.Meta().(*Config))
	if err != nil {
		return err
	}

	client, err := clients.GetIronicClient()
	if err != nil {
		return err
	}

	for _, rs := range state.RootModule().Resources {
		if rs.Type != "ironic_allocation_v1" {
			continue
		}

		_, err := allocations.Get(context.TODO(), client, rs.Primary.ID).Extract()
		if _, ok := err.(gophercloud.ErrDefault404); !ok {
			return fmt.Errorf("unexpected error: %s, expected 404", err)
		}
	}

	return nil
}

// Create the resource declaration for a node, and an allocation that should consume it.
func testAccAllocationResource(node, resourceClass, allocation string) string {
	return fmt.Sprintf(`
		resource "ironic_node_v1" "%s" {
			name = "%s"
			driver = "fake-hardware"
			available = true
			target_power_state = "power off"

			boot_interface = "fake"
			deploy_interface = "fake"
			management_interface = "fake"
			power_interface = "fake"
			resource_class = "%s"
			vendor_interface = "no-vendor"
		}

		resource "ironic_allocation_v1" "%s" {
			name = "%s"
			resource_class = "%s"
			candidate_nodes = [
				"${ironic_node_v1.%s.id}"
			]
		}`, node, node, resourceClass, allocation, allocation, resourceClass, node)
}

func testAccCheckAllocationExistsSDK(name string, allocation *allocations.Allocation) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		client, err := testAccProvider.Meta().(*Clients).GetIronicClient()
		if err != nil {
			return err
		}

		rs, ok := state.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("not found: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no allocation ID is set")
		}

		result, err := allocations.Get(context.TODO(), client, rs.Primary.ID).Extract()
		if err != nil {
			return fmt.Errorf("allocation (%s) not found: %s", rs.Primary.ID, err)
		}

		*allocation = *result

		return nil
	}
}

func testAccAllocationResourceSDK(node, resourceClass, allocation string) string {
	return testAccAllocationResource(node, resourceClass, allocation)
}
