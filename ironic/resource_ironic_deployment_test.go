//go:build acceptance
// +build acceptance

package ironic

import (
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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Creates a node, and an allocation that should use it
func TestAccIronicDeployment(t *testing.T) {
	var node nodes.Node

	nodeName := th.RandomString("TerraformACC-Node-", 8)
	allocationName := th.RandomString("TerraformACC-Allocation-", 8)
	resourceClass := th.RandomString("baremetal-", 8)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccDeploymentDestroy,
		Steps: []resource.TestStep{
			// Create a test deployment
			{
				Config: testAccDeploymentResource(nodeName, resourceClass, allocationName),
				Check: resource.ComposeTestCheckFunc(
					CheckNodeExists("ironic_node_v1."+nodeName, &node),
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
	client, err := testAccProvider.Meta().(*Clients).GetIronicClient()
	if err != nil {
		return err
	}

	for _, rs := range state.RootModule().Resources {
		if rs.Type != "ironic_node_v1" {
			continue
		}

		_, err := nodes.Get(client, rs.Primary.ID).Extract()
		if _, ok := err.(gophercloud.ErrDefault404); !ok {
			return fmt.Errorf("unexpected error: %s, expected 404", err)
		}
	}

	return nil
}

func testAccDeploymentResource(node, resourceClass, allocation string) string {
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
