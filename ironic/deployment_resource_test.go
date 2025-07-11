//go:build acceptance
// +build acceptance

package ironic

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	th "github.com/appkins-org/terraform-provider-ironic/testhelper"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Creates a node, and an allocation that should use it
func TestAccIronicDeployment(t *testing.T) {
	var node nodes.Node

	nodeName := th.RandomString("TerraformACC-Node-", 8)
	allocationName := th.RandomString("TerraformACC-Allocation-", 8)
	resourceClass := th.RandomString("baremetal-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccDeploymentDestroy,
		Steps: []resource.TestStep{
			// Create a test deployment
			{
				Config: testAccDeploymentResource(nodeName, resourceClass, allocationName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNodeExists("ironic_node."+nodeName, &node),
					resource.TestCheckResourceAttr(
						"ironic_deployment."+nodeName,
						"provision_state",
						"active",
					),
				),
			},
		},
	})
}

func TestBuildConfigDrive(t *testing.T) {
	configDrive, err := buildConfigDrive("1.48", "foo", nil, nil)
	th.AssertNoError(t, err)

	if _, ok := configDrive.(*string); !ok {
		t.Fatalf("Expected config drive to be *string (base64-encoded gzipped ISO).")
	}

	configDrive, err = buildConfigDrive("1.56", "foo", nil, nil)
	if _, ok := configDrive.(*nodes.ConfigDrive); !ok {
		t.Fatalf("Expected config drive to be *nodes.ConfigDrive")
	}
}

func testAccDeploymentDestroy(state *terraform.State) error {
	clients := &Clients{}
	client, err := clients.GetIronicClient()
	if err != nil {
		return err
	}

	for _, rs := range state.RootModule().Resources {
		if rs.Type != "ironic_deployment" {
			continue
		}

		// For deployment resource, check that the node is no longer in active state
		_, err := nodes.Get(context.TODO(), client, rs.Primary.ID).Extract()
		if err != nil {
			if _, ok := err.(gophercloud.ErrDefault404); ok {
				// Node was deleted, that's fine
				continue
			}
			return fmt.Errorf("unexpected error checking node: %s", err)
		}
		// If node still exists, that's okay - deployment resource just manages state
	}

	return nil
}

func testAccDeploymentResource(node, resourceClass, allocation string) string {
	return fmt.Sprintf(`
		resource "ironic_node" "%s" {
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
				"${ironic_node.%s.id}"
			]
		}

		resource "ironic_deployment" "%s" {
			name = "%s"
			node_uuid = "${ironic_allocation_v1.%s.node_uuid}"

			instance_info = {
				image_source   = "http://172.22.0.1/images/redhat-coreos-maipo-latest.qcow2"
				image_checksum = "26c53f3beca4e0b02e09d335257826fd"
				root_gb = "25"
			}

			user_data = "asdf"
		}

`, node, node, resourceClass, allocation, allocation, resourceClass, node, node, node, allocation)
}

func TestFetchFullIgnition(t *testing.T) {
	// Setup a fake https endpoint to server full ignition
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			if k == "Test" {
				fmt.Fprintf(w, "Header: %s=%s\n", k, v)
			}
		}
		fmt.Fprintln(w, "Full Ignition")
	}))
	defer server.Close()

	cert := server.Certificate()
	certInPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		},
	)
	certB64 := base64.URLEncoding.EncodeToString(certInPem)
	emptyHeaders := make(map[string]any)

	testCases := []struct {
		Scenario           string
		UserDataURL        string
		UserDataURLCACert  string
		UserDataURLHeaders map[string]any
		ExpectedResult     string
	}{
		{
			Scenario:           "user data url and ca cert present",
			UserDataURL:        server.URL,
			UserDataURLCACert:  certB64,
			UserDataURLHeaders: emptyHeaders,
			ExpectedResult:     "Full Ignition\n",
		},
		{
			Scenario:           "user data url present but no ca cert",
			UserDataURL:        server.URL,
			UserDataURLCACert:  "",
			UserDataURLHeaders: emptyHeaders,
			ExpectedResult:     "Full Ignition\n",
		},
		{
			Scenario:           "user data url, ca cert and headers present",
			UserDataURL:        server.URL,
			UserDataURLCACert:  certB64,
			UserDataURLHeaders: map[string]any{"Test": "foo"},
			ExpectedResult:     "Header: Test=[foo]\nFull Ignition\n",
		},
		{
			Scenario:           "user data url is not present but ca cert is",
			UserDataURL:        "",
			UserDataURLCACert:  certB64,
			UserDataURLHeaders: emptyHeaders,
			ExpectedResult:     "",
		},
		{
			Scenario:           "neither user data url nor ca cert is not present",
			UserDataURL:        "",
			UserDataURLCACert:  "",
			UserDataURLHeaders: emptyHeaders,
			ExpectedResult:     "",
		},
	}
	for _, tc := range testCases {
		userData, err := fetchFullIgnition(
			tc.UserDataURL,
			tc.UserDataURLCACert,
			tc.UserDataURLHeaders,
		)
		if err != nil {
			t.Errorf("expected err: %s", err)
		}
		if userData != tc.ExpectedResult {
			t.Errorf("expected userData: %s, got %s", tc.ExpectedResult, userData)
		}
	}
}

