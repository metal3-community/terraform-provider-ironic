package ironic

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	util "github.com/gophercloud/utils/openstack/baremetal/v1/nodes"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Schema resource definition for an Ironic deployment.
func resourceDeployment() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceDeploymentCreate,
		ReadContext:   resourceDeploymentRead,
		DeleteContext: resourceDeploymentDelete,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"node_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"instance_info": {
				Type:     schema.TypeMap,
				Required: true,
				ForceNew: true,
			},
			"deploy_steps": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"user_data": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"user_data_url": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"user_data_url_ca_cert": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"user_data_url_headers": {
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
			},
			"network_data": {
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
			},
			"metadata": {
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
			},
			"fixed_ips": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Schema{
					Type: schema.TypeMap,
				},
			},
			"provision_state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"last_error": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

// Create an deployment, including driving Ironic's state machine.
func resourceDeploymentCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	// Reload the resource before returning
	defer func() { _ = resourceDeploymentRead(ctx, d, meta) }()

	nodeUUID := d.Get("node_uuid").(string)
	// Set instance info
	instanceInfo := d.Get("instance_info").(map[string]any)
	if instanceInfo != nil {
		instanceInfoCapabilities, found := instanceInfo["capabilities"]
		capabilities := make(map[string]string)
		if found {
			for _, e := range strings.Split(instanceInfoCapabilities.(string), ",") {
				parts := strings.Split(e, ":")
				if len(parts) != 2 {
					return diag.FromErr(fmt.Errorf(
						"error while parsing capabilities: %s, the correct format is key:value",
						e,
					))
				}
				capabilities[parts[0]] = parts[1]

			}
			delete(instanceInfo, "capabilities")
		}
		delete(instanceInfo, "fixed_ips")
		_, err := UpdateNode(ctx, client, nodeUUID, nodes.UpdateOpts{
			nodes.UpdateOperation{
				Op:    nodes.AddOp,
				Path:  "/instance_info",
				Value: instanceInfo,
			},
		})
		if err != nil {
			return diag.FromErr(fmt.Errorf("could not update instance info: %s", err))
		}

		if len(capabilities) != 0 {
			_, err = UpdateNode(ctx, client, nodeUUID, nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/capabilities",
					Value: capabilities,
				},
			})
			if err != nil {
				return diag.FromErr(fmt.Errorf("could not update instance info capabilities: %s", err))
			}
		}
	}

	d.SetId(nodeUUID)

	// deploy_steps is a json string
	dSteps := d.Get("deploy_steps").(string)
	var deploySteps []nodes.DeployStep
	if len(dSteps) > 0 {
		deploySteps, err = buildDeploySteps(dSteps)
		if err != nil {
			return diag.FromErr(fmt.Errorf("could not fetch deploy steps: %s", err))
		}
	}

	fixedIps, ok := d.Get("fixed_ips").([]any)
	if ok && len(fixedIps) > 0 {
		// Update the node with fixed_ips
		_, err = UpdateNode(ctx, client, nodeUUID, nodes.UpdateOpts{
			nodes.UpdateOperation{
				Op:    nodes.AddOp,
				Path:  "/instance_info/fixed_ips",
				Value: fixedIps,
			},
		})
		if err != nil {
			return diag.FromErr(fmt.Errorf("could not update fixed_ips: %s", err))
		}
	}

	userData := d.Get("user_data").(string)
	userDataURL := d.Get("user_data_url").(string)
	userDataCaCert := d.Get("user_data_url_ca_cert").(string)
	userDataHeaders := d.Get("user_data_url_headers").(map[string]any)

	// if user_data_url is specified in addition to user_data, use the former
	ignitionData, err := fetchFullIgnition(userDataURL, userDataCaCert, userDataHeaders)
	if err != nil {
		return diag.FromErr(fmt.Errorf("could not fetch data from user_data_url: %s", err))
	}
	if ignitionData != "" {
		userData = ignitionData
	}

	configDrive, err := buildConfigDrive(client.Microversion,
		userData,
		d.Get("network_data").(map[string]any),
		d.Get("metadata").(map[string]any))
	if err != nil {
		return diag.FromErr(err)
	}

	// Deploy the node - drive Ironic state machine until node is 'active'
	if err := ChangeProvisionStateToTarget(
		ctx,
		client,
		nodeUUID,
		"active",
		&configDrive,
		deploySteps,
		nil,
	); err != nil {
		return diag.FromErr(fmt.Errorf("could not deploy node: %s", err))
	}
	return nil
}

