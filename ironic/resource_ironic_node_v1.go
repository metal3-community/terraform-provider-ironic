package ironic

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic"
)

// Schema resource definition for an Ironic node.
func resourceNodeV1() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNodeV1Create,
		ReadContext:   resourceNodeV1Read,
		UpdateContext: resourceNodeV1Update,
		DeleteContext: resourceNodeV1Delete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceNodeV1Import,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"boot_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"clean": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"conductor_group": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"console_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"deploy_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"driver": {
				Type:     schema.TypeString,
				Required: true,
			},
			"driver_info": {
				Type:     schema.TypeMap,
				Optional: true,
				DiffSuppressFunc: func(k, old, _ string, _ *schema.ResourceData) bool {
					/* FIXME: Password updates aren't considered. How can I know if the *local* data changed? */
					/* FIXME: Support drivers other than IPMI */
					if k == "driver_info.ipmi_password" && old == "******" {
						return true
					}

					return false
				},

				// driver_info could contain passwords
				Sensitive: true,
			},
			"instance_info": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"properties": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"root_device": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"extra": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"inspect_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"instance_uuid": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"inspect": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"available": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"manage": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"management_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"network_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"power_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"raid_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"rescue_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"resource_class": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"storage_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"vendor_interface": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"owner": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"ports": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeMap,
				},
			},
			"provision_state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"power_state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"target_power_state": {
				Type:     schema.TypeString,
				Optional: true,

				// If power_state is same as target_power_state, we have no changes to apply
				DiffSuppressFunc: func(_, _, newState string, d *schema.ResourceData) bool {
					return newState == d.Get("power_state").(string)
				},
			},
			"power_state_timeout": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"raid_config": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"bios_settings": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

// Create a node, including driving Ironic's state machine.
func resourceNodeV1Create(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := meta.(*Clients).GetIronicClient()
	if err != nil {
		return diag.FromErr(err)
	}

	// Create the node object in Ironic
	createOpts := schemaToCreateOpts(ctx, d)
	result, err := nodes.Create(ctx, client, createOpts).Extract()
	if err != nil {
		d.SetId("")
		return diag.FromErr(err)
	}

	// Setting the ID is what tells terraform we were successful in creating the node
	log.Printf("[DEBUG] Node created with ID %s\n", d.Id())
	d.SetId(result.UUID)

	// Create ports as part of the node object - you may also use the native port resource
	portSet := d.Get("ports").(*schema.Set)
	if portSet != nil {
		portList := portSet.List()
		for _, portInterface := range portList {
			port := portInterface.(map[string]any)

			// Terraform map can't handle bool... seriously.
			var pxeEnabled bool
			if port["pxe_enabled"] != nil {
				if port["pxe_enabled"] == "true" {
					pxeEnabled = true
				} else {
					pxeEnabled = false
				}
			}
			// FIXME: All values other than address and pxe
			portCreateOpts := ports.CreateOpts{
				NodeUUID:   d.Id(),
				Address:    port["address"].(string),
				PXEEnabled: &pxeEnabled,
			}
			_, err := ports.Create(ctx, client, portCreateOpts).Extract()
			if err != nil {
				_ = resourcePortV1Read(ctx, d, meta)
				return diag.FromErr(err)
			}
		}
	}

	if instanceInfo := d.Get("instance_info").(map[string]any); len(instanceInfo) > 0 {
		_, err = nodes.Update(ctx, client, d.Id(), nodes.UpdateOpts{
			nodes.UpdateOperation{
				Op:    nodes.ReplaceOp,
				Path:  "/instance_info",
				Value: instanceInfo,
			},
		}).Extract()
		if err != nil {
			return diag.FromErr(fmt.Errorf("could not update instance_info: %s", err))
		}
	}

	// Make node manageable
	if d.Get("manage").(bool) || d.Get("clean").(bool) || d.Get("inspect").(bool) {
		if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "manage", nil, nil, nil); err != nil {
			return diag.FromErr(fmt.Errorf("could not manage: %s", err))
		}
	}

	// Clean node
	if d.Get("clean").(bool) {
		if err := setRAIDConfig(ctx, client, d); err != nil {
			return diag.FromErr(fmt.Errorf("fail to set raid config: %s", err))
		}

		var cleanSteps []nodes.CleanStep
		if cleanSteps, err = buildManualCleaningSteps(d.Get("raid_interface").(string), d.Get("raid_config").(string), d.Get("bios_settings").(string)); err != nil {
			return diag.FromErr(fmt.Errorf("fail to build raid clean steps: %s", err))
		}

		if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "clean", nil, nil, cleanSteps); err != nil {
			return diag.FromErr(fmt.Errorf("could not clean: %s", err))
		}
	}

	// Inspect node
	if inspect, ok := d.Get("inspect").(bool); ok && inspect {
		if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "inspect", nil, nil, nil); err != nil {
			return diag.FromErr(fmt.Errorf("could not inspect: %s", err))
		}
	}

	// Make node available
	if available, ok := d.Get("available").(bool); ok && available {
		if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "provide", nil, nil, nil); err != nil {
			return diag.FromErr(fmt.Errorf("could not make node available: %s", err))
		}
	}

	// Change power state, if required
	if targetPowerState, ok := d.Get("target_power_state").(string); ok && targetPowerState != "" {
		err := changePowerState(ctx, client, d, nodes.TargetPowerState(targetPowerState))
		if err != nil {
			return diag.FromErr(fmt.Errorf("could not change power state: %s", err))
		}
	}

	return resourceNodeV1Read(ctx, d, meta)
}