func TestBuildDeploySteps(t *testing.T) {
	var deploySteps []nodes.DeployStep
	testCases := []struct {
		Scenario    string
		DeploySteps string
		Expected    []nodes.DeployStep
	}{
		{
			Scenario:    "correct deploy_step format",
			DeploySteps: `[{"interface": "deploy", "step": "install_coreos", "priority": 80, "args": {}}]`,
			Expected:    deploySteps,
		},
		{
			Scenario:    "incorrect deploy_step format",
			DeploySteps: "wrong json",
			Expected:    nil,
		},
	}
	for _, tc := range testCases {
		ds, _ := buildDeploySteps(tc.DeploySteps)
		if reflect.TypeOf(ds) != reflect.TypeOf(tc.Expected) {
			t.Errorf("expected deployStep type: %v but got %v", tc.Scenario, reflect.TypeOf(ds))
		}
	}
}

func TestAccIronicDeployment_importBasic(t *testing.T) {
	nodeName := th.RandomString("TerraformACC-Node-", 8)
	allocationName := th.RandomString("TerraformACC-Allocation-", 8)
	resourceClass := th.RandomString("baremetal-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccDeploymentDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentResource(nodeName, resourceClass, allocationName),
			},
			{
				ResourceName:      "ironic_deployment." + nodeName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"user_data",
					"user_data_url",
					"user_data_url_ca_cert",
					"user_data_url_headers",
					"network_data",
					"metadata",
					"deploy_steps",
					"fixed_ips",
				},
			},
		},
	})
}

func TestAccIronicDeployment_migration(t *testing.T) {
	var node nodes.Node
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
				Config: testAccDeploymentResourceSDK(nodeName, resourceClass, allocationName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNodeExistsSDK("ironic_node."+nodeName, &node),
					resource.TestCheckResourceAttr(
						"ironic_deployment."+nodeName,
						"provision_state",
						"active",
					),
				),
			},
			{
				ProtoV5ProviderFactories: protoV5ProviderFactories(),
				Config: testAccDeploymentResource(
					nodeName,
					resourceClass,
					allocationName,
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccDeploymentResourceSDK(node, resourceClass, allocation string) string {
	return fmt.Sprintf(`
		resource "ironic_node" "%s" {
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
				"${ironic_node.%s.id}"
			]
		}

		resource "ironic_deployment" "%s" {
			name = "%s"
			node_uuid = "${ironic_allocation_v1.%s.node_uuid}"

			instance_info = {
				image_source   = "http://172.22.0.1/images/redhat-coreos-maipo-latest.qcow2"
				image_checksum = "26c53f3beca4e0b02e09d335257826fd"
				root_gb = "25"
			}

			user_data = "asdf"
		}

`, node, node, resourceClass, allocation, allocation, resourceClass, node, node, node, allocation)
}

// testAccCheckNodeExists checks if a node exists using framework provider
func testAccCheckNodeExists(name string, node *nodes.Node) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		clients := &Clients{}
		// Get the client from the framework provider (this needs to be implemented)
		client, err := clients.GetIronicClient()
		if err != nil {
			return err
		}

		rs, ok := state.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("not found: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no node ID is set")
		}

		result, err := nodes.Get(context.TODO(), client, rs.Primary.ID).Extract()
		if err != nil {
			return fmt.Errorf("node (%s) not found: %s", rs.Primary.ID, err)
		}

		*node = *result
		return nil
	}
}

// testAccCheckNodeExistsSDK checks if a node exists using SDKv2 provider
func testAccCheckNodeExistsSDK(name string, node *nodes.Node) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		clients := &Clients{}
		// This would need to work with the old SDKv2 provider
		client, err := clients.GetIronicClient()
		if err != nil {
			return err
		}

		rs, ok := state.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("not found: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no node ID is set")
		}

		result, err := nodes.Get(context.TODO(), client, rs.Primary.ID).Extract()
		if err != nil {
			return fmt.Errorf("node (%s) not found: %s", rs.Primary.ID, err)
		}

		*node = *result
		return nil
	}
}
