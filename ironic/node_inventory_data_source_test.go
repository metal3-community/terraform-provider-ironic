//go:build acceptance
// +build acceptance

package ironic

import (
	"fmt"
	"testing"

	th "github.com/appkins-org/terraform-provider-ironic/testhelper"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccIntrospectionFramework creates a node resource and verifies the framework-based introspection data source
// returns the inventory information from the Ironic nodes API.
func TestAccIntrospectionFramework(t *testing.T) {
	nodeName := th.RandomString("TerraformACC-Node-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIntrospectionFrameworkConfig(nodeName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"data.ironic_introspection.test-data",
						"id",
					),
				),
			},
		},
	})
}

func testAccIntrospectionFrameworkConfig(nodeName string) string {
	return fmt.Sprintf(`
resource "ironic_node" "test-node" {
  name           = "%s"
  driver         = "fake-hardware"
  target_provision_state = "manageable"
}

data "ironic_introspection" "test-data" {
  uuid = ironic_node.test-node.id
}
`, nodeName)
}