// Read the node's data from Ironic.
func resourceNodeV1Read(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	clients, ok := meta.(*Clients)
	if !ok {
		return diag.FromErr(fmt.Errorf("expected meta to be of type *Clients, got %T", meta))
	}
	client, err := clients.GetIronicClient()
	if err != nil {
		return diag.FromErr(err)
	}

	node, err := nodes.Get(ctx, client, d.Id()).Extract()
	if err != nil {
		d.SetId("")
		return diag.FromErr(err)
	}

	// TODO: Ironic's Create is different than the Node object itself, GET returns things like the
	//  RaidConfig, we need to add those and handle them in CREATE
	err = d.Set("boot_interface", node.BootInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("conductor_group", node.ConductorGroup)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("console_interface", node.ConsoleInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("deploy_interface", node.DeployInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("driver", node.Driver)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("driver_info", node.DriverInfo)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("extra", node.Extra)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("inspect_interface", node.InspectInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("instance_uuid", node.InstanceUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("management_interface", node.ManagementInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("name", node.Name)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("network_interface", node.NetworkInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("owner", node.Owner)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("power_interface", node.PowerInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("power_state", node.PowerState)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("root_device", node.Properties["root_device"])
	if err != nil {
		return diag.FromErr(err)
	}
	delete(node.Properties, "root_device")
	err = d.Set("properties", cleanProperties(node.Properties))
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("raid_interface", node.RAIDInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("rescue_interface", node.RescueInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("resource_class", node.ResourceClass)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("storage_interface", node.StorageInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("vendor_interface", node.VendorInterface)
	if err != nil {
		return diag.FromErr(err)
	}
	return diag.FromErr(d.Set("provision_state", node.ProvisionState))
}

// Import the node's data from Ironic.
func resourceNodeV1Import(ctx context.Context, d *schema.ResourceData, meta any) ([]*schema.ResourceData, error) {
	client, err := meta.(*Clients).GetIronicClient()
	if err != nil {
		return []*schema.ResourceData{d}, err
	}

	node, err := nodes.Get(ctx, client, d.Id()).Extract()
	if err != nil {
		d.SetId("")
		return []*schema.ResourceData{d}, err
	}

	// TODO: Ironic's Create is different than the Node object itself, GET returns things like the
	//  RaidConfig, we need to add those and handle them in CREATE
	err = d.Set("boot_interface", node.BootInterface)
	if err != nil {
		return nil, err
	}
	err = d.Set("conductor_group", node.ConductorGroup)
	if err != nil {
		return nil, err
	}
	err = d.Set("console_interface", node.ConsoleInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("deploy_interface", node.DeployInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("driver", node.Driver)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("driver_info", node.DriverInfo)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("extra", node.Extra)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("inspect_interface", node.InspectInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("instance_uuid", node.InstanceUUID)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("management_interface", node.ManagementInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("name", node.Name)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("network_interface", node.NetworkInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("owner", node.Owner)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("power_interface", node.PowerInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("power_state", node.PowerState)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("root_device", node.Properties["root_device"])
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	delete(node.Properties, "root_device")
	err = d.Set("properties", cleanProperties(node.Properties))
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("raid_interface", node.RAIDInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("rescue_interface", node.RescueInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("resource_class", node.ResourceClass)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("storage_interface", node.StorageInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("vendor_interface", node.VendorInterface)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	err = d.Set("provision_state", node.ProvisionState)
	if err != nil {
		return []*schema.ResourceData{d}, err
	}
	return []*schema.ResourceData{d}, nil
}

// Update a node's state based on the terraform config - TODO: handle everything.
func resourceNodeV1Update(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := meta.(*Clients).GetIronicClient()
	if err != nil {
		return diag.FromErr(err)
	}

	d.Partial(true)

	stringFields := []string{
		"boot_interface",
		"conductor_group",
		"console_interface",
		"deploy_interface",
		"driver",
		"inspect_interface",
		"management_interface",
		"name",
		"network_interface",
		"owner",
		"power_interface",
		"raid_interface",
		"rescue_interface",
		"resource_class",
		"storage_interface",
		"vendor_interface",
	}

	for _, field := range stringFields {
		if d.HasChange(field) {
			opts := nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    nodes.ReplaceOp,
					Path:  fmt.Sprintf("/%s", field),
					Value: d.Get(field).(string),
				},
			}

			if _, err := UpdateNode(ctx, client, d.Id(), opts); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if d.HasChange("instance_info") && len(d.Get("instance_info").(map[string]any)) > 0 {
		opts := nodes.UpdateOpts{
			nodes.UpdateOperation{
				Op:    nodes.ReplaceOp,
				Path:  "/instance_info",
				Value: d.Get("instance_info").(map[string]any),
			},
		}

		if _, err := UpdateNode(ctx, client, d.Id(), opts); err != nil {
			return diag.FromErr(err)
		}
	}

	// Make node manageable
	if (d.HasChange("manage") && d.Get("manage").(bool)) ||
		(d.HasChange("clean") && d.Get("clean").(bool)) ||
		(d.HasChange("inspect") && d.Get("inspect").(bool)) {
		if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "manage", nil, nil, nil); err != nil {
			return diag.FromErr(fmt.Errorf("could not manage: %s", err))
		}
	}

	// Update power state if required
	if targetPowerState := d.Get("target_power_state").(string); d.HasChange(
		"target_power_state",
	) &&
		targetPowerState != "" {
		if diags := changePowerState(ctx, client, d, nodes.TargetPowerState(targetPowerState)); diags.HasError() {
			return diags
		}
	}

	// Clean node
	if d.HasChange("clean") && d.Get("clean").(bool) {
		if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "clean", nil, nil, nil); err != nil {
			return diag.FromErr(fmt.Errorf("could not clean: %s", err))
		}
	}

	// Inspect node
	if d.HasChange("inspect") && d.Get("inspect").(bool) {
		if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "inspect", nil, nil, nil); err != nil {
			return diag.FromErr(fmt.Errorf("could not inspect: %s", err))
		}
	}

	// Make node available
	if d.HasChange("available") && d.Get("available").(bool) {
		if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "provide", nil, nil, nil); err != nil {
			return diag.FromErr(fmt.Errorf("could not make node available: %s", err))
		}
	}

	if d.HasChange("properties") || d.HasChange("root_device") {
		properties := propertiesMerge(ctx, d, "root_device")
		opts := nodes.UpdateOpts{
			nodes.UpdateOperation{
				Op:    nodes.AddOp,
				Path:  "/properties",
				Value: properties,
			},
		}
		if _, err := UpdateNode(ctx, client, d.Id(), opts); err != nil {
			return diag.FromErr(err)
		}
	}

	d.Partial(false)

	return resourceNodeV1Read(ctx, d, meta)
}

// Delete a node from Ironic.
func resourceNodeV1Delete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := meta.(*Clients).GetIronicClient()
	if err != nil {
		return diag.FromErr(err)
	}

	if err := ChangeProvisionStateToTarget(ctx, client, d.Id(), "deleted", nil, nil, nil); err != nil {
		return diag.FromErr(err)
	}

	return diag.FromErr(nodes.Delete(ctx, client, d.Id()).ExtractErr())
}

func propertiesMerge(ctx context.Context, d *schema.ResourceData, key string) map[string]any {
	properties := d.Get("properties").(map[string]any)
	properties[key] = d.Get(key).(map[string]any)
	return properties
}

// Convert terraform schema to gophercloud CreateOpts
// TODO: Is there a better way to do this? Annotations?
func schemaToCreateOpts(ctx context.Context, d *schema.ResourceData) *nodes.CreateOpts {
	properties := propertiesMerge(ctx, d, "root_device")
	return &nodes.CreateOpts{
		BootInterface:       d.Get("boot_interface").(string),
		ConductorGroup:      d.Get("conductor_group").(string),
		ConsoleInterface:    d.Get("console_interface").(string),
		DeployInterface:     d.Get("deploy_interface").(string),
		Driver:              d.Get("driver").(string),
		DriverInfo:          d.Get("driver_info").(map[string]any),
		Extra:               d.Get("extra").(map[string]any),
		InspectInterface:    d.Get("inspect_interface").(string),
		ManagementInterface: d.Get("management_interface").(string),
		Name:                d.Get("name").(string),
		NetworkInterface:    d.Get("network_interface").(string),
		Owner:               d.Get("owner").(string),
		PowerInterface:      d.Get("power_interface").(string),
		Properties:          properties,
		RAIDInterface:       d.Get("raid_interface").(string),
		RescueInterface:     d.Get("rescue_interface").(string),
		ResourceClass:       d.Get("resource_class").(string),
		StorageInterface:    d.Get("storage_interface").(string),
		VendorInterface:     d.Get("vendor_interface").(string),
	}
}

// UpdateNode wraps gophercloud's update function, so we are able to retry on 409 when Ironic is busy.
func UpdateNode(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	uuid string,
	opts nodes.UpdateOpts,
) (node *nodes.Node, err error) {
	interval := 5 * time.Second
	for range 5 {
		node, err = nodes.Update(ctx, client, uuid, opts).Extract()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
				log.Printf(
					"[DEBUG] Failed to update node: ironic is busy, will try again in %s",
					interval.String(),
				)
				time.Sleep(interval)
				interval *= 2
				continue
			}
		} else {
			break
		}
	}

	return
}

// Call Ironic's API and change the power state of the node.
func changePowerState(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	d *schema.ResourceData,
	target nodes.TargetPowerState,
) diag.Diagnostics {
	opts := nodes.PowerStateOpts{
		Target: target,
	}

	timeout := d.Get("power_state_timeout").(int)
	if timeout != 0 {
		opts.Timeout = timeout
	} else {
		timeout = 300 // used below for how long to wait for Ironic to finish
	}

	interval := 5 * time.Second
	for retries := 0; retries < 5; retries++ {
		err := nodes.ChangePowerState(ctx, client, d.Id(), opts).ExtractErr()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
				log.Printf(
					"[DEBUG] Failed to change power state: ironic is busy, will try again in %s",
					interval.String(),
				)
				time.Sleep(interval)
				interval *= 2
				continue
			}
		}
	}

	// Wait for target_power_state to be empty, i.e. Ironic thinks it's finished
	checkInterval := 5

	for {
		node, err := nodes.Get(ctx, client, d.Id()).Extract()
		if err != nil {
			return diag.FromErr(err)
		}

		if node.TargetPowerState == "" {
			break
		}

		time.Sleep(time.Duration(checkInterval) * time.Second)
		timeout -= checkInterval
		if timeout <= 0 {
			return diag.FromErr(fmt.Errorf("timed out waiting for power state change"))
		}
	}

	return nil
}

// setRAIDConfig calls ironic's API to send request to change a Node's RAID config.
func setRAIDConfig(ctx context.Context, client *gophercloud.ServiceClient, d *schema.ResourceData) (err error) {
	var logicalDisks []nodes.LogicalDisk
	var targetRAID *metal3v1alpha1.RAIDConfig

	raidConfig := d.Get("raid_config").(string)
	if raidConfig == "" {
		return nil
	}

	err = json.Unmarshal([]byte(raidConfig), &targetRAID)
	if err != nil {
		return
	}

	_, err = ironic.CheckRAIDInterface(d.Get("raid_interface").(string), targetRAID, nil)
	if err != nil {
		return
	}

	// Build target for RAID configuration steps
	logicalDisks, err = ironic.BuildTargetRAIDCfg(targetRAID)
	if len(logicalDisks) == 0 || err != nil {
		return
	}

	// Set root volume
	if len(d.Get("root_device").(map[string]any)) == 0 {
		logicalDisks[0].IsRootVolume = new(bool)
		*logicalDisks[0].IsRootVolume = true
	} else {
		log.Printf("rootDeviceHints is used, the first volume of raid will not be set to root")
	}

	// Set target for RAID configuration steps
	return nodes.SetRAIDConfig(
		ctx,
		client,
		d.Id(),
		nodes.RAIDConfigOpts{LogicalDisks: logicalDisks},
	).ExtractErr()
}

// buildManualCleaningSteps builds the clean steps for RAID and BIOS configuration.
func buildManualCleaningSteps(
	raidInterface, raidConfig, biosSetings string,
) (cleanSteps []nodes.CleanStep, err error) {
	var targetRAID *metal3v1alpha1.RAIDConfig
	var settings []map[string]string

	if raidConfig != "" {
		if err = json.Unmarshal([]byte(raidConfig), &targetRAID); err != nil {
			return nil, err
		}

		// Build raid clean steps
		raidCleanSteps, err := ironic.BuildRAIDCleanSteps(raidInterface, targetRAID, nil)
		if err != nil {
			return nil, err
		}
		cleanSteps = append(cleanSteps, raidCleanSteps...)
	}

	if biosSetings != "" {
		if err = json.Unmarshal([]byte(biosSetings), &settings); err != nil {
			return nil, err
		}

		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: "bios",
				Step:      "apply_configuration",
				Args: map[string]any{
					"settings": settings,
				},
			},
		)
	}

	return
}

func cleanProperties(nodeProperties map[string]any) map[string]any {
	// Clean up the properties map to remove any sensitive data
	properties := make(map[string]any)
	for k, v := range nodeProperties {
		switch typedValue := v.(type) {
		case string:
			properties[k] = typedValue
		case bool:
			properties[k] = strconv.FormatBool(typedValue)
		case int:
			properties[k] = strconv.FormatInt(int64(typedValue), 10)
		case int32:
			properties[k] = strconv.FormatInt(int64(typedValue), 10)
		case int64:
			properties[k] = strconv.FormatInt(typedValue, 10)
		case float32:
			properties[k] = strconv.FormatFloat(float64(typedValue), 'f', -1, 32)
		case float64:
			properties[k] = strconv.FormatFloat(typedValue, 'f', -1, 64)
		case map[string]any:
			properties[k] = typedValue
		default:
			properties[k] = fmt.Sprintf("%v", v)
		}
	}
	return properties
}