// fetchFullIgnition gets full igntion from the URL and cert passed to it and returns userdata as a string.
func fetchFullIgnition(
	userDataURL string,
	userDataCaCert string,
	userDataHeaders map[string]any,
) (string, error) {
	// Send full ignition, if the URL is specified
	if userDataURL != "" {
		caCertPool := x509.NewCertPool()
		transport := &http.Transport{}

		if userDataCaCert != "" {
			caCert, err := base64.StdEncoding.DecodeString(userDataCaCert)
			if err != nil {
				log.Printf("could not decode user_data_url_ca_cert: %s", err)
				return "", err
			}
			caCertPool.AppendCertsFromPEM(caCert)
			// disable "G402 (CWE-295): TLS MinVersion too low. (Confidence: HIGH, Severity: HIGH)"
			// #nosec G402
			transport.TLSClientConfig = &tls.Config{RootCAs: caCertPool}
		} else {
			// Disable certificate verification
			// disable "G402 (CWE-295): TLS MinVersion too low. (Confidence: HIGH, Severity: HIGH)"
			// #nosec G402
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}

		client := retryablehttp.NewClient()
		client.HTTPClient.Transport = transport

		// Get the data
		req, err := retryablehttp.NewRequest("GET", userDataURL, nil)
		if err != nil {
			log.Printf("could not get user_data_url: %s", err)
			return "", err
		}
		for k, v := range userDataHeaders {
			req.Header.Add(k, v.(string))
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("could not get user_data_url: %s", err)
			return "", err
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("could not close user_data_url response body: %s", err)
			}
		}()
		var userData []byte
		userData, err = io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("could not read user_data_url: %s", err)
			return "", err
		}
		return string(userData), nil
	}
	return "", nil
}

// buildDeploySteps handles customized deploy steps.
func buildDeploySteps(steps string) ([]nodes.DeployStep, error) {
	var deploySteps []nodes.DeployStep
	err := json.Unmarshal([]byte(steps), &deploySteps)
	if err != nil {
		log.Printf("could not unmarshal deploy_steps.\n")
		return nil, err
	}

	return deploySteps, nil
}

func convertNetworkData(networkData map[string]any) (map[string]any, error) {
	networkDataU := map[string]any{}
	for k, v := range networkData {
		if vs, ok := v.(string); ok {
			firstChar := vs[0]
			switch firstChar {
			case '[':
				vd := []any{}
				if err := json.Unmarshal([]byte(vs), &vd); err != nil {
					return nil, fmt.Errorf(
						"error unmarshalling network data for key %s: %v",
						k,
						err,
					)
				}
				networkDataU[k] = vd
			case '{':
				// If the value is a JSON string, unmarshal it into an interface
				vd := map[string]any{}
				if err := json.Unmarshal([]byte(vs), &vd); err != nil {
					return nil, fmt.Errorf(
						"error unmarshalling network data for key %s: %v",
						k,
						err,
					)
				}
				networkDataU[k] = vd
			default:
				// If the value is a string, just use it as is
				networkDataU[k] = vs
			}
		} else {
			networkDataU[k] = v
		}
	}
	return networkDataU, nil
}

// buildConfigDrive handles building a config drive appropriate for the Ironic version we are using.  Newer versions
// support sending the user data directly, otherwise we need to build an ISO image.
func buildConfigDrive(
	apiVersion string,
	userData any,
	networkData, metaData map[string]any,
) (any, error) {
	networkDataU, err := convertNetworkData(networkData)
	if err != nil {
		return nil, fmt.Errorf("error converting network data: %v", err)
	}

	actual, err := version.NewVersion(apiVersion)
	if err != nil {
		return nil, err
	}
	minimum, err := version.NewVersion("1.56")
	if err != nil {
		return nil, err
	}

	if minimum.GreaterThan(actual) {
		userDataStr, ok := userData.(string)
		if !ok {
			return nil, fmt.Errorf(
				"user_data must be a string for Ironic versions < 1.56, got %T",
				userData,
			)
		}
		// Create config drive ISO directly with gophercloud/utils
		configDriveData := util.ConfigDrive{
			UserData:    util.UserDataString(userDataStr),
			NetworkData: networkDataU,
			MetaData:    metaData,
		}
		configDriveISO, err := configDriveData.ToConfigDrive()
		if err != nil {
			return nil, err
		}
		return &configDriveISO, nil
	}
	// Let Ironic handle creating the config drive
	return &nodes.ConfigDrive{
		UserData:    userData,
		NetworkData: networkDataU,
		MetaData:    metaData,
	}, nil
}

// Read the deployment's data from Ironic.
func resourceDeploymentRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	// Ensure node exists first
	id := d.Get("node_uuid").(string)
	result, err := nodes.Get(ctx, client, id).Extract()
	if err != nil {
		return diag.FromErr(fmt.Errorf("could not find node %s: %s", id, err))
	}

	err = d.Set("provision_state", result.ProvisionState)
	if err != nil {
		return diag.FromErr(err)
	}
	return diag.FromErr(d.Set("last_error", result.LastError))
}

// Delete an deployment from Ironic - this cleans the node and returns it's state to 'available'.
func resourceDeploymentDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	if err = ChangeProvisionStateToTarget(
		ctx,
		client,
		d.Id(),
		"deleted",
		nil,
		nil,
		nil,
	); err != nil {
		return diag.FromErr(fmt.Errorf("could not delete deployment: %s", err))
	}
	return diag.Diagnostics{}
}
